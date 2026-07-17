package content

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer/html"
	"gopkg.in/yaml.v3"

	"github.com/pletka-io/pletka/pkg/frontendrefs"
	"github.com/pletka-io/pletka/pkg/i18n"
)

// inlineTRef matches a `t:<key>` reference anywhere in a string.
// Keys are dot-separated identifiers (a-z, A-Z, 0-9, ., _, -).
var inlineTRef = regexp.MustCompile(`t:[A-Za-z0-9._-]+`)

var (
	contentWidgetHero  = frontendrefs.ContentWidget("hero")
	contentWidgetProse = frontendrefs.ContentWidget("prose")
	contentWidgetSteps = frontendrefs.ContentWidget("steps")
)

// Loader turns markdown bytes from a ContentSource into a fully
// resolved PageSchema. Frontmatter values starting with "t:" are
// resolved against the supplied i18n.Manager so translators can
// share strings across pages without duplicating into every
// per-language file.
//
// The body of the markdown (everything after the closing --- of the
// frontmatter) is rendered to HTML via goldmark and slotted into an
// implicit `prose` block when present.
type Loader struct {
	i18n i18n.Manager
	md   goldmark.Markdown
}

// NewLoader builds a Loader. nil i18n disables t: resolution (values
// stay literal).
func NewLoader(i18nMgr i18n.Manager) *Loader {
	md := goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithRendererOptions(html.WithUnsafe()),
	)
	return &Loader{i18n: i18nMgr, md: md}
}

// LoadFromSources scans every source and returns slug → lang → schema.
// Sources are processed in order; later sources shadow earlier ones
// for the same (slug, lang) pair. That's how the platform overlay
// shadows the OSS embed.
func (l *Loader) LoadFromSources(ctx context.Context, sources []ContentSource) (map[string]map[string]*PageSchema, error) {
	out := make(map[string]map[string]*PageSchema)
	for _, src := range sources {
		entries, err := src.List(ctx)
		if err != nil {
			return nil, fmt.Errorf("list %s: %w", src.Name(), err)
		}
		for _, e := range entries {
			body, err := src.Read(ctx, e.Slug, e.Lang)
			if err != nil {
				if errors.Is(err, fs.ErrNotExist) {
					continue
				}
				return nil, fmt.Errorf("read %s/%s.%s: %w", src.Name(), e.Slug, e.Lang, err)
			}
			schema, err := l.parse(e.Slug, e.Lang, body)
			if err != nil {
				return nil, fmt.Errorf("parse %s/%s.%s: %w", src.Name(), e.Slug, e.Lang, err)
			}
			if out[e.Slug] == nil {
				out[e.Slug] = make(map[string]*PageSchema)
			}
			out[e.Slug][e.Lang] = schema
		}
	}
	return out, nil
}

// parse splits the file into frontmatter + body, decodes the YAML
// header, normalises blocks, resolves t: references, and renders the
// body to HTML for an implicit prose block.
func (l *Loader) parse(slug, lang string, raw []byte) (*PageSchema, error) {
	fmBytes, body, err := splitFrontmatter(raw)
	if err != nil {
		return nil, err
	}

	var fm frontmatter
	if err := yaml.Unmarshal(fmBytes, &fm); err != nil {
		return nil, fmt.Errorf("yaml: %w", err)
	}

	// Slug from frontmatter overrides the filename slug; default to
	// filename slug when not set.
	if fm.Slug == "" {
		fm.Slug = slug
	}
	if fm.Template == "" {
		fm.Template = "article"
	}

	// Resolve t: references throughout the frontmatter recursively.
	for i := range fm.Blocks {
		fm.Blocks[i] = l.resolve(fm.Blocks[i], lang).(map[string]any)
	}
	fm.SEO.Title = l.resolveString(fm.SEO.Title, lang)
	fm.SEO.Description = l.resolveString(fm.SEO.Description, lang)
	fm.Nav.Label = l.resolveString(fm.Nav.Label, lang)

	blocks := make([]Block, 0, len(fm.Blocks)+1)
	for _, raw := range fm.Blocks {
		blk, ok := normaliseBlock(raw)
		if !ok {
			continue
		}
		// Prose blocks can declare an inline `body` field (markdown
		// string). Render to HTML at load time so the widget receives
		// pre-rendered HTML in `html` like the implicit body case.
		if blk.Type == contentWidgetProse && blk.Data != nil {
			if body, ok := blk.Data["body"].(string); ok && body != "" {
				if html, err := l.renderBody([]byte(body)); err == nil {
					blk.Data["html"] = html
				}
				delete(blk.Data, "body")
			}
		}
		// Hero subtitles are short frontmatter strings, but translators
		// sometimes use inline Markdown for emphasis. Render just this
		// field to HTML so widgets can display emphasis without exposing
		// Markdown markers in the hero.
		if blk.Type == contentWidgetHero && blk.Data != nil {
			if subtitle, ok := blk.Data["subtitle"].(string); ok && subtitle != "" {
				if html, err := l.renderInline([]byte(subtitle)); err == nil {
					blk.Data["subtitle_html"] = html
				}
			}
		}
		// Steps items: each item's `body` markdown → `html`.
		if blk.Type == contentWidgetSteps && blk.Data != nil {
			if items, ok := blk.Data["items"].([]any); ok {
				for i, raw := range items {
					item, ok := raw.(map[string]any)
					if !ok {
						continue
					}
					if body, ok := item["body"].(string); ok && body != "" {
						if html, err := l.renderBody([]byte(body)); err == nil {
							item["html"] = html
						}
						delete(item, "body")
						items[i] = item
					}
				}
			}
		}
		blocks = append(blocks, blk)
	}

	// If there's body content, append it as an implicit prose block
	// (or fill the first explicit prose block that has no html yet).
	if html, err := l.renderBody(body); err == nil && html != "" {
		filled := false
		for i, b := range blocks {
			if b.Type == contentWidgetProse && b.Data["html"] == nil {
				blocks[i].Data["html"] = html
				filled = true
				break
			}
		}
		if !filled {
			blocks = append(blocks, Block{Type: contentWidgetProse, Data: map[string]any{"html": html}})
		}
	}

	return &PageSchema{
		Slug:     fm.Slug,
		Lang:     lang,
		Template: fm.Template,
		Nav:      fm.Nav,
		SEO:      fm.SEO,
		Blocks:   blocks,
	}, nil
}

