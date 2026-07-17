// Package model implements the Model vertical slice — model metadata
// CRUD plus the parent-owned override-editor endpoints. This package is a
// full slice: it owns its store, service, handler, and routes.
//
// Per the override editor design (docs/reference/override-editor-design.md):
//   - Model owns intrinsic definition (ui_name, description, ontology_scope,
//     status, deprecated). NO override editing on the model row itself.
//   - Model owns the HTTP routes for editing the model's CONTEXTUAL
//     overrides (the fields and collections used in this model). Those
//     routes call into override.Service.SaveForEntity for the writes.
//
// Mounted at /projects/{projectID}/models by pkg/weave/router.
//
// Routes registered:
//
//	GET    /                          → List
//	POST   /                          → Create (model metadata only)
//	GET    /list-schema               → list view schema
//	GET    /api/{modelID}             → Detail (JSON)
//	PUT    /{modelID}                 → Update (metadata only)
//	DELETE /{modelID}                 → Delete (in-use preflight; 409 with usage)
//	GET    /{modelID}/stats           → Model + UsageReport
//	POST   /{modelID}/deprecate       → soft-retire
//	POST   /{modelID}/activate        → reverse soft-retire
//	GET    /{modelID}/overrides       → flat list of model's override rows
//	PUT    /{modelID}/overrides       → bulk replace via override.Service
//
// In-use rule: a model is "in use" when any field's override-refs point
// at it as a value target (ref_type IN ('resource_model','collection_model')).
// The model's own overrides (entity_type='model', entity_id=this) are
// the model's content — they don't count.
//
// The override editor's nested category→items payload is a frontend
// concern; the slice serves a flat list of override rows and lets the
// frontend group. Server-side grouping can land later if needed (the
// shape is documented in the override editor design doc).
package model
