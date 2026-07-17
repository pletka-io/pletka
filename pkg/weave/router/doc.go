// Package router mounts every weave vertical slice's routes on a chi
// router. Slices should expose narrow Host contracts; pkg/app assembles those
// hosts and passes them through Options. Shared project middleware and global
// error-page rendering are passed as separate narrow hosts. This package is a
// shared infrastructure slice: it owns no store of its own and is the single
// place every full slice and handler-only module is imported directly for
// mounting.
//
// Mount is intentionally agnostic about the parent router. It can be
// invoked from:
//
//   - cmd/serve.go directly on the root chi.Mux
//   - a future standalone weave server with its own chi.Mux
//
// Each slice mounts under its full project-scoped path
// (/projects/{projectID}/<sub>) via chi.Mount with a private sub-mux, so
// it doesn't collide with any other Route() registered on the same
// parent.
package router
