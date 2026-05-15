// Package weave is the PostgreSQL store foundation for Pletka's semantic
// pattern data: pgx pool ownership, sqlc query access, and transaction
// helpers for Fields, Models, Collections, categories, projects, and the
// override chain.
//
// Imports allowed: domain, domain/weave, store/postgres,
// store/sqlcgen, third-party drivers. Never imports server, never
// imports gitsync.
package weave
