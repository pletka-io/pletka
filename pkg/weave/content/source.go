package content

import (
	"context"
	"embed"
	"io/fs"
)

// ContentSource is the abstraction over where page content comes from.
// Three implementations expected:
//
//   - EmbedSource:   //go:embed content/pages/*.md (the OSS bundle)
//   - OverlaySource: os.DirFS(path) for a runtime-supplied directory
//                    (the platform binary appends one)
//   - DBSource:      future weave_pages table backend
//
// Sources are scanned at boot; later sources shadow earlier ones for
// the same slug, so the platform overlay wins over the embed default.
type ContentSource interface {
	// Name identifies the source for diagnostics + log lines.
	Name() string

	// List returns one entry per (slug, language) the source provides.
	List(ctx context.Context) ([]ContentEntry, error)

	// Read returns the raw markdown bytes for a (slug, language) pair.
	// Implementations should return os.ErrNotExist when the entry is
	// not in this source.
	Read(ctx context.Context, slug, lang string) ([]byte, error)
}

// embedContentFS is the embedded OSS bundle. Page files live next to
// the slice's Go files at pkg/weave/content/pages/<slug>.<lang>.md
// because Go's embed directive cannot reach paths above the package
// directory.
//
//go:embed all:pages
var embedContentFS embed.FS

// EmbedSource reads pages from the binary's embedded fs. Returns the
// OSS bundle. Always present at runtime.
type EmbedSource struct {
	fs fs.FS
}

// NewEmbedSource builds the default OSS source. listEntries walks the
// embedded `pages/` directory; the embed directive above puts the
// markdown files there directly so no fs.Sub is needed.
func NewEmbedSource() *EmbedSource {
	return &EmbedSource{fs: embedContentFS}
}

func (s *EmbedSource) Name() string { return "embed" }

func (s *EmbedSource) List(ctx context.Context) ([]ContentEntry, error) {
	return listEntries(s.fs)
}

func (s *EmbedSource) Read(ctx context.Context, slug, lang string) ([]byte, error) {
	return fs.ReadFile(s.fs, contentPath(slug, lang))
}

// OverlaySource wraps any fs.FS — typically os.DirFS(path) for a
// runtime-supplied directory. Scanning + read paths mirror EmbedSource;
// the only difference is the underlying fs.
type OverlaySource struct {
	fs fs.FS
}

// NewOverlaySource wraps the given fs.FS. Caller decides whether to
// pass os.DirFS, an embed.FS, etc. Returns nil when the input is nil
// so the caller can use idiomatic `if src := ...; src != nil { ... }`.
func NewOverlaySource(fsys fs.FS) *OverlaySource {
	if fsys == nil {
		return nil
	}
	return &OverlaySource{fs: fsys}
}

func (s *OverlaySource) Name() string { return "overlay" }

func (s *OverlaySource) List(ctx context.Context) ([]ContentEntry, error) {
	return listEntries(s.fs)
}

func (s *OverlaySource) Read(ctx context.Context, slug, lang string) ([]byte, error) {
	return fs.ReadFile(s.fs, contentPath(slug, lang))
}

// listEntries walks `pages/` of the given fs and returns one entry
// per `<slug>.<lang>.md` file found. Anything else is skipped.
func listEntries(fsys fs.FS) ([]ContentEntry, error) {
	var out []ContentEntry
	err := fs.WalkDir(fsys, "pages", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// `pages/` may not exist in an overlay that only adds non-
			// page assets; treat that as "no entries" rather than fatal.
			if path == "pages" {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		slug, lang, ok := parseEntryName(d.Name())
		if !ok {
			return nil
		}
		out = append(out, ContentEntry{Slug: slug, Lang: lang})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// parseEntryName splits "<slug>.<lang>.md" into (slug, lang, true).
// Anything else returns ("", "", false) — silently skipped.
func parseEntryName(name string) (slug, lang string, ok bool) {
	const ext = ".md"
	if len(name) <= len(ext) || name[len(name)-len(ext):] != ext {
		return "", "", false
	}
	stem := name[:len(name)-len(ext)]
	// Find the LAST dot in the stem; that separates slug from lang.
	dot := -1
	for i := len(stem) - 1; i >= 0; i-- {
		if stem[i] == '.' {
			dot = i
			break
		}
	}
	if dot <= 0 || dot >= len(stem)-1 {
		return "", "", false
	}
	return stem[:dot], stem[dot+1:], true
}

// contentPath returns the canonical fs path for a given (slug, lang).
func contentPath(slug, lang string) string {
	return "pages/" + slug + "." + lang + ".md"
}
