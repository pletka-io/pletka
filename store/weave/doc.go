// Package weave is the PostgreSQL implementation of
// domain/weave.WeaveStore: pgx + sqlc-generated query layer for
// Fields, Models, Collections, categories, projects, and the
// override chain.
//
// Imports allowed: domain, domain/weave, store/postgres,
// store/sqlcgen, third-party drivers. Never imports server, never
// imports gitsync.
package weave
