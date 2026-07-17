# Verification

Use this document to audit an existing implementation or verify new work before
merge.

## Feature Classification

Answer these before implementation:

- Is this schema-driven management UI or a domain DTO reader?
- Is the backend package a full slice or handler-only module?
- Which slice owns each entity schema?
- Which frontend renderer or island should render it?
- Which permissions/capabilities gate the actions?
- Which scope owns the URLs?

## Slice Audit

For a full slice:

- `routes.go` exposes one canonical mount entry point.
- `handler.go` does not import pgxpool or sqlcgen.
- `handler.go` writes JSON errors through the shared error contract.
- `service.go` owns validation and permission checks.
- `store.go` is an interface.
- `store_postgres.go` owns database implementation details.
- `formschema.go` owns the entity's schema builders.
- sibling slice dependencies are narrow interfaces, not direct imports.

For a handler-only module:

- no store exists unless the module truly owns data
- composed entity schemas delegate to owning slices
- page/read DTO assembly is explicit
- the package documents why it is handler-only

## Schema Contract Audit

- Schema field names match request JSON keys.
- Validation errors use schema field names.
- Mutation URLs come from schema endpoints or capabilities.
- Capabilities are omitted when unavailable.
- Role checks are not duplicated in TypeScript.
- Generic renderers do not branch on entity type.
- Custom widgets are registered by manifest-declared widget name.
- Backend-emitted island/widget names use marker helpers, not raw string
  literals.
- Unknown widget behavior is visible.
- Read-only state is represented in the schema.

## Registry And Activation Audit

- Registry construction accepts host-provided open source and platform/private
  contributions.
- Registration is explicit app wiring, not hidden package side effects.
- Registered contributions are not treated as automatically active.
- Project or organization configuration is checked before exposing configured
  services.
- User permissions and capabilities are checked server-side before exposing
  actions.
- Paid entitlement, license, feature-flag, or plan checks live in host
  activation policy, not reusable core registry code.
- Frontend widgets and actions are selected from active schema/capability
  output; TypeScript does not duplicate entitlement policy.
- Disabled paid or unavailable features either disappear server-side or expose a
  clear disabled reason through the schema.

## Backend Tests

Recommended coverage:

- service validation tests
- permission/capability tests
- delete guard and usage-impact tests
- schema builder tests
- route tests for schema endpoints
- route tests for mutation error shapes
- store tests for non-trivial queries

Use the smallest useful test set for the risk. A single narrow slice does not
need exhaustive browser tests; a shared renderer or schema contract change does.

## Frontend Checks

Run when frontend/schema rendering changed:

- TypeScript/Svelte check command
- build command if bundle or imports changed
- frontend manifest conformance check
- browser smoke for the mounted page
- console-error check
- mobile/desktop screenshot check for layout-sensitive surfaces

Smoke checklist:

- schema endpoint returns expected JSON
- island mounts
- fields/lists/page sections render
- allowed actions appear
- forbidden actions are absent
- validation errors render inline
- mutation refreshes or redirects as specified

Manifest checklist:

- every island, widget, global entry, and content widget is declared in a
  frontend manifest
- platform/private overrides use explicit `overrides: "core"`
- backend schema/page emitters use `frontendrefs.*(...)` markers for registry
  lookups
- raw string literals are not used for `IslandMount.Name`,
  `IslandMount.Dependencies`, `EntityListSchema.RowWidget`, `ViewMode.Widget`,
  or `EntityListEditor.Widget`
- exported form widget constants have matching `frontendrefs.FormWidget(...)`
  markers
- `go test ./pkg/frontendmanifest` passes

## Reuse Audit

Use this to find design weaknesses that limit extraction:

- Does the slice hard-code Pletka route prefixes?
- Does it assume project scope when it could accept a generic scope?
- Does a generic frontend component know domain entity names?
- Does a schema builder query the database?
- Does the slice depend on a large app-level service instead of a narrow
  interface?
- Does it require a custom widget that is really generic and should be promoted?
- Does it return raw persistence rows anywhere?
- Does a page have both a schema contract and a competing DTO contract?
- Are TypeScript types hand-maintained without a drift check?
- Are migration/setup requirements undocumented?

## Agent Completion Checklist

Before ending an implementation task:

- list files changed
- list schema endpoints added or changed
- list mutation endpoints added or changed
- state which contract family was used
- state which tests/checks ran
- include `go test ./pkg/frontendmanifest` when frontend manifest references
  changed
- state any skipped checks
- mention known reuse limitations discovered
