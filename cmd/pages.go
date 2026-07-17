// Package cmd — pages: roundtrip page prose into vanilla markdown for editing.
//
// extract: walks pkg/weave/content/pages/*.en.md, finds every t:KEY in the
// frontmatter, and emits an editable .md per (slug, language) under
// pkg/weave/content/pages-edits/. The user edits prose between
// <!--key:...--> markers.
//
// hydrate: parses editable markdown and writes the new prose values back
// into pkg/assets/i18n/<lang>.json. Byte-level replace preserves key order
// and surrounding formatting.
package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

const (
	tPrefix       = "t:"
	editableHdr   = "<!-- pletka-pages-edit v1"
	keyMarkerOpen = "<!--key:"
	keyMarkerEnd  = "-->"
)

var (
	pagesSrcDir       string
	pagesOutDir       string
	pagesI18nDir      string
	pagesHydrateInDir string
	pagesLangsCSV     string
)

func newPagesCommand() *cobra.Command {
	pagesCmd := &cobra.Command{
		Use:   "pages",
		Short: "Roundtrip page prose to vanilla markdown for editing",
	}
	pagesExtractCmd := &cobra.Command{
		Use:   "extract",
		Short: "Extract t:KEY values into editable .md files (one per slug+lang)",
		RunE:  runPagesExtract,
	}
	pagesHydrateCmd := &cobra.Command{
		Use:   "hydrate",
		Short: "Write edited values back into i18n JSON files",
		RunE:  runPagesHydrate,
	}
	pagesPromoteCmd := &cobra.Command{
		Use:   "promote [slug...]",
		Short: "Convert literal strings in a page to t:KEY refs and append to en.json",
		Long: `Walks frontmatter of pkg/weave/content/pages/<slug>.en.md, replaces
every translatable string scalar with a t:pages.<slug>.<auto_key> reference,
and appends the originals to pages.<slug>.* in pkg/assets/i18n/en.json.

With no args: promotes every page that has zero existing t:KEY references.
With slug args: promotes only those pages.
Use --all to force-promote every page (idempotent — already-keyed strings skip).`,
		RunE: runPagesPromote,
	}

	pagesExtractCmd.Flags().StringVar(&pagesSrcDir, "src", "pkg/weave/content/pages", "source page dir")
	pagesExtractCmd.Flags().StringVar(&pagesOutDir, "out", "pkg/weave/content/pages-edits", "editable output dir")
	pagesExtractCmd.Flags().StringVar(&pagesI18nDir, "i18n", "pkg/assets/i18n", "i18n json dir")
	pagesExtractCmd.Flags().StringVar(&pagesLangsCSV, "langs", "", "comma-separated language codes (default: all *.json in i18n dir)")

	pagesHydrateCmd.Flags().StringVar(&pagesHydrateInDir, "in", "pkg/weave/content/pages-edits", "editable input dir")
	pagesHydrateCmd.Flags().StringVar(&pagesI18nDir, "i18n", "pkg/assets/i18n", "i18n json dir")

	pagesPromoteCmd.Flags().StringVar(&pagesSrcDir, "src", "pkg/weave/content/pages", "source page dir")
	pagesPromoteCmd.Flags().StringVar(&pagesI18nDir, "i18n", "pkg/assets/i18n", "i18n json dir")
	pagesPromoteCmd.Flags().BoolVar(&pagesPromoteAll, "all", false, "promote every page (default: only pages with zero t:KEY refs)")

	pagesCmd.AddCommand(pagesExtractCmd, pagesHydrateCmd, pagesPromoteCmd)
	return pagesCmd
}

var pagesPromoteAll bool

// ─── promote ──────────────────────────────────────────────────────────────

// promoteSkipKeys are mapping keys whose scalar values are NOT promoted.
// Anything outside this list (and not already `t:`) is promoted.
var promoteSkipKeys = map[string]bool{
	"slug": true, "template": true,
	"type": true, "style": true, "columns": true, "icon": true, "href": true,
	"order": true, "visible": true,
	"dot": true, "color": true, "levels": true, "rule": true,
}

