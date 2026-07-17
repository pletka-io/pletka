// Package attribution owns project credits — the author / funder /
// adopter rows that appear in the project overview "Credits" section
// and in the curator-facing settings panel.
//
// Module shape: **full slice** per ADR-0001. Owns:
//
//   - the weave_project_attributions table (migration 056)
//   - the Store interface + pgx + sqlc implementation
//   - the Service layer (kind validation, ProjectEdit gating, reorder)
//   - HTTP handlers for the settings CRUD endpoints
//   - the list + form schema builders
//   - the routes wiring
//
// Deliberately kept apart from:
//
//   - weave_memberships (authorisation — who can do what)
//   - weave_project_actors (institutional ownership — who owns the project)
//
// The settings handler in pkg/weave/settings/ dispatches sections via
// a hard-coded switch (a section registry was discussed but not yet
// built). Until that ships, the settings dispatcher calls into this
// slice's BuildSettingsListSchema / BuildSettingsFormSchema for the
// "attributions" section.
package attribution
