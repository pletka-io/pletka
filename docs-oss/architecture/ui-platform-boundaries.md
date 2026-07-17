# UI Platform Boundaries

This document defines the two UI platforms in Pletka and the boundary between them.

The goal is to prevent architectural drift.

## Summary

Pletka has two distinct frontend/backend composition models:

1. `formschema`
2. `detailview`

They are both Svelte-based and API-driven, but they solve different problems and must not be merged by accident.

## Platform 1: `formschema`

`formschema` is the generic schema-driven UI platform.

It is used for:

- CRUD forms
- list/browse pages
- settings panes
- overview-style page composition
- generic create/edit/delete flows

Its backend contracts are the shared schema types under `pkg/formschema`, including:

- `FormSchema`
- `ListSchema`
- `EntityListSchema`
- `ProjectPageSchema`
- `CompositePaneSchema`

Its frontend renderers are generic interpreters of those contracts.

Current examples:

- `entity-form`
- `list-manager`
- `entity-list`
- `project-settings`
- `project-detail`

## Platform 2: `detailview`

`detailview` is the dedicated detail-page and structural-editor platform.

It is used for:

- entity detail pages
- widget-composed read views
- rich structural editing
- override-heavy editing
- model/collection/pattern-oriented workflows

Its backend contract is not `formschema`. It is its own API-driven detail contract.

Its frontend is widget-based:

- the API decides what is shown
- the frontend renders reusable detail widgets
- richer state and editor behavior are expected

This platform is the correct home for structural entities that outgrow generic form/list rendering.

Current example:

- `entity-view`

## Why The Split Exists

`formschema` is good at declarative generic UI:

- forms
- tables/lists
- panes
- capabilities
- lightweight page composition

`detailview` is better for richer, stateful surfaces:

- nested sections
- structural reuse
- pattern/override inspection
- diagram/statistics/detail widgets
- entity-specific editing flows

Trying to force `detailview` behavior into `formschema` will make the generic schema system harder to reason about and harder to maintain.

## Naming Rule

Use these names consistently:

- `formschema` for the generic schema-driven platform
- `detailview` for the API-driven entity detail/editor platform

Avoid vague hybrid labels like:

- "special schema"
- "custom page schema"
- "entity-view but basically formschema"

If a surface belongs to `detailview`, call it `detailview`.

## Package Direction

The current package organization is:

- generic schema platform:
  - `pkg/formschema`
  - schema-driven wrapper pages under `pkg/weave/templates` and `pkg/weave/pages`

- detail platform (implemented):
  - backend: `pkg/weave/detailview/` (handler + contract types)
  - frontend: `frontend/src/lib/detailview/...` (Svelte components and state)

The boundary between the platforms is fixed and the detailview platform is now active.

## Migration Rule

When adding or migrating UI, decide first which platform it belongs to.

Use `formschema` when the page can be expressed as:

- a generic form
- a generic list
- a settings pane
- a tab/page composed from existing schema blocks

Use `detailview` when the page needs:

- widget-based detail rendering
- structural editing
- model/collection/pattern-specific UX
- richer interaction than a generic schema renderer should own

## Current Classification

Belongs to `formschema`:

- `project-detail`
- `project-settings`
- `entity-list`
- `list-manager`
- `entity-form`

Belongs to `detailview`:

- `entity-view`

Out of scope for migration into either generic wrapper path in its current form:

- `model-editor`

`model-editor` should not be migrated into the new schema-driven wrapper system. If its responsibilities remain needed, they should be absorbed into the `detailview` platform rather than revived as a separate ad hoc surface.

## Hard Rule

Do not add new bespoke entity detail APIs outside `detailview`.

Do not force structural editors into `formschema`.

Do not treat `entity-view` as a temporary exception. Treat it as a first-class platform surface with its own contract and widgets.

## Legacy Compatibility Names

Today the running system still uses the legacy compatibility name `entity-view`
in a few places:

- routes: `/projects/{projectID}/entity-view/{entityType}/{entityID}`
- stats routes: `/projects/{projectID}/entity-view/{entityType}/{entityID}/stats`
- frontend island name: `entity-view`
- some current filenames and comments

Those names are compatibility names, not architecture names.

When discussing the platform, use `detailview`.

When adding new code, prefer the `detailview` package/namespace even if the
external route or island name has not yet been renamed.
