// Package release provides the backend foundation for first-class weave
// project releases. The initial cut is intentionally narrow: release
// metadata listing/lookup plus the archive-schema groundwork in
// migrations. Snapshot creation and version-aware project reads land in
// follow-up slices. This package is a full slice: it owns its store,
// service, handler, and routes; it also holds a *pgxpool.Pool directly for
// transaction-owning writes, tracked as pool-holder debt in the compliance
// backlog.
package release
