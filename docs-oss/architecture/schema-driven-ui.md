# Schema-driven UI

The frontend fetches one schema endpoint and renders whatever it gets back. That
response carries everything: structure, the data to display, which actions are
available, and the URLs to submit them to. The frontend never constructs an API
URL, never decides a permission, never branches on what kind of entity it is
looking at.

This is the single most important contract in the codebase. The normative version
is [`../principles.md`](../principles.md) §1–§6; this doc explains how it plays out.

## The schema is the complete contract

A schema response describes:

- layout — sections, columns, tabs
- fields or widgets, with their types and validation rules
- the data to render, or a data URL to refresh it from
- capabilities — what the current actor may do
- mutation URLs — where create / update / delete / reorder submit
- user-facing labels and messages

The frontend renders it. It does not rediscover any of the above.

## Two contract families

Not everything is a generic schema. There are two valid shapes:

| Use a **schema** for | Use an explicit **domain DTO** for |
|---|---|
| forms, lists, settings panes | structural editors |
| admin and overview pages | graphs and diagrams |
| lifecycle / action surfaces | detail views with substantial state |

Generic management surfaces are schema-driven. Rich readers and editors get a
purpose-built DTO. Do not force a structural editor into a generic form schema,
and do not smuggle entity-specific branches into a generic renderer.

## URLs flow from the server

The frontend may know one entry-point URL. Every follow-up URL — create, edit,
delete, reorder, stats, imports, version changes — comes from the schema or DTO.

This keeps route shape, scoping, slugs, permissions, and deployment differences
server-owned. When the URL scheme changes, schemas emit new URLs and the frontend
does not change. (Current scheme is `/projects/{ULID}/…`; the target
`/{org}/{project}/…` ships with organisations. The frontend is indifferent.)

**A form has one submit endpoint.** `FieldDef` carries per-field URLs, but
none of them is a per-field submit override:

- `options_url` is a **GET**. It fetches that field's choices (optionally
  with `{token}` substitution from other field values, via `depends_on`),
  and says nothing about where the form submits.
- `create_url` is a **POST**, but only for one shape: inline-create a new
  *referenced entity* without leaving the form. The widget renders a
  "+ Create new" button and a free-text name box, requires `entity_type`,
  and posts a fixed body — `{type, project_id, name: {en}, ui_name: {en}}`.
  That is why every in-tree use points at `/api/v1/drafts`. It cannot carry
  a different body, and the entity it creates is not the form's own record.

**Known gap:** enabling a vocabulary for a project is a create
(`POST /projects/{id}/settings/vocabularies` with `{mount, lang}`), but the
vocabularies settings form's endpoint is the `PUT` that saves
`enforce_concept_lists` and `concept_namespace`. `create_url` cannot express
that target: `add_vocabulary_id` picks an *existing* service mount by name,
not a free-text new entity, and the body it needs is `{mount, lang}`. So the
field needs bespoke wiring to that `POST` — a control of its own, not a
`create_url`. Until that wiring exists, `add_vocabulary_id` and the owned
`vocabulary_ids` list are `readonly` display, so the form cannot take an
edit its one endpoint would discard. See
[`vocabulary-service-contract.md`](../reference/vocabulary-service-contract.md#vocabularies-are-project-owned)
for the full explanation.

## Capabilities are server-gated

If the actor cannot do something, the schema omits the capability or endpoint:

- no endpoint → no submit
- no delete capability → no delete button
- no reorder capability → no drag handle

Mutation endpoints still enforce permissions server-side — the omission is UX, not
security. Role-based filtering happens in the schema builder, never in the frontend.

## Generic renderers stay generic

A renderer may branch on schema type, widget type, capability, or layout block. It
must **not** branch on application entity type. If code needs
`entity_type == "field"` inside a generic renderer, either add a real generic
capability or move the feature into a dedicated island/DTO.

Widget resolution itself is not a branch — it is a `Map` lookup
(`resolveWidget(field.widget)`) against a registry, not an `{:else if}` chain.
See [`svelte-islands.md`](svelte-islands.md) for the registry mechanism and the
current widget inventory.

## Response conventions

Mutations return the entity directly — no envelope.

| Operation | Success | Conflict | Validation error |
|---|---|---|---|
| Create | `201` + entity | `409` + `{"error": …}` | `422` + `{"errors": {…}}` |
| Update | `200` + entity | `409` + `{"error": …}` | `422` + `{"errors": {…}}` |
| Delete | `204` (no body) | `409` + `{"error": …, "field_count": N}` | — |
| Reorder | `204` (no body) | — | `422` + `{"errors": {…}}` |

Validation errors always use `{"errors": {"field": ["msg"]}}` so the FormRenderer
can show inline per-field errors. The full error shape is in
[`errors.md`](errors.md).

## Contract drift is a bug

Schema field names, request structs, validation-error keys, and the TypeScript
types must agree. If a field is `ui_name` in the schema, it is `ui_name` in the
request body and in the error keys.

`make typegen` (tygo) has a narrower scope than that name suggests: it only
regenerates `frontend/src/lib/types/path-types.ts` from `pkg/domain` (see
`tygo.yaml`). `form-schema.ts`, `list-schema.ts`, and the rest of the
formschema TypeScript types are **hand-maintained mirrors** of the Go structs
in `pkg/formschema/` — when a Go shape changes, update the mirror by hand. Any
exception is documented and tested.
