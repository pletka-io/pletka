// Package settings owns the project-settings-v2 surface that the Svelte
// settings island consumes. This package is a full slice: it owns its
// store, handler, and routes (business logic lives inline in the handler
// rather than a separate service.go).
//
// Mount root: /projects/{projectID}/settings. All settings endpoints live
// under this single prefix to keep the URL space self-describing — when
// you see "settings" in a URL you know the settings slice owns it.
//
// Routes registered (relative to /projects/{projectID}/settings):
//
//	GET  /schema                      → page schema with capability-filtered sections
//	GET  /form-schema/{section}       → form schema for a single section
//	GET  /pane-schema/{section}       → composite pane schema (ontology only today)
//	PUT  /general                     → update UIName / Description
//	PUT  /ontology                    → update ParentProjectID with cycle detection
//
// The settings page shell (GET /projects/{pid}/settings) is separate from
// this API slice; this package owns the schema/data endpoints below it.
//
// Schema builders live in pkg/formschema (BuildSettingsSchema,
// BuildGeneralSettingsSchema, BuildOntologySettingsSchema, etc.). The
// slice composes them with the project store + auth snapshot; no schema
// logic lives in the slice itself.
//
// Permission model: snap.Can(auth.ProjectRead, ...) gates reads;
// snap.Can(auth.ProjectEdit, ...) gates writes. The cycle-detection
// helper for parent-project linking lives at cycle.go.
//
// Out of scope: deprecated gohtml settings pages such as
// /projects/{pid}/settings/access and /settings/repository.
package settings
