// Package category implements the Category vertical slice: data store, service,
// HTTP handler, form-schema builders, and route registration. It is the
// reference template for splitting pkg/weave into per-entity slices. This
// package is a full slice: it owns its store, service, handler, and routes.
//
// The slice exposes its own Store interface (this file) so handlers and
// services can be wired against an in-memory mock for tests, or against a
// pgx/sqlc-backed implementation in production. Cross-slice consumers (e.g.,
// a future field.Service that needs read-only category access) import this
// package's Store interface directly rather than going through a central
// aggregate.
package category