type promotePair struct {
	key   string // dotted path within pages.<slug>, e.g. "blocks_0_title"
	value string
}

func runPagesPromote(_ *cobra.Command, args []string) error {
	targets, err := resolvePromoteTargets(args)
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		fmt.Println("no pages need promotion")
		return nil
	}

	enPath := filepath.Join(pagesI18nDir, "en.json")
	enJSON, err := os.ReadFile(enPath)
	if err != nil {
		return fmt.Errorf("read en.json: %w", err)
	}

	for _, slug := range targets {
		pagePath := filepath.Join(pagesSrcDir, slug+".en.md")
		pairs, err := promotePageFile(pagePath, slug)
		if err != nil {
			return fmt.Errorf("promote %s: %w", slug, err)
		}
		if len(pairs) == 0 {
			fmt.Printf("%s — nothing to promote\n", slug)
			continue
		}
		enJSON, err = insertPageKeys(enJSON, slug, pairs)
		if err != nil {
			return fmt.Errorf("insert keys for %s: %w", slug, err)
		}
		fmt.Printf("%s — promoted %d strings\n", slug, len(pairs))
	}

	if err := os.WriteFile(enPath, enJSON, 0o644); err != nil {
		return fmt.Errorf("write en.json: %w", err)
	}
	fmt.Printf("updated %s\n", enPath)
	return nil
}

func resolvePromoteTargets(args []string) ([]string, error) {
	if len(args) > 0 {
		return args, nil
	}
	pages, err := filepath.Glob(filepath.Join(pagesSrcDir, "*.en.md"))
	if err != nil {
		return nil, err
	}
	var out []string
	for _, p := range pages {
		slug := strings.TrimSuffix(filepath.Base(p), ".en.md")
		if pagesPromoteAll {
			out = append(out, slug)
			continue
		}
		keys, err := extractKeysFromPage(p)
		if err != nil {
			return nil, err
		}
		if len(keys) == 0 {
			out = append(out, slug)
		}
	}
	sort.Strings(out)
	return out, nil
}

// promotePageFile rewrites the page's frontmatter, replacing literal string
// scalars with `t:pages.<slug>.<key>` references. Returns the (key, value)
// pairs collected.
func promotePageFile(path, slug string) ([]promotePair, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	src := string(raw)
	if !strings.HasPrefix(src, "---") {
		return nil, fmt.Errorf("missing frontmatter")
	}
	endIdx := strings.Index(src[3:], "\n---")
	if endIdx < 0 {
		return nil, fmt.Errorf("unterminated frontmatter")
	}
	fmBody := src[3 : 3+endIdx]
	rest := src[3+endIdx+4:] // text after the closing "---" line

	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(fmBody), &doc); err != nil {
		return nil, fmt.Errorf("yaml: %w", err)
	}

	var pairs []promotePair
	seen := map[string]int{}
	promoteWalk(&doc, "", nil, slug, &pairs, seen)

	if len(pairs) == 0 {
		return nil, nil
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		return nil, fmt.Errorf("yaml encode: %w", err)
	}
	enc.Close()
	newFM := unescapeYAMLUnicode(strings.TrimSpace(buf.String()))

	out := "---\n" + newFM + "\n---" + rest
	if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
		return nil, err
	}
	return pairs, nil
}

