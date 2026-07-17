# Schema-driven API

The backend emits one JSON schema per view: structure, data, capabilities, and
mutation URLs in a single response. Generic renderers — `FormRenderer`,
`ListManager`, `EntityListView` — read `schema.endpoint.url` and
`schema.capabilities.*` and act on what they find. They do not know the URL
scheme, the entity type, or the permission model; the schema tells them
everything they need.

## The absolute rule

**The frontend never constructs an API URL.** Every URL a component calls
comes from a schema field (`endpoint.url`, `capabilities.delete.url_template`,
`data_url`, …), never from string-concatenating a known prefix and an id.

Client-side substitution of `{id}`-style placeholders into a schema-provided
`url_template` is COMPLIANT — the schema owns the URL shape (this is how
`ListManager` and `EntityListView` consume capability templates). A violation
is a hardcoded or hand-assembled path in TypeScript.

The two previously-logged violators (`ExampleWorkspace.svelte`,
`ArchesFleet.svelte`) were brought in line by the 2026-07-07 compliance
sweep (see [Compliance debt](#compliance-debt)): their endpoint URLs now
arrive server-side — `EntityListSchema.editor.endpoints` for the example
workspace, a `data-prop-endpoints` island prop built in
`pkg/archesfleet/pages.go` for the fleet panel. New code that constructs a
URL is a regression, not a precedent to follow.

## The canonical error envelope

Every JSON error response in the weave API uses one shape, built by
`pkg/weave/apierror` (`pkg/weave/apierror/apierror.go:47-55`):

```json
{
  "code": "validation",
  "error": "validation error",
  "message": "Please correct the highlighted fields and try again.",
  "errors": { "field_name": ["message 1", "message 2"] }
}
```

- `error` — human summary, always present.
- `code` — the machine-readable branch key. The frontend should switch on
  `code`, not parse `error` text. The constant list *is* the contract:
  `bad_request`, `unauthorized`, `forbidden`, `not_found`, `conflict`,
  `validation`, `in_use`, `internal`, `method_not_allowed`, `unprocessable`.
- `message` — optional longer help text (the FormRenderer banner).
- `errors` — per-field messages for inline field rendering; only present on
  validation (422) responses.

Validation errors are always `422` via `apierror.Validation(fields)`. Full
constructor reference and the slice adapter-interface pattern
(`ValidationFielder`, `Conflicter`, `Forbidder`, `NotFounder`, `InUser`) are in
[`errors.md`](errors.md) — this page states the contract shape, that page
covers the package internals.

**`field_count` on a delete conflict is slice-specific, not generic.** Hand-built `in_use` 409 payloads carrying `field_count` (and `field_samples`) exist in the `collection`, `model`, and `projectontologyversion` handlers; still not part of `apierror.InUse` itself — slice-specific extension, not a guaranteed envelope field.

## Status codes

| Operation | Method | Success | Conflict | Validation |
|---|---|---|---|---|
| Create | POST | `201` + entity | `409` + envelope | `422` + envelope |
| Update | PUT | `200` + entity | `409` + envelope | `422` + envelope |
| Delete | DELETE | `204` (no body) | `409` + envelope (`in_use` when dependents exist) | — |
| Reorder | PATCH | `204` (no body) | — | `422` + envelope |

No response envelope on success. The entity is returned directly; schema
responses are self-describing. Verified against `pkg/weave/category/handler.go:119-266`
(`Create` → 201, `Update` → 200, `Delete` → 204, `Reorder` → 204) and holds
across the other full slices.

## Schema endpoint URLs are per-slice

There is no single central schema endpoint. Each full slice mounts its own
list-schema and form-schema routes under its own prefix
(`pkg/weave/category/routes.go:120-140`):

```
GET  /projects/{projectID}/categories/list-schema
GET  /projects/{projectID}/categories/form-schema
GET  /projects/{projectID}/categories/{id}/form-schema
```

The same shape repeats per entity (`fields`, `models`, `collections`, …) —
each slice owns its own schema builder and routes; there is no shared
dispatcher a new slice must plug into.

**Legacy delegator, not the real pattern:** `GET
/projects/{projectID}/form-schema/{entityType}` (`pkg/weave/entityschema/handler.go:86`)
still exists and still works, but it is a central switch-style delegator kept
for callers that haven't migrated to the per-slice URL, not the pattern to
copy for a new entity type. A new slice gets its own `/list-schema` and
`/form-schema` routes, not a `case` added to `entityschema`.

## Roles

Real role strings, in ascending order (`pkg/formschema/types.go:265-279`):

```
viewer < contributor < maintainer < owner < admin, superadmin
```

(`admin` and `superadmin` are the same level — `roleToLevel` maps both to 4.)

Two separate mechanisms operate on a `FormSchema`, and they are not the same
thing:

- **`FilterForRole(role)`** (`pkg/formschema/types.go:237-263`) drops fields
  below the role's level (`FieldDef.MinRole`) and, for `viewer`, nils out
  `Endpoint` entirely so there is nothing to submit to. It is called from
  exactly one place in the codebase — `entityschema/handler.go:308`, in the
  central form-schema delegator above. Per-slice handlers do not call it by
  default; a slice that wants role-based field filtering has to opt in
  explicitly.
- **`MakeReadOnly()`** (`pkg/formschema/types.go:285-299`) is a distinct,
  simpler mechanism: it clears `Endpoint` and `Delete` and marks every field
  `Readonly: true`, for turning a mutable form into a display-only view
  contract. It doesn't consult role at all — callers decide when to invoke
  it.

Neither mechanism is the enforcement point. **Security is server-side**:
mutation handlers check the actor's capability directly, e.g.
`snap.Can(auth.ProjectEdit, resource, nil)` (repeated across
`category/service.go:606`, `field/service.go:989`, `model/service.go:1119`,
`collection/service.go:1034`, and others). Schema filtering is a UX
convenience — it hides a button the user couldn't use anyway — never the only
check.

## Autocomplete exception

`POST /api/v1/ontology/autocomplete` (`pkg/weave/ontology/routes.go:168`) is a
deliberate, documented exception to "the frontend never constructs URLs." It
is a global, unscoped search endpoint — not tied to a project schema — and the
frontend is allowed to know this one URL directly, the same way it knows the
handful of schema entry-point URLs. The same carve-out covers the other
search/suggestion endpoints the frontend queries directly (model search
`/projects/{id}/models?…`, vocabulary resolve `/api/v2/vocabulary-entries/resolve`)
— specialized lookup endpoints, not entity mutations. Mutation and CRUD URLs are
never hardcoded; this carve-out is not grounds for widening.

## Aspirational — not built yet

These appear in older docs in the present tense. They don't exist in code
today:

- **Org-slug URL scheme + resolver middleware** (`/{org-slug}/{project-slug}/...`
  resolving to a project the way `/projects/{ULID}/...` does now). No
  resolver middleware exists; every URL in every schema is still
  `/projects/{ULID}/...`.
- **`check_url` real-time field validation.** The field-schema slot for a
  validation-check URL exists in the type, but no handler serves one and no
  schema builder populates it yet.

Treat both as direction, not as a contract a client can rely on.

## Compliance debt

Tracked here so the enforcement sweep has one place to check off against:

RESOLVED (2026-07-07 sweep):
- `frontend/src/lib/components/example/ExampleWorkspace.svelte` — the four
  hardcoded `/projects/${projectId}/examples...` fetches (list, get-one,
  create/edit form-schema) now consume `EntityListSchema.editor.endpoints`
  (`list_url`, `detail_url_template`, `form_schema_url`) populated by
  `pkg/formschema/example_entity_list.go` and threaded through
  `ExampleEntityListEditor.svelte`. The remaining model-search and
  vocabulary-resolve fetches fall under the autocomplete carve-out
  (specialized search endpoints the frontend is allowed to know).
- `frontend/src/lib/components/archesfleet/ArchesFleet.svelte` — all ten
  hardcoded `/admin/arches/api/...` fetches (including the initial
  `Promise.all` load and `refreshFleet`) now consume a server-provided
  `endpoints` island prop (`data-prop-endpoints`, JSON) built by
  `pkg/archesfleet/pages.go` next to the route table it mirrors; `{id}` /
  `{task_id}` placeholders are filled client-side (compliant template
  substitution).
- `pkg/weave/example/handler.go` — plain-text `http.Error` responses
  replaced with the apierror envelope (resolved earlier in the sweep).

RE-VERIFIED CLEAN: `LinkedOntologiesPanel.svelte` and `ConceptListEntriesTab.svelte`
use schema/capability-provided URLs (the latter's `updateTemplate.replace('{id}', ...)`
is the compliant template-substitution pattern); `IntegrationsPanel.svelte` mutations
use schema URLs too (its only construction is a navigation `href` at line 242, a UI nav link).

Fixing an item here means bringing the component or handler in line with this
page, not documenting a new exception.
