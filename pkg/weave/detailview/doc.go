// Package detailview is the first-class home of Pletka's API-driven
// detail-page and structural-editor platform. This package is a
// handler-only module: it composes widget-driven detail-page schemas from
// other slices' data and owns no store.
//
// This platform is distinct from pkg/formschema:
//
//   - formschema: generic schema-driven CRUD, lists, settings, and overview pages
//   - detailview: widget-composed entity detail pages and richer structural editing
//
// The current runtime still uses the legacy "entity-view" route and frontend
// island names for compatibility:
//
//   - routes: /projects/{projectID}/entity-view/{entityType}/{entityID}
//   - island: entity-view
//
// Those names should be treated as compatibility names, not as the conceptual
// platform name. New backend/frontend work for this surface should target the
// detailview concept and package area, even before route renames happen.
package detailview