// promoteWalk traverses the YAML node tree. parentKey is the mapping key
// that introduced the current node (empty at root). pathStack accumulates
// readable path components used to build the auto-key.
func promoteWalk(n *yaml.Node, parentKey string, pathStack []string, slug string, out *[]promotePair, seen map[string]int) {
	if n == nil {
		return
	}
	switch n.Kind {
	case yaml.DocumentNode:
		for _, c := range n.Content {
			promoteWalk(c, parentKey, pathStack, slug, out, seen)
		}
	case yaml.SequenceNode:
		for i, c := range n.Content {
			promoteWalk(c, parentKey, append(pathStack, fmt.Sprintf("%d", i)), slug, out, seen)
		}
	case yaml.MappingNode:
		for i := 0; i+1 < len(n.Content); i += 2 {
			k := n.Content[i].Value
			promoteWalk(n.Content[i+1], k, append(pathStack, k), slug, out, seen)
		}
	case yaml.ScalarNode:
		if promoteSkipKeys[parentKey] {
			return
		}
		if n.Tag != "" && n.Tag != "!!str" {
			return
		}
		v := strings.TrimRight(n.Value, " \t\n\r")
		if v == "" {
			return
		}
		if strings.HasPrefix(v, "t:") {
			return
		}
		base := strings.Join(pathStack, "_")
		base = sanitizeKeyPart(base)
		key := base
		if seen[key] > 0 {
			key = fmt.Sprintf("%s_%d", base, seen[base])
		}
		seen[base]++
		*out = append(*out, promotePair{key: key, value: v})
		n.Value = "t:pages." + slug + "." + key
		n.Style = 0
		n.Tag = ""
	}
}

// unescapeYAMLUnicode replaces \UXXXXXXXX and \uXXXX escape sequences in
// the marshaled YAML with the literal UTF-8 codepoint. yaml.v3 escapes
// non-BMP characters in double-quoted strings; this restores them so the
// page source stays human-readable.
var (
	yamlEscape8 = regexp.MustCompile(`\\U([0-9A-Fa-f]{8})`)
	yamlEscape4 = regexp.MustCompile(`\\u([0-9A-Fa-f]{4})`)
)

func unescapeYAMLUnicode(s string) string {
	s = yamlEscape8.ReplaceAllStringFunc(s, func(m string) string {
		var cp uint32
		fmt.Sscanf(m[2:], "%x", &cp)
		return string(rune(cp))
	})
	s = yamlEscape4.ReplaceAllStringFunc(s, func(m string) string {
		var cp uint32
		fmt.Sscanf(m[2:], "%x", &cp)
		return string(rune(cp))
	})
	return s
}

func sanitizeKeyPart(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '_':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + 32)
		default:
			b.WriteByte('_')
		}
	}
	out := b.String()
	for strings.Contains(out, "__") {
		out = strings.ReplaceAll(out, "__", "_")
	}
	return strings.Trim(out, "_")
}

// insertPageKeys inserts the given key/value pairs into pages.<slug>.* in
// the JSON bytes. If pages.<slug> does not exist, it is created. Surrounding
// formatting and key order of unrelated entries are preserved.
func insertPageKeys(raw []byte, slug string, pairs []promotePair) ([]byte, error) {
	pagesOpen, err := findKeyObjectOpen(raw, 0, "pages")
	if err != nil {
		return nil, fmt.Errorf("locate pages: %w", err)
	}
	pagesClose, err := matchBrace(raw, pagesOpen)
	if err != nil {
		return nil, err
	}

	// Try to find slug block within pages.
	slugOpen, slugErr := findKeyObjectOpen(raw[:pagesClose], pagesOpen+1, slug)
	if slugErr != nil {
		// Slug doesn't exist — create it.
		return createSlugBlock(raw, pagesOpen, pagesClose, slug, pairs)
	}
	slugClose, err := matchBrace(raw, slugOpen)
	if err != nil {
		return nil, err
	}
	return appendIntoSlugBlock(raw, slugOpen, slugClose, pairs)
}

