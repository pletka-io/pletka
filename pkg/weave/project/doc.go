// Package project implements the Project vertical slice. This package is a
// full slice: it owns its store, service, handler, and routes.
//
// Mounted under /projects via direct routes and chi.Mount on individual
// sub-paths so project data/schema/mutation endpoints remain explicit.
//
// Routes registered:
//
//	GET /projects/api                          → list (paginated, filterable)
//	GET /projects/api/{projectID}              → detail JSON
//	GET /projects/data                         → list+stats payload (entity-list)
//	GET /projects/entity-list-schema           → entity-list schema
//	GET /projects/filters/institutions         → filter options
//	GET /projects/check-prefix                 → IDPrefix availability check
//	GET /projects/form-schema/project          → project form schema
//	GET /projects/{projectID}/inheritance-tree → ancestor walk
//
// The slice's Service is the reader interface other slices already use
// via deps.Weave.Projects() (project-ontology-version's ProjectReader,
// future Field/Model/Collection cross-slice lookups). Once the legacy
// WeaveStore aggregate goes away, downstream slices switch from
// deps.Weave.Projects() to deps.Project (a future deps field) without
// changing semantics.
package project