// resolve walks any decoded YAML value and replaces "t:..." strings
// with their translated counterpart. Maps, slices, and strings are
// the only relevant cases — anything else passes through untouched.
func (l *Loader) resolve(v any, lang string) any {
	switch t := v.(type) {
	case string:
		return l.resolveString(t, lang)
	case map[string]any:
		for k, sub := range t {
			t[k] = l.resolve(sub, lang)
		}
		return t
	case []any:
		for i, sub := range t {
			t[i] = l.resolve(sub, lang)
		}
		return t
	default:
		return v
	}
}

func (l *Loader) resolveString(s, lang string) string {
	if l.i18n == nil {
		return s
	}
	// Whole-string t: ref — common case for short fields like titles.
	if strings.HasPrefix(s, "t:") && !strings.ContainsAny(s, " \n\t") {
		return l.i18n.T(s[2:], lang)
	}
	// Inline refs — substitute any t:key occurrence inside a longer
	// string (e.g. markdown bodies that mix prose with i18n keys).
	return inlineTRef.ReplaceAllStringFunc(s, func(match string) string {
		return l.i18n.T(match[2:], lang)
	})
}

// renderBody runs goldmark over the body of the markdown file and
// returns the resulting HTML. Empty input returns "".
func (l *Loader) renderBody(body []byte) (string, error) {
	body = bytes.TrimSpace(body)
	if len(body) == 0 {
		return "", nil
	}
	var buf bytes.Buffer
	if err := l.md.Convert(body, &buf); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// renderInline renders a short Markdown fragment and removes the single
// paragraph wrapper goldmark adds for ordinary inline content.
func (l *Loader) renderInline(body []byte) (string, error) {
	html, err := l.renderBody(body)
	if err != nil {
		return "", err
	}
	trimmed := strings.TrimSpace(html)
	if strings.HasPrefix(trimmed, "<p>") && strings.HasSuffix(trimmed, "</p>") {
		return strings.TrimSuffix(strings.TrimPrefix(trimmed, "<p>"), "</p>"), nil
	}
	return trimmed, nil
}

// splitFrontmatter cuts a markdown file into (frontmatter-yaml, body).
// Frontmatter is delimited by leading "---\n" and a closing "\n---\n"
// or "\n---\r\n". Files without frontmatter return (nil, full-body).
func splitFrontmatter(raw []byte) ([]byte, []byte, error) {
	const sep = "---"
	trimmed := bytes.TrimPrefix(raw, []byte{0xEF, 0xBB, 0xBF}) // strip UTF-8 BOM
	if !bytes.HasPrefix(trimmed, []byte(sep)) {
		return nil, trimmed, nil
	}
	// Skip the opening separator + its line ending.
	rest := trimmed[len(sep):]
	rest = bytes.TrimLeft(rest, "\r\n")
	end := bytes.Index(rest, []byte("\n"+sep))
	if end < 0 {
		return nil, nil, errors.New("frontmatter: missing closing ---")
	}
	fm := rest[:end]
	body := rest[end+1+len(sep):]
	body = bytes.TrimLeft(body, "\r\n")
	return fm, body, nil
}

// normaliseBlock pulls "type" out of a raw frontmatter map and packs
// the remaining keys into the Block.Data map. Returns false when the
// raw map is missing a usable type.
func normaliseBlock(raw map[string]any) (Block, bool) {
	t, ok := raw["type"].(string)
	if !ok || t == "" {
		return Block{}, false
	}
	data := make(map[string]any, len(raw))
	for k, v := range raw {
		if k == "type" {
			continue
		}
		data[k] = v
	}
	return Block{Type: t, Data: data}, true
}
