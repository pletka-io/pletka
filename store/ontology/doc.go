// Package ontology is the PostgreSQL implementation of
// domain/ontology.OntologyStore: pgx + sqlc-generated queries for
// ontology classes, properties, paths, version handling, and
// namespace bindings.
//
// Imports allowed: domain, domain/ontology, store/postgres,
// store/sqlcgen, third-party drivers. Never imports server, never
// imports gitsync, never imports store/weave.
package ontology
