package generators

// Format identifies a generator output format.
type Format string

const (
	FormatCSV           Format = "csv"
	FormatRDF           Format = "rdf"
	FormatTurtle        Format = "turtle"
	FormatJSONLD        Format = "jsonld"
	FormatSHACL         Format = "shacl"
	FormatSPARQL        Format = "sparql"
	FormatX3ML          Format = "x3ml"   // form A: standalone mapping document
	FormatX3MLB         Format = "x3ml-b" // form B: domain split into collection-spine + field-tail mappings
	FormatResearchSpace Format = "researchspace"
	FormatMermaid       Format = "mermaid"
	FormatExportGraph   Format = "exportgraph"
	FormatCytoscape     Format = "cytoscape"
	FormatJSON          Format = "json"
	FormatArches        Format = "arches"
	// FormatSnapshot / FormatASCIITree are not render formats (the snapshot is
	// encoded directly and the ascii tree via WriteSnapshotASCII); they exist
	// as capability-gate keys so the /snapshot and /ascii-tree endpoints route
	// through the same derivative policy as every other generator.
	FormatSnapshot  Format = "snapshot"
	FormatASCIITree Format = "ascii-tree"
)

// EntityKind identifies the weave entity used as the export root.
type EntityKind string

const (
	EntityProject    EntityKind = "project"
	EntityModel      EntityKind = "model"
	EntityCollection EntityKind = "collection"
	EntityField      EntityKind = "field"
)
