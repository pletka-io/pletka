package generators

// SlugPolicy controls how resource slugs are produced.
type SlugPolicy string

const (
	// SlugPolicyDefault uses stable semantic IDs when present, falling back to
	// system names or IDs, with collision suffixes allocated per sibling set.
	SlugPolicyDefault SlugPolicy = "default"
)

type RDFNodeMode string

const (
	RDFNodeModeResource  RDFNodeMode = "resource"
	RDFNodeModeBlankNode RDFNodeMode = "blank_node"
)

type MermaidMode string

const (
	MermaidModeOntology MermaidMode = "ontology"
	MermaidModeInstance MermaidMode = "instance"
)

// Options carries generator-independent snapshot options.
type Options struct {
	Lang                       string
	IncludeInheritedOntologies bool
	SlugPolicy                 SlugPolicy
	LegacyInverseMode          LegacyInverseMode
	BasePrefix                 string
	BaseURI                    string
	RDFNodeMode                RDFNodeMode
	MermaidMode                MermaidMode

	// SPARQLLimit caps the result count emitted by the SPARQL renderer.
	// Zero means "no LIMIT clause"; legacy Python hardcoded 100.
	SPARQLLimit int
	// SPARQLCount swaps the SELECT projection for COUNT(?value) — the
	// caller wants result cardinality, not the rows themselves.
	SPARQLCount bool

	// X3MLTargets supplies the project's linked ontologies so the X3ML
	// renderer can emit one <target> block per ontology.
	// Empty means the renderer falls back to a single CIDOC-CRM target.
	X3MLTargets []X3MLTarget

	// ComposeCollections enables the snap-level collection composition
	// pass: every Collection-stub field whose path ends at a CIDOC class
	// X gets the children of every collection that anchors at X grafted
	// under it, recursively, with the collection's SharedPathPrefix
	// stripped. Off by default during the staged rollout so existing
	// renderers stay byte-stable; flip on per call to verify.
	ComposeCollections bool

	// ComposeMaxDepth caps graft recursion. Zero falls back to the
	// built-in default. The per-(target, anchor) guard already prevents
	// re-grafting the same collection on the same anchor twice; this is
	// the backstop for pathological data.
	ComposeMaxDepth int
}

// X3MLTarget describes one ontology the X3ML document maps onto — used
// to build the <info>/<target> blocks. SchemaFile is the bundled RDFS
// file name; empty when no raw schema file is available for the
// ontology (the <target_schema schema_file=...> attr is then omitted).
type X3MLTarget struct {
	Prefix     string
	Namespace  string
	Label      string
	Version    string
	SchemaFile string
}

// LegacyInverseMode controls handling for legacy Airtable paths that encode
// inverse traversal as "^P..." local names.
type LegacyInverseMode string

const (
	LegacyInverseReject    LegacyInverseMode = "reject"
	LegacyInverseNormalize LegacyInverseMode = "normalize"
	LegacyInverseSkip      LegacyInverseMode = "skip"
)

// Report carries non-fatal snapshot construction diagnostics.
type Report struct {
	Warnings []Diagnostic `json:"warnings,omitempty"`
	Errors   []Diagnostic `json:"errors,omitempty"`
}

// Diagnostic identifies a generator issue in a machine-readable form.
type Diagnostic struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	FieldID   string `json:"field_id,omitempty"`
	PathIndex int    `json:"path_index,omitempty"`
}

func normalizeOptions(opts Options) Options {
	if opts.Lang == "" {
		opts.Lang = "en"
	}
	if opts.SlugPolicy == "" {
		opts.SlugPolicy = SlugPolicyDefault
	}
	if opts.LegacyInverseMode == "" {
		opts.LegacyInverseMode = LegacyInverseReject
	}
	if opts.BasePrefix == "" {
		opts.BasePrefix = "ex"
	}
	if opts.RDFNodeMode == "" {
		opts.RDFNodeMode = RDFNodeModeBlankNode
	}
	if opts.MermaidMode == "" {
		opts.MermaidMode = MermaidModeOntology
	}
	if opts.ComposeMaxDepth <= 0 {
		opts.ComposeMaxDepth = DefaultComposeMaxDepth
	}
	return opts
}

// DefaultComposeMaxDepth caps composition recursion when Options leaves
// it unset. Real-world OGEE Activity's deepest observed chain is 5
// grafts (Activity → Activity Part → TimeSpan → Statement → Name →
// Name Part); 10 is the agreed safety net (see plan §9). The intended
// stopping condition is structural (no Collection-stub fields left to
// graft), not the cap — the cap exists only to refuse runaway data.
const DefaultComposeMaxDepth = 10
