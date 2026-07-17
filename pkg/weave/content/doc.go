// Package content owns the static-content page surface — home, about,
// zen, vision, community, and any future page added via a markdown
// file. This package is a handler-only module: it composes page schemas
// from pluggable ContentSource implementations and owns no store.
//
// Pages are stored as markdown with YAML frontmatter, one file per
// (slug, language) pair under content/pages/<slug>.<lang>.md. The
// frontmatter declares a list of typed content blocks (hero, prose,
// feature_grid, principles, cta_strip, toc, quote, ...). The package
// loads each file into a PageSchema, JSON-encodes the schema, and
// hands it to a single shared Svelte island ("content") that walks
// the blocks and dispatches each to the matching widget — same shape
// as the form-schema + WidgetDispatcher pattern.
//
// The page-level "template" frontmatter field picks the outer chrome
// (article | hero | long-form). Per-block widgets handle their own
// rendering + styling.
//
// Content sources are pluggable via ContentSource:
//   - EmbedSource:   //go:embed content/pages/* (the OSS bundle)
//   - OverlaySource: os.DirFS(path) for a runtime-supplied directory
//     (the platform binary appends one)
//   - DBSource:      future weave_pages table backend
//
// The handler scans sources at boot, builds a slug-indexed registry,
// and registers chi routes per slug. Adding a page = drop a markdown
// file (or row, when DBSource lands).
package content
