package views

import "github.com/pletka-io/pletka/pkg/domain"

// NodeKind identifies the role of a node in a weave view tree.
type NodeKind string

const (
	NodeProject    NodeKind = "project"
	NodeModel      NodeKind = "model"
	NodeCategory   NodeKind = "category"
	NodeCollection NodeKind = "collection"
	NodeField      NodeKind = "field"
	NodePath       NodeKind = "path"
)

// Tree is the ordered view tree consumed by APIs, visualizations, and
// weave-native generators.
type Tree struct {
	Root TreeNode `json:"root"`
}

// TreeNode is renderer-neutral but presentation-shaped: it preserves grouping,
// order, labels, slugs, and ontology context without owning entity data.
type TreeNode struct {
	Kind       NodeKind             `json:"kind"`
	ID         string               `json:"id,omitempty"`
	SemanticID string               `json:"semantic_id,omitempty"`
	SystemName string               `json:"system_name,omitempty"`
	Label      domain.Translations  `json:"label,omitempty"`
	Order      int                  `json:"order,omitempty"`
	Slug       string               `json:"slug,omitempty"`
	Path       string               `json:"path,omitempty"`
	Scope      *domain.PathElement  `json:"scope,omitempty"`
	// Anchor is the collection's anchor class — the class its fields
	// converge on (the last element of PathPrefix), as opposed to Scope,
	// the collection's declared, deliberately generic scope. Set on
	// collection nodes only.
	Anchor     *domain.PathElement  `json:"anchor,omitempty"`
	Element    *domain.PathElement  `json:"element,omitempty"`
	PathPrefix []domain.PathElement `json:"path_prefix,omitempty"`
	Children   []TreeNode           `json:"children,omitempty"`
}
