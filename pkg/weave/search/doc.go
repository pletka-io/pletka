// Package search serves cross-entity search (fields, models, collections,
// and their inheritance/path context) for a project, running ad-hoc sqlc
// queries against the pool directly rather than through any single entity
// slice's Store. This package is a handler-only module: it composes search
// results across entity types and owns no store of its own.
package search