// createSlugBlock inserts a new "<slug>": { ... } entry just before the
// closing `}` of the pages object.
func createSlugBlock(raw []byte, pagesOpen, pagesClose int, slug string, pairs []promotePair) ([]byte, error) {
	// Determine if pages object is empty (only whitespace between { and }).
	body := raw[pagesOpen+1 : pagesClose]
	hasEntries := false
	for _, c := range body {
		if !isWS(c) {
			hasEntries = true
			break
		}
	}

	var sb strings.Builder
	if hasEntries {
		// Existing entries — need to add comma after last entry.
	}
	sb.WriteString("\n    \"")
	sb.WriteString(slug)
	sb.WriteString("\": {\n")
	for i, p := range pairs {
		v, err := encodeJSONString(p.value)
		if err != nil {
			return nil, err
		}
		sb.WriteString("      \"")
		sb.WriteString(p.key)
		sb.WriteString("\": ")
		sb.Write(v)
		if i < len(pairs)-1 {
			sb.WriteByte(',')
		}
		sb.WriteByte('\n')
	}
	sb.WriteString("    }\n  ")

	insertion := sb.String()
	if hasEntries {
		// Walk back from pagesClose to find the last non-whitespace; append
		// `,` after it, then our block.
		i := pagesClose - 1
		for i > pagesOpen && isWS(raw[i]) {
			i--
		}
		// i now points at last non-ws char before }. If it is `,`, the
		// pages object already has a trailing-comma style — leave alone;
		// otherwise insert comma.
		needComma := raw[i] != ','
		out := make([]byte, 0, len(raw)+len(insertion)+1)
		out = append(out, raw[:i+1]...)
		if needComma {
			out = append(out, ',')
		}
		out = append(out, []byte(insertion)...)
		out = append(out, raw[pagesClose:]...)
		return out, nil
	}
	// Empty pages object — replace its body.
	out := make([]byte, 0, len(raw)+len(insertion))
	out = append(out, raw[:pagesOpen+1]...)
	out = append(out, []byte(insertion)...)
	out = append(out, raw[pagesClose:]...)
	return out, nil
}

// appendIntoSlugBlock inserts pairs just before the closing `}` of the slug
// block, adding a comma to the previous last entry as needed.
func appendIntoSlugBlock(raw []byte, slugOpen, slugClose int, pairs []promotePair) ([]byte, error) {
	body := raw[slugOpen+1 : slugClose]
	hasEntries := false
	for _, c := range body {
		if !isWS(c) {
			hasEntries = true
			break
		}
	}

	var sb strings.Builder
	if !hasEntries {
		sb.WriteByte('\n')
	}
	for i, p := range pairs {
		v, err := encodeJSONString(p.value)
		if err != nil {
			return nil, err
		}
		sb.WriteString("      \"")
		sb.WriteString(p.key)
		sb.WriteString("\": ")
		sb.Write(v)
		if i < len(pairs)-1 {
			sb.WriteByte(',')
		}
		sb.WriteByte('\n')
	}
	sb.WriteString("    ")

	insertion := sb.String()
	if hasEntries {
		i := slugClose - 1
		for i > slugOpen && isWS(raw[i]) {
			i--
		}
		needComma := raw[i] != ','
		out := make([]byte, 0, len(raw)+len(insertion)+1)
		out = append(out, raw[:i+1]...)
		if needComma {
			out = append(out, ',')
		}
		out = append(out, '\n')
		out = append(out, []byte(insertion)...)
		out = append(out, raw[slugClose:]...)
		return out, nil
	}
	out := make([]byte, 0, len(raw)+len(insertion))
	out = append(out, raw[:slugOpen+1]...)
	out = append(out, []byte(insertion)...)
	out = append(out, raw[slugClose:]...)
	return out, nil
}

// ─── extract ──────────────────────────────────────────────────────────────

func runPagesExtract(_ *cobra.Command, _ []string) error {
	if err := os.MkdirAll(pagesOutDir, 0o755); err != nil {
		return fmt.Errorf("mkdir out: %w", err)
	}

	langs, err := resolveLangs()
	if err != nil {
		return err
	}

	pages, err := filepath.Glob(filepath.Join(pagesSrcDir, "*.en.md"))
	if err != nil {
		return fmt.Errorf("glob pages: %w", err)
	}
	if len(pages) == 0 {
		return fmt.Errorf("no pages found in %s", pagesSrcDir)
	}

	// Pre-load each lang JSON once.
	langData := map[string]map[string]any{}
	for _, lang := range langs {
		m, err := readJSONMap(filepath.Join(pagesI18nDir, lang+".json"))
		if err != nil {
			return fmt.Errorf("read %s.json: %w", lang, err)
		}
		langData[lang] = m
	}

	for _, p := range pages {
		slug := strings.TrimSuffix(filepath.Base(p), ".en.md")
		keys, err := extractKeysFromPage(p)
		if err != nil {
			return err
		}
		if len(keys) == 0 {
			fmt.Printf("skip %s — no t:KEY references\n", slug)
			continue
		}
		for _, lang := range langs {
			out := filepath.Join(pagesOutDir, fmt.Sprintf("%s.%s.md", slug, lang))
			body := renderEditable(slug, lang, keys, langData[lang])
			if err := os.WriteFile(out, []byte(body), 0o644); err != nil {
				return fmt.Errorf("write %s: %w", out, err)
			}
			fmt.Printf("wrote %s (%d keys)\n", out, len(keys))
		}
	}
	return nil
}

