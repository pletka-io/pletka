package content

// Block is the in-memory shape of a single content block parsed from
// frontmatter. Type names match the widget registry on the frontend
// (hero, prose, feature_grid, principles, cta_strip, toc, quote, ...).
//
// The Data field holds the raw block payload as a generic map; per-
// type validation happens at widget level on the frontend. This
// keeps the loader simple and lets new block types ship without
// touching server-side struct definitions.
type Block struct {
	Type string         `json:"type"`
	Data map[string]any `json:"data,omitempty"`
}

// NavMeta is the optional nav frontmatter section. Used by future
// nav-generators (top nav, footer link list, sitemap). Today it's
// just metadata that ships in the schema for the frontend to ignore.
type NavMeta struct {
	Label   string `json:"label,omitempty"`
	Order   int    `json:"order,omitempty"`
	Visible bool   `json:"visible,omitempty"`
}

// SEOMeta carries page-level metadata used to populate <title> and
// <meta name="description"> in the shell.
type SEOMeta struct {
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
}

// PageSchema is what the loader produces and the frontend consumes.
// JSON-encoded onto the island's data-prop-schema attribute.
type PageSchema struct {
	Slug     string  `json:"slug"`
	Lang     string  `json:"lang"`
	Template string  `json:"template"`
	Nav      NavMeta `json:"nav,omitempty"`
	SEO      SEOMeta `json:"seo,omitempty"`
	Blocks   []Block `json:"blocks"`
}

// ContentEntry is what a ContentSource lists: one row per
// (slug, language) pair. Slug is the route segment ("home" → "/",
// "about" → "/about"). Lang is a BCP-47 code ("en", "nl").
type ContentEntry struct {
	Slug string
	Lang string
}

// frontmatter is the YAML-decoded shape of the page header. Fields
// match the frontmatter keys verbatim. The blocks slice carries raw
// `map[string]any` entries because each block type has its own
// payload — the loader normalises into Block records.
type frontmatter struct {
	Slug     string                   `yaml:"slug"`
	Template string                   `yaml:"template"`
	Nav      NavMeta                  `yaml:"nav"`
	SEO      SEOMeta                  `yaml:"seo"`
	Blocks   []map[string]any         `yaml:"blocks"`
}
