# API Design — Design Contract

Read `docs-oss/architecture/schema-driven-api.md` before implementing or modifying any API endpoint.

## Core Principle: Schema Is the Complete Contract

The backend emits one JSON schema per view: structure, data, capabilities, and mutation URLs in one response. Generic renderers (`FormRenderer`, `ListManager`, `EntityListView`) read `schema.endpoint.url` and `schema.capabilities.*` and act on what they find — they never know the URL scheme, the entity type, or the permission model.

## The Absolute Rule

**The frontend never constructs an API URL.** Every URL comes from a schema field (`endpoint.url`, `capabilities.delete.url_template`, `data_url`, …) — never from string-concatenating a known prefix and an id.

Client-side substitution of `{id}`-style placeholders into a schema-provided `url_template` is COMPLIANT — this is the canonical mechanism `ListManager`/`EntityListView` use to consume capability templates. A violation is a hand-assembled path in TypeScript.

**Confirmed compliance debt** (logged, not precedent for a third): `ExampleWorkspace.svelte:131`, `ArchesFleet.svelte` (7 hardcoded fetches), `pkg/weave/example/handler.go` (plain-text `http.Error`, breaks the envelope). Full list: schema-driven-api.md § Compliance debt.

## The Canonical Error Envelope

```json
{"code": "validation", "error": "validation error", "message": "...", "errors": {"field_name": ["message 1"]}}
```

- `code` — machine-readable branch key; frontend switches on this, not `error` text. Constants: `bad_request`, `unauthorized`, `forbidden`, `not_found`, `conflict`, `validation`, `in_use`, `internal`, `method_not_allowed`, `unprocessable`.
- `error` — human summary, always present. `message` — optional longer help text. `errors` — per-field messages, only on `422`.
- `field_count` on a delete conflict is slice-specific (`collection`, `model`, and `projectontologyversion` handlers add it) — don't assume every `in_use` 409 has one.

## Status Codes

| Operation | Method | Success | Conflict | Validation |
|---|---|---|---|---|
| Create | POST | `201` + entity | `409` + envelope | `422` + envelope |
| Update | PUT | `200` + entity | `409` + envelope | `422` + envelope |
| Delete | DELETE | `204` (no body) | `409` + envelope (`in_use` when dependents exist) | — |
| Reorder | PATCH | `204` (no body) | — | `422` + envelope |

No response envelope on success — the entity is returned directly; schema responses are self-describing.

## Schema Endpoints Are Per-Slice

No central schema endpoint. Each full slice mounts its own routes under its own prefix, e.g. `GET /projects/{projectID}/categories/{list,form}-schema`. The legacy `GET /projects/{projectID}/form-schema/{entityType}` delegator (`pkg/weave/entityschema`) still works but is not the pattern to copy — a new slice gets its own routes, not a `case` added to the dispatcher.

## Roles

```
viewer < contributor < maintainer < owner < admin, superadmin
```

(`admin`/`superadmin` are the same level.) `FilterForRole(role)` drops fields below `MinRole` and, for `viewer`, nils `Endpoint`; it's called only from the legacy central delegator, not by default in per-slice handlers. `MakeReadOnly()` is a separate mechanism (clears `Endpoint`/`Delete`, marks fields readonly) and doesn't consult role. **Security is server-side** — mutation handlers check the actor's capability directly (e.g. `snap.Can(auth.ProjectEdit, resource, nil)`); schema filtering is UX convenience only, never the sole check.

## Autocomplete Exception

`POST /api/v1/ontology/autocomplete` is a deliberate exception: a global, unscoped search endpoint the frontend is allowed to know directly, the same way it knows schema entry-point URLs. This is the only other sanctioned hardcoded API URL — not grounds for hardcoding more.

## Constraints

- Never construct mutation URLs in the frontend — read them from the schema (template substitution is fine)
- Never add entity-specific error handling in TypeScript — use the standard envelope
- Always use proper HTTP status codes (201/200/204 success; 409/422 errors)
- The org-slug URL scheme and `check_url` real-time validation are aspirational — not built; every URL today is `/projects/{ULID}/...`
