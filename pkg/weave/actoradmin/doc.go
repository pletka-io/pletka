// Package actoradmin implements the platform-actor administration vertical
// slice behind the /admin/actors console: actor search, role and status
// management, and org-membership adjustments. This package is a standard
// full slice: it owns its Store (store.go / store_postgres.go), Service,
// Handler, formschema, and routes, per .claude/rules/service-layer.md. The
// Service holds no *pgxpool.Pool — all data access goes through Store.
package actoradmin
