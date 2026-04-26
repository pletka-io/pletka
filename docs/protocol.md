# Protocol (Schema Contract)

> **Status — draft / placeholder.** This file is the normative reference
> for the JSON contract between the Pletka server and any renderer. It is
> stubbed at v0.1.0-dev and will be filled in as widgets and link relations
> land. Each section below maps to a piece of the schema that needs to be
> formalised before the first real release.

## Scope

This document describes:

- The response **envelope** the server returns for any page.
- The **widget** types the renderer must support.
- The **link relations** (rels) the server may emit on widgets and pages.
- **Pagination, filtering, validation errors** conventions.
- **Version negotiation** between server and renderer.

It is the only public contract. The Go API surface in `server/widgets/` and
`server/relations/` is implementation; this document is what binds the two
sides together.

## Page envelope

```json
{
  "schemaVersion": "0.1.0",
  "title": "Example",
  "lang": "en",
  "links": [],
  "capabilities": { ... },
  "body": { "type": "...", ... },
  "meta": {}
}
```

Fields:

- `schemaVersion` — semver string. Renderers MUST accept any minor or patch
  version equal to or greater than the renderer's compiled-in baseline,
  ignoring unknown fields.
- `title` — page title for the document head.
- `lang` — BCP-47 locale code.
- `links` — page-scoped, open-ended hypermedia link relations: `self`,
  `parent`, breadcrumb chain, related entities, alternative views.
  See [`schema/relations/`](../schema/relations/) for the rel vocabulary.
- `capabilities` — typed, page-level mutation capabilities (e.g. a page
  may carry a typed `delete` capability for the resource it represents).
  Each capability is a nullable struct; non-nil = permitted.
- `body` — root widget. The renderer dispatches on `body.type`.
- `meta` — non-rendered metadata (current user, feature flags, etc.).

`links` and `capabilities` both honour the same invariant: presence
implies the user is permitted to act. They are complementary; see
[`authorization-model.md`](authorization-model.md) for which to pick.

## Widgets

Each widget has a stable `type` string. Concrete widget specs live under
[`schema/widgets/`](../schema/widgets/) and have at least one example
fixture under [`schema/examples/`](../schema/examples/).

Renderers MUST treat unknown widget types as a generic dispatcher
fallthrough; they MUST NOT crash when encountering a widget added in a
later schema minor.

## Capabilities (typed action grants)

Forms, lists, and other well-shaped action-bearing schemas embed a
typed `Capabilities` struct. Each field is a nullable pointer to a
typed cap whose shape carries everything the renderer needs to issue
the request (URL, method, payload schema). Non-nil = permitted.

This is the default for mutations. Concrete capability shapes are
documented per-schema (e.g. a `ListSchema` has a different capability
set than a `FormSchema`).

## Link relations (open-ended hypermedia)

For navigation, breadcrumbs, related entities, pagination, and any
relation whose set is not closed, use the generic `links` array. The
presence of a link is the permission grant — see
[`authorization-model.md`](authorization-model.md). Each relation has a
spec under [`schema/relations/`](../schema/relations/) describing meaning,
HTTP method, and request / response shape.

Common relations (initial set, to be expanded):

| rel       | method | description                                     |
|-----------|--------|-------------------------------------------------|
| `self`    | GET    | The current resource. Always present.           |
| `parent`  | GET    | Container resource (for breadcrumbs).           |
| `next-page`/`prev-page` | GET | Pagination cursors.                |
| `related` | GET    | Related entity (uses `title` for display).      |
| `share`   | GET    | Public-share URL.                                |

Mutations on well-shaped resources (edit / delete on a row in a list,
submit on a form) live in the typed `Capabilities`, not in `links`.

## Pagination

TBD — placeholder for cursor format and `next-page` / `prev-page` shape.

## Filtering

TBD — placeholder for filter query parameters and filter envelope.

## Validation errors

TBD — `422` body shape, format compatible with the `FormRenderer` widget.

## Version negotiation

TBD — request `Accept-Schema` header? Compatibility windows? Behaviour
when renderer is older than server?

## Conformance

A renderer is conformant if every fixture under
[`schema/examples/`](../schema/examples/) renders without error and
honours the link relations present in the fixture (i.e. shows what the
fixture says is showable, hides what is hidden by absence). Server-side,
every fixture must round-trip through the type definitions in
`server/widgets/` and `server/relations/`.
