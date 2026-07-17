// Package gitrestoreadmin implements the admin vertical slice for restoring
// a project from a git-materialized snapshot: it previews and runs restore
// jobs that hydrate a target project's shell, entities, overrides, and
// provenance via pkg/service/gitmaterializer. This package is a full slice:
// it owns its store, service, handler, and routes.
package gitrestoreadmin