func resolveLangs() ([]string, error) {
	if pagesLangsCSV != "" {
		var out []string
		for _, s := range strings.Split(pagesLangsCSV, ",") {
			s = strings.TrimSpace(s)
			if s != "" {
				out = append(out, s)
			}
		}
		return out, nil
	}
	matches, err := filepath.Glob(filepath.Join(pagesI18nDir, "*.json"))
	if err != nil {
		return nil, fmt.Errorf("glob i18n: %w", err)
	}
	var out []string
	for _, m := range matches {
		base := strings.TrimSuffix(filepath.Base(m), ".json")
		if base == "languages" {
			continue
		}
		out = append(out, base)
	}
	sort.Strings(out)
	return out, nil
}

// extractKeysFromPage returns the ordered, deduplicated list of dotted i18n
// keys referenced by `t:` strings in the page's frontmatter.
func extractKeysFromPage(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	fm := splitFrontmatter(data)
	var node yaml.Node
	if err := yaml.Unmarshal(fm, &node); err != nil {
		return nil, fmt.Errorf("yaml %s: %w", path, err)
	}
	seen := map[string]bool{}
	var keys []string
	walkScalars(&node, func(s string) {
		for _, m := range tKeyRe.FindAllString(s, -1) {
			k := strings.TrimPrefix(m, tPrefix)
			if seen[k] {
				continue
			}
			seen[k] = true
			keys = append(keys, k)
		}
	})
	return keys, nil
}

var tKeyRe = regexp.MustCompile(`t:[A-Za-z0-9_][A-Za-z0-9_.\-]*`)

func splitFrontmatter(data []byte) []byte {
	s := string(data)
	if !strings.HasPrefix(s, "---") {
		return data
	}
	rest := s[3:]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return data
	}
	return []byte(rest[:end])
}

func walkScalars(n *yaml.Node, fn func(string)) {
	if n == nil {
		return
	}
	switch n.Kind {
	case yaml.DocumentNode, yaml.SequenceNode:
		for _, c := range n.Content {
			walkScalars(c, fn)
		}
	case yaml.MappingNode:
		for i := 0; i+1 < len(n.Content); i += 2 {
			walkScalars(n.Content[i+1], fn)
		}
	case yaml.ScalarNode:
		if n.Tag == "" || n.Tag == "!!str" {
			fn(n.Value)
		}
	}
}

func readJSONMap(path string) (map[string]any, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return m, nil
}

// lookup resolves a dotted path "pages.home.tagline" against a generic map.
func lookup(m map[string]any, dotted string) (string, bool) {
	parts := strings.Split(dotted, ".")
	var cur any = m
	for _, p := range parts {
		mm, ok := cur.(map[string]any)
		if !ok {
			return "", false
		}
		cur, ok = mm[p]
		if !ok {
			return "", false
		}
	}
	s, ok := cur.(string)
	return s, ok
}

func renderEditable(slug, lang string, keys []string, data map[string]any) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\nslug: %s\nlang: %s\n-->\n\n", editableHdr, slug, lang)
	fmt.Fprintf(&b, "# %s (%s)\n\n", titleSlug(slug), lang)
	b.WriteString("Edit prose between the `<!--key:...-->` markers. Do not rename, reorder, or remove the markers. Run `pletka pages hydrate` to write changes back to the language JSON.\n\n")
	for _, k := range keys {
		val, ok := lookup(data, k)
		fmt.Fprintf(&b, "<!--key:%s-->\n", k)
		if !ok || val == "" {
			b.WriteString("<!--MISSING-->\n\n")
			continue
		}
		b.WriteString(val)
		b.WriteString("\n\n")
	}
	return b.String()
}

