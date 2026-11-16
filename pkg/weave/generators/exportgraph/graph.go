package exportgraph

import (
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/generators"
	"github.com/pletka-io/pletka/pkg/weave/views"
)

// Graph is the renderer-neutral export graph for a model, collection, or field.
//
// Resource identity is structural by default: the same predicate/class branch
// under the same parent is one entity. PathElement.InstanceID is retained as an
// optional explicit identity hint, not as required input for correctness.
type Graph struct {
	Source *generators.Snapshot `json:"-"`

	RootKey string          `json:"root_key"`
	Nodes   []*ResourceNode `json:"nodes"`
	Groups  []*VisualGroup  `json:"groups,omitempty"`
	Fields  []FieldBinding  `json:"fields,omitempty"`
}

// ResourceNode is an RDF-like resource in the export graph template.
type ResourceNode struct {
	Source ResourceSource `json:"-"`

	Key             string              `json:"key"`
	Class           domain.PathElement  `json:"class"`
	IdentitySource  IdentitySource      `json:"identity_source"`
	InstanceIDs     []string            `json:"instance_ids,omitempty"`
	Edges           []ResourceEdge      `json:"edges,omitempty"`
	Literals        []LiteralBinding    `json:"literals,omitempty"`
	Fields          []FieldBinding      `json:"fields,omitempty"`
	Groups          []GroupBinding      `json:"groups,omitempty"`
	SourcePaths     []SourcePathBinding `json:"source_paths,omitempty"`
	AdditionalTypes []domain.TypeRef    `json:"additional_types,omitempty"`
	// Collection is set on the anchor node of a placed collection.
	Collection *CollectionAnchor `json:"collection,omitempty"`
}

// CollectionAnchor identifies the placed collection a node anchors.
type CollectionAnchor struct {
	SemanticID string              `json:"semantic_id"`
	Label      domain.Translations `json:"label,omitempty"`
	// Address is "<root semantic id>/<collection semantic id>", with
	// "/<anchor path node id>" appended when the same collection is placed
	// more than once. Renderers add their own prefix.
	Address string `json:"address"`
}

type ResourceSource struct {
	Fields []*generators.FieldNode `json:"-"`
	Paths  []*generators.PathNode  `json:"-"`
	Groups []*views.TreeNode       `json:"-"`
}

type IdentitySource string

const (
	IdentityStructural IdentitySource = "structural"
	IdentityInstanceID IdentitySource = "instance_id"
)

// ResourceEdge connects two ResourceNodes through an ontology property.
type ResourceEdge struct {
	Predicate domain.PathElement `json:"predicate"`
	TargetKey string             `json:"target_key"`
	FieldIDs  []string           `json:"field_ids,omitempty"`

	// Min/Max count the step per parent node: the first ending field's
	// counts, the collection placement for an entry step (made stricter by
	// a field ending on it), or Min 1 when something below is required.
	Min int  `json:"min"`
	Max *int `json:"max,omitempty"`
	// Entry marks the step into a placed collection's anchor.
	Entry bool `json:"entry,omitempty"`
	// Endings are the fields whose path ends on this step, in field order.
	Endings []Ending `json:"endings,omitempty"`
}

// Ending is one field's own counts on the step its path ends on.
type Ending struct {
	FieldID string `json:"field_id"`
	Min     int    `json:"min"`
	Max     *int   `json:"max,omitempty"`
	// Field is the ending field itself, for readers that need its set value.
	// In memory only: a graph decoded from JSON has none.
	Field *generators.FieldNode `json:"-"`
}

// LiteralBinding records a terminal literal/value field on a resource node.
type LiteralBinding struct {
	Predicate domain.PathElement `json:"predicate"`
	// End is the leaf element: a datatype or literal; zero when the path
	// ends at the property.
	End   domain.PathElement `json:"end,omitzero"`
	Field FieldBinding       `json:"field"`
	// Min/Max are the field's own counts per parent node.
	Min int  `json:"min"`
	Max *int `json:"max,omitempty"`
}

// FieldBinding keeps the form-field identity attached to the graph node that
// receives the field value.
type FieldBinding struct {
	Source *generators.FieldNode `json:"-"`

	ID           string              `json:"id"`
	SemanticID   string              `json:"semantic_id,omitempty"`
	SystemName   string              `json:"system_name,omitempty"`
	Label        domain.Translations `json:"label,omitempty"`
	RelativePath string              `json:"relative_path,omitempty"`
	NodeKey      string              `json:"node_key,omitempty"`
	GroupKeys    []string            `json:"group_keys,omitempty"`
	// SetValue and ExpectedValueType are the field's own, for readers of
	// the JSON document, which does not carry Source.
	SetValue          string `json:"set_value,omitempty"`
	ExpectedValueType string `json:"expected_value_type,omitempty"`
}

// VisualGroup is a presentation grouping from the model view tree. Categories
// and collections are not resource identity; they describe how users mentally
// and visually organize the graph/form.
type VisualGroup struct {
	Source *views.TreeNode `json:"-"`

	Key        string               `json:"key"`
	Kind       views.NodeKind       `json:"kind"`
	ID         string               `json:"id,omitempty"`
	SemanticID string               `json:"semantic_id,omitempty"`
	SystemName string               `json:"system_name,omitempty"`
	Label      domain.Translations  `json:"label,omitempty"`
	Order      int                  `json:"order,omitempty"`
	Path       string               `json:"path,omitempty"`
	ParentKey  string               `json:"parent_key,omitempty"`
	BranchKey  string               `json:"branch_key,omitempty"`
	PathPrefix []domain.PathElement `json:"path_prefix,omitempty"`
	Children   []string             `json:"children,omitempty"`
	FieldIDs   []string             `json:"field_ids,omitempty"`
}

type GroupBinding struct {
	Key  string         `json:"key"`
	Kind views.NodeKind `json:"kind"`
}

type SourcePathBinding struct {
	FieldID string `json:"field_id,omitempty"`
	Path    string `json:"path,omitempty"`
}
