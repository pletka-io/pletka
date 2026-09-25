package generators

import (
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/views"
)

// Snapshot is the common generator input. Renderers should consume this
// resolved shape instead of querying stores or re-deriving path semantics.
type Snapshot struct {
	RootKind EntityKind `json:"root_kind"`

	Project    domain.Project     `json:"project"`
	Model      *domain.Model      `json:"model,omitempty"`
	Collection *domain.Collection `json:"collection,omitempty"`
	Field      *domain.Field      `json:"field,omitempty"`

	Fields     []FieldNode                      `json:"fields"`
	Tree       *views.Tree                      `json:"tree,omitempty"`
	Namespaces NamespaceSet                     `json:"namespaces"`
	Ontologies []domain.ResolvedOntologyVersion `json:"ontologies,omitempty"`
	Options    Options                          `json:"options"`
	Report     Report                           `json:"report,omitempty"`
}

// FieldNode is a resolved field plus its generator resource path.
type FieldNode struct {
	Field        domain.ResolvedField `json:"field"`
	Slug         string               `json:"slug"`
	RelativePath string               `json:"relative_path"`
	Scope        domain.PathElement   `json:"scope"`
	Path         []PathNode           `json:"path"`

	// SubPaths holds the path-node chains for legacy <br><br> subfield paths,
	// in source order. Empty for non-legacy fields. Renderers that emit one
	// graph pattern per path (X3ML, SPARQL) iterate Path followed by SubPaths,
	// all under the same field binding.
	SubPaths [][]PathNode `json:"sub_paths,omitempty"`

	// CollectionID is the ID of the Pletka Collection this field appears in
	// for the current model view, or empty when the field is directly on
	// the model (model-only category). One Pletka field can appear in more
	// than one collection across models; this is the per-view membership.
	CollectionID string `json:"collection_id,omitempty"`
	// CollectionSemanticID is the human-readable counterpart (e.g. LAC.1).
	CollectionSemanticID string `json:"collection_semantic_id,omitempty"`
	// CollectionSystemName is the slug-style identifier (e.g. "name").
	CollectionSystemName string `json:"collection_system_name,omitempty"`
	// CollectionName carries the multilingual collection label so
	// renderers can produce cards / sections without re-fetching the
	// Collection entity.
	CollectionName domain.Translations `json:"collection_name,omitempty"`

	// ConceptEnum is the sorted, de-duplicated set of member concept URIs from
	// the field's bound *sealed* concept lists. Populated only when a concept
	// enum reader is wired (host build) and at least one bound list is sealed;
	// generators (SHACL sh:in, later Arches) emit it as an exhaustive value
	// constraint (#3599). Empty for open/unbound fields.
	ConceptEnum []string `json:"concept_enum,omitempty"`

	// GraftPath is the chain of Collection-stub fields that anchored this
	// field's grafted position. Empty on an un-grafted field (the field
	// sits directly on the model or its declared collection). Non-empty on
	// a grafted copy: each element is the SystemName (slug) of one parent
	// stub, in graft order from root toward leaf. Example: a Name leaf
	// composed into Activity → Activity Part → TimeSpan → Statement → Name
	// has GraftPath = ["activity_part", "activity_part_timespan",
	// "activity_part_timespan_statement"].
	//
	// The arches renderer keys per-leaf UUIDs and aliases off
	// (SemanticID, GraftPath) so a logically-same field grafted at two
	// distinct anchors becomes two distinct Arches nodes (Arches'
	// node-belongs-to-one-nodegroup constraint).
	//
	// Stays nil/empty for now; populated when the snap-level collection
	// composition pass lands.
	GraftPath []string `json:"graft_path,omitempty"`
}

// PathDirection is explicit so renderers never infer direction from local-name
// conventions.
type PathDirection string

const (
	PathDirectionForward PathDirection = "forward"
)

// PathNode is one ontology path step as consumed by renderers.
type PathNode struct {
	Element      domain.PathElement `json:"element"`
	Index        int                `json:"index"`
	Direction    PathDirection      `json:"direction"`
	Role         PathRole           `json:"role"`
	Slug         string             `json:"slug"`
	RelativePath string             `json:"relative_path"`
}

type PathRole string

const (
	PathRoleScope    PathRole = "scope"
	PathRoleProperty PathRole = "property"
	PathRoleClass    PathRole = "class"
	PathRoleLiteral  PathRole = "literal"
	PathRoleTerminal PathRole = "terminal"
)