// ─── hydrate ──────────────────────────────────────────────────────────────

type editableDoc struct {
	slug   string
	lang   string
	values map[string]string // key (dotted) -> new value (may be empty)
}

func runPagesHydrate(_ *cobra.Command, _ []string) error {
	files, err := filepath.Glob(filepath.Join(pagesHydrateInDir, "*.md"))
	if err != nil {
		return fmt.Errorf("glob editable: %w", err)
	}
	if len(files) == 0 {
		return fmt.Errorf("no editable files in %s", pagesHydrateInDir)
	}

	// Group updates by language.
	perLang := map[string][]editableDoc{}
	for _, f := range files {
		doc, err := parseEditable(f)
		if err != nil {
			return fmt.Errorf("parse %s: %w", f, err)
		}
		perLang[doc.lang] = append(perLang[doc.lang], *doc)
	}

	for lang, docs := range perLang {
		path := filepath.Join(pagesI18nDir, lang+".json")
		raw, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		// Build flat map slug -> { leaf-key -> new value } for non-empty values.
		bySlug := map[string]map[string]string{}
		for _, d := range docs {
			for fullKey, v := range d.values {
				if v == "" {
					continue
				}
				leaf, slug, ok := splitLeaf(fullKey, d.slug)
				if !ok {
					continue
				}
				if bySlug[slug] == nil {
					bySlug[slug] = map[string]string{}
				}
				bySlug[slug][leaf] = v
			}
		}

		out := raw
		for slug, kv := range bySlug {
			out, err = applySlugUpdates(out, slug, kv)
			if err != nil {
				return fmt.Errorf("apply %s/%s: %w", lang, slug, err)
			}
		}
		if !bytes.Equal(out, raw) {
			if err := os.WriteFile(path, out, 0o644); err != nil {
				return fmt.Errorf("write %s: %w", path, err)
			}
			fmt.Printf("updated %s\n", path)
		} else {
			fmt.Printf("%s — no changes\n", path)
		}
	}
	return nil
}

// splitLeaf takes "pages.home.tagline" + slug "home" and returns "tagline".
func splitLeaf(fullKey, slug string) (leaf, foundSlug string, ok bool) {
	parts := strings.Split(fullKey, ".")
	if len(parts) < 3 || parts[0] != "pages" {
		return "", "", false
	}
	foundSlug = parts[1]
	if foundSlug != slug {
		return "", "", false
	}
	leaf = strings.Join(parts[2:], ".")
	return leaf, foundSlug, true
}

var (
	hdrSlugRe = regexp.MustCompile(`(?m)^slug:\s*(\S+)\s*$`)
	hdrLangRe = regexp.MustCompile(`(?m)^lang:\s*(\S+)\s*$`)
	keyRe     = regexp.MustCompile(`<!--key:(.+?)-->`)
)

func parseEditable(path string) (*editableDoc, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	src := string(b)
	if !strings.HasPrefix(src, editableHdr) {
		return nil, fmt.Errorf("missing pletka-pages-edit header")
	}
	hdrEnd := strings.Index(src, "-->")
	if hdrEnd < 0 {
		return nil, fmt.Errorf("unterminated header comment")
	}
	hdr := src[:hdrEnd]
	slugMatch := hdrSlugRe.FindStringSubmatch(hdr)
	langMatch := hdrLangRe.FindStringSubmatch(hdr)
	if slugMatch == nil || langMatch == nil {
		return nil, fmt.Errorf("header missing slug/lang")
	}
	doc := &editableDoc{slug: slugMatch[1], lang: langMatch[1], values: map[string]string{}}

	body := src[hdrEnd+3:]
	matches := keyRe.FindAllStringSubmatchIndex(body, -1)
	for i, m := range matches {
		key := body[m[2]:m[3]]
		valStart := m[1]
		var valEnd int
		if i+1 < len(matches) {
			valEnd = matches[i+1][0]
		} else {
			valEnd = len(body)
		}
		raw := body[valStart:valEnd]
		raw = strings.TrimSpace(raw)
		if raw == "<!--MISSING-->" {
			raw = ""
		}
		doc.values[key] = raw
	}
	return doc, nil
}

