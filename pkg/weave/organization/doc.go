// Package organization owns the organization domain slice. This package is
// a full slice: it owns its store, service, handler, and routes; it also
// holds a *pgxpool.Pool directly for transaction-owning writes, tracked as
// pool-holder debt in the compliance backlog.
//
// Canonical storage:
//   - weave_actors rows where type = "organization"
//
// This slice owns:
//   - self-service organization creation
//   - organization browse data/schema
//   - organization general settings schema + update
//   - org lookup by slug for workspace pages
package organization
