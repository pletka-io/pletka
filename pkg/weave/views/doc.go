// Package views defines the renderer-neutral view-tree types (Tree,
// TreeNode) that present grouping, order, labels, and ontology context for
// APIs, visualizations, and weave-native generators without owning entity
// data. This package is a shared infrastructure slice: it owns no store or
// table and is imported directly wherever a slice needs the shared tree
// shape.
package views
