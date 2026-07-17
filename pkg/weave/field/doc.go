// Package field implements the Field vertical slice — read endpoints
// only at first landing. Write paths (Create, Update, Delete) follow
// in a separate commit because they need a co-located base override
// upsert (category_id and set_value moved off weave_fields in
// migration 027 and now live on weave_field_overrides entity_type=”).
// This package is a full slice: it owns its store, service, handler, and
// routes.
//
// Mounted at /projects/{projectID}/fields by pkg/weave/router.
//
// Routes registered (this commit):
//
//	GET /projects/{projectID}/fields                    → List
//	GET /projects/{projectID}/fields/api/{fieldID}      → Detail (JSON)
//	GET /projects/{projectID}/fields/{fieldID}/models   → ModelRefs
//	GET /projects/{projectID}/fields/{fieldID}/collections → CollectionRefs
//	GET /projects/{projectID}/fields/list-schema        → list view schema
//
// Form schema delivery for fields lives on the entityschema
// dispatcher (cross-entity dispatcher per ADR-0001) which delegates
// to BuildFormSchema in this package.
//
// Write paths live in the schema-driven weave slices.
package field
