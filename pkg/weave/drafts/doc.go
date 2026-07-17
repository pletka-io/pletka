// Package drafts owns POST /api/v1/drafts — the unified inline-create
// endpoint used by every "+ Create new" button across the schema-driven
// forms. Instead of each referenced entity (category, model, collection)
// having its own inline-create POST, every form posts here and the
// handler dispatches by req.Type. Permission, dedup, counter allocation,
// and ontology-coverage gating are shared. This package is a handler-only
// module: it dispatches inline-create requests into the target entity's
// store through the strangler-fig WeaveStore aggregate and owns no store
// of its own.
//
// Mount point: /api/v1/drafts (legacy URL preserved). Mounted directly on
// the parent router; no per-project sub-mux because the project ID arrives in
// the request body, not the path.
//
// Auth: callers must be authenticated and hold project-edit or
// model-create / collection-create capability for the target project,
// resolved via auth.AuthSnapshot in request context. No cross-slice
// reader interface needed — the slice talks to deps.Weave (the
// strangler-fig aggregate) directly for category / model / collection
// stores until those slices fully cover the surface.
//
// The slice does not yet have its own service layer — the handler
// directly composes the WeaveStore reads + writes inside Create. Split
// out a Service when validation/business rules grow beyond the current
// inline checks.
package drafts
