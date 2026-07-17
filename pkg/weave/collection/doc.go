// Package collection implements the Collection vertical slice —
// collection metadata CRUD plus the parent-owned override-editor
// endpoints. Mirror of pkg/weave/model with entity_type='collection'.
// This package is a full slice: it owns its store, service, handler, and
// routes.
//
// Per the override editor design (docs/reference/override-editor-design.md):
// the collection slice owns intrinsic definition (ui_name,
// description, ontology_scope, default_category_id, status,
// deprecated). Override editing for the collection's contextual
// composition (its fields and how they're grouped) flows through
// /{collectionID}/overrides which delegates to override.Service.
//
// Mounted at /projects/{projectID}/collections by pkg/weave/router.
//
// Routes:
//
//	GET    /                              → List
//	POST   /                              → Create (metadata only)
//	GET    /list-schema                   → list view schema
//	GET    /api/{collectionID}            → Detail
//	PUT    /{collectionID}                → Update (metadata only)
//	DELETE /{collectionID}                → Delete (in-use preflight)
//	GET    /{collectionID}/stats          → Collection + UsageReport
//	POST   /{collectionID}/deprecate      → soft-retire
//	POST   /{collectionID}/activate       → reverse soft-retire
//	GET    /{collectionID}/overrides      → flat list of collection-context overrides
//	PUT    /{collectionID}/overrides      → bulk replace via override.Service
//
// In-use rule: a collection is "in use" when any field's override-refs
// point at it as a value target. The collection's own override rows
// (entity_type='collection', entity_id=this) are content — they don't
// count and FK-cascade on collection delete.
package collection