// ─── targeted JSON byte-replace ───────────────────────────────────────────

// applySlugUpdates replaces leaf string values within pages.<slug> in raw JSON
// bytes, preserving all surrounding formatting and key order. Keys that do
// not yet exist in the slug block are appended; if the slug block itself is
// missing, it is created.
func applySlugUpdates(raw []byte, slug string, kv map[string]string) ([]byte, error) {
	if len(kv) == 0 {
		return raw, nil
	}
	slugStart, _, err := findSlugBlock(raw, slug)
	if err != nil {
		// Slug block missing — create it with all kv as new pairs.
		var pairs []promotePair
		for k, v := range kv {
			pairs = append(pairs, promotePair{key: k, value: v})
		}
		sort.Slice(pairs, func(i, j int) bool { return pairs[i].key < pairs[j].key })
		return insertPageKeys(raw, slug, pairs)
	}
	ranges, err := scanLeafStringRanges(raw, slugStart)
	if err != nil {
		return nil, fmt.Errorf("scan slug body: %w", err)
	}
	type rep struct {
		start, end int
		val        string
	}
	var reps []rep
	var newPairs []promotePair
	for k, v := range kv {
		r, ok := ranges[k]
		if !ok {
			newPairs = append(newPairs, promotePair{key: k, value: v})
			continue
		}
		reps = append(reps, rep{r[0], r[1], v})
	}
	sort.Slice(reps, func(i, j int) bool { return reps[i].start > reps[j].start })

	out := make([]byte, len(raw))
	copy(out, raw)
	for _, r := range reps {
		encoded, err := encodeJSONString(r.val)
		if err != nil {
			return nil, err
		}
		buf := make([]byte, 0, len(out)+len(encoded))
		buf = append(buf, out[:r.start]...)
		buf = append(buf, encoded...)
		buf = append(buf, out[r.end:]...)
		out = buf
	}
	if len(newPairs) > 0 {
		sort.Slice(newPairs, func(i, j int) bool { return newPairs[i].key < newPairs[j].key })
		out, err = insertPageKeys(out, slug, newPairs)
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

// encodeJSONString returns the JSON-encoded form of s without HTML-escaping
// `&`, `<`, `>` (matches the existing files' style) and without the trailing
// newline that json.Encoder appends.
func encodeJSONString(s string) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(s); err != nil {
		return nil, err
	}
	out := buf.Bytes()
	// Drop the trailing newline that Encoder always writes.
	if len(out) > 0 && out[len(out)-1] == '\n' {
		out = out[:len(out)-1]
	}
	return out, nil
}

// findSlugBlock returns [open, close+1) byte range of pages.<slug>'s object
// value (inclusive of opening { and closing }).
func findSlugBlock(data []byte, slug string) (int, int, error) {
	pagesOpen, err := findKeyObjectOpen(data, 0, "pages")
	if err != nil {
		return 0, 0, err
	}
	pagesClose, err := matchBrace(data, pagesOpen)
	if err != nil {
		return 0, 0, err
	}
	slugOpen, err := findKeyObjectOpen(data[:pagesClose], pagesOpen+1, slug)
	if err != nil {
		return 0, 0, err
	}
	slugClose, err := matchBrace(data, slugOpen)
	if err != nil {
		return 0, 0, err
	}
	return slugOpen, slugClose + 1, nil
}

// findKeyObjectOpen scans data[from:] for `"key"` followed by `:` then `{`,
// skipping over JSON strings so it never matches the key name inside a value.
// Returns the index of the `{`.
func findKeyObjectOpen(data []byte, from int, key string) (int, error) {
	needle := []byte(`"` + key + `"`)
	i := from
	for i < len(data) {
		c := data[i]
		if c == '"' {
			// Check for direct match at i.
			if i+len(needle) <= len(data) && bytes.Equal(data[i:i+len(needle)], needle) {
				j := i + len(needle)
				for j < len(data) && (data[j] == ' ' || data[j] == '\t' || data[j] == '\n' || data[j] == '\r') {
					j++
				}
				if j < len(data) && data[j] == ':' {
					j++
					for j < len(data) && (data[j] == ' ' || data[j] == '\t' || data[j] == '\n' || data[j] == '\r') {
						j++
					}
					if j < len(data) && data[j] == '{' {
						return j, nil
					}
				}
			}
			// Skip past this string.
			i = skipString(data, i)
			continue
		}
		i++
	}
	return 0, fmt.Errorf("key %q not found as object", key)
}

// skipString returns the index just past the closing `"` of the JSON string
// starting at data[start] == '"'. Handles backslash escapes.
func skipString(data []byte, start int) int {
	i := start + 1
	for i < len(data) {
		c := data[i]
		if c == '\\' {
			i += 2
			continue
		}
		if c == '"' {
			return i + 1
		}
		i++
	}
	return len(data)
}

// matchBrace finds the index of the `}` matching the `{` at openIdx,
// honoring strings and escapes.
func matchBrace(data []byte, openIdx int) (int, error) {
	if openIdx >= len(data) || data[openIdx] != '{' {
		return 0, fmt.Errorf("not at open brace")
	}
	depth := 0
	for i := openIdx; i < len(data); i++ {
		c := data[i]
		switch c {
		case '"':
			i = skipString(data, i) - 1
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i, nil
			}
		}
	}
	return 0, fmt.Errorf("no matching brace")
}

