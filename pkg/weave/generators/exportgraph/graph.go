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
}

// LiteralBinding records a terminal literal/value field on a resource node.
type LiteralBinding struct {
	Predicate domain.PathElement `json:"predicate"`
	Field     FieldBinding       `json:"field"`
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
