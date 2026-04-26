// Package server is Pletka's HTTP layer. It composes domain consumers
// (domain/weave, domain/ontology) with schema builders, link emitters,
// authentication, sessions, persistence wiring, and the embedded renderer
// bundle.
//
// Subpackages:
//
//   - server/widgets   — schema builders that produce widget JSON
//   - server/relations — link emission helpers (auth-aware hypermedia)
//   - server/handlers  — HTTP handlers
//   - server/middleware
//   - server/auth
//   - server/assets    — embedded static frontend bundle
//   - server/templates — thin shell .gohtml that mounts the renderer root
//   - server/cmd/pletka — the binary entry point
//
// server depends on domain/ and gitsync/. Nothing in domain/ or gitsync/
// may import server.
package server