// scanLeafStringRanges returns key -> [start,end) byte range of the leaf
// string value (including surrounding quotes) for each top-level key in the
// JSON object that opens at data[objOpen] == '{'. Non-string leaves are
// skipped silently.
func scanLeafStringRanges(data []byte, objOpen int) (map[string][2]int, error) {
	if objOpen >= len(data) || data[objOpen] != '{' {
		return nil, fmt.Errorf("not at open brace")
	}
	out := map[string][2]int{}
	i := objOpen + 1
	for i < len(data) {
		// skip whitespace
		for i < len(data) && isWS(data[i]) {
			i++
		}
		if i >= len(data) {
			break
		}
		if data[i] == '}' {
			return out, nil
		}
		if data[i] == ',' {
			i++
			continue
		}
		if data[i] != '"' {
			return nil, fmt.Errorf("expected key string at %d, got %q", i, data[i])
		}
		keyStart := i + 1
		keyEnd := skipString(data, i) - 1 // position of closing quote
		key := string(data[keyStart:keyEnd])
		i = keyEnd + 1
		for i < len(data) && isWS(data[i]) {
			i++
		}
		if i >= len(data) || data[i] != ':' {
			return nil, fmt.Errorf("expected colon after key %q", key)
		}
		i++
		for i < len(data) && isWS(data[i]) {
			i++
		}
		if i >= len(data) {
			return nil, fmt.Errorf("unexpected EOF after key %q", key)
		}
		switch data[i] {
		case '"':
			valStart := i
			valEnd := skipString(data, i)
			out[key] = [2]int{valStart, valEnd}
			i = valEnd
		case '{':
			closeIdx, err := matchBrace(data, i)
			if err != nil {
				return nil, err
			}
			i = closeIdx + 1
		case '[':
			i = skipArray(data, i)
		default:
			// number, bool, null — read until comma or }
			for i < len(data) && data[i] != ',' && data[i] != '}' {
				i++
			}
		}
	}
	return nil, fmt.Errorf("unterminated object")
}

func isWS(c byte) bool { return c == ' ' || c == '\t' || c == '\n' || c == '\r' }

func titleSlug(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// skipArray returns index just past the closing `]` of the array starting
// at data[start] == '['. Honors strings and nested arrays.
func skipArray(data []byte, start int) int {
	depth := 0
	i := start
	for i < len(data) {
		switch data[i] {
		case '"':
			i = skipString(data, i)
			continue
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return i + 1
			}
		}
		i++
	}
	return len(data)
}
