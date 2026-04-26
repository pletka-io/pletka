# Authorization Model

Pletka uses **hypermedia-driven authorization**. The principle is one
sentence; the rest of this document is consequences and enforcement.

## Principle

> The presence of an action in the response **is** the user's permission
> to take it. The renderer renders what it received and never decides.

A user without edit permission receives a response with no edit
capability and no `edit` link. The renderer displays no edit button —
not because it chose to hide it, but because there is nothing to render.

## Two shapes, one invariant

Pletka encodes the "presence iff permitted" rule in two shapes. Both
honour the same invariant; pick whichever fits the action.

### Shape 1 — Typed capabilities (the default)

This is the formschema pattern (`pkg/formschema` in legacy zellij,
`server/widgets/` going forward). Forms, lists, and other well-shaped
action-bearing schemas embed a typed nullable capability struct:

```go
type Capabilities struct {
    Reorder      *ReorderCap      `json:"reorder,omitempty"`
    Create       *CreateCap       `json:"create,omitempty"`
    Edit         *EditCap         `json:"edit,omitempty"`
    Delete       *DeleteCap       `json:"delete,omitempty"`
    InlineRename *InlineRenameCap `json:"inline_rename,omitempty"`
    Stats        *StatsCap        `json:"stats,omitempty"`
}
```

A non-nil pointer is the permission grant. A nil pointer means "not
permitted; do not render". The renderer's TS interface mirrors this:

```ts
interface Capabilities {
  reorder?: ReorderCap;
  create?: CreateCap;
  edit?: EditCap;
  delete?: DeleteCap;
  inline_rename?: InlineRenameCap;
  stats?: StatsCap;
}
```

When to pick this shape:
- The action carries a structured request body or its own sub-schema
  (a form, a reorder payload, an inline rename URL template).
- The set of possible actions is closed and known to the renderer.
- Type safety on the consumer side is worth the schema-major cost of
  adding a new capability kind.

### Shape 2 — Generic link relations

For open-ended hypermedia (navigation, breadcrumbs, related entities,
pagination, alternative views, sharing, "open in...", etc.), use a
generic links array on the page envelope, on widgets, and on entity
items:

```go
type Link struct {
    Rel    string `json:"rel"`
    Href   string `json:"href"`
    Method string `json:"method,omitempty"`
    Title  string `json:"title,omitempty"`
    Type   string `json:"type,omitempty"`
}

// On PageSchema, on widgets, on entity items.
Links []Link `json:"links,omitempty"`
```

Presence of a link with `rel == "X"` is the permission grant for
following X. Absence means "not available to this user, in this view,
right now."

When to pick this shape:
- The action is navigational or read-only (rarely carries a body).
- New relations are added regularly and additively (a new `rel` is
  forwards-compatible — old renderers ignore unknown rels).
- A third-party / alternative renderer would benefit from generic
  dispatch on `rel` rather than mirroring typed structs.

### Both shapes coexist on the same response

A page can carry typed `Capabilities` and a generic `Links` array
simultaneously. They do not overlap by design: capabilities express
"what mutations are allowed on this thing"; links express "where else
can the user go from here." Both honour the presence-iff-permitted
invariant.

## Consequences (good)

- **No client-side permission code.** New role, scope, or capability is
  a pure server change. No renderer redeploy.
- **Permission-by-construction.** It is impossible to build a "show the
  button but make it not work" bug. The button is rendered iff the
  schema says it is allowed.
- **Alternative renderers respect permissions for free.** A mobile app,
  a terminal client, a third-party integration — all see the same
  per-user schema.
- **No permission logic to keep in sync** between server and renderer.

## Consequences (bear in mind)

- **Per-user response computation.** Capability presence and link
  emission depend on the current user. Caching is therefore user-scoped
  or partitioned by role. Plan for this in the response cache layer.
- **Every action requires either a capability or a link.** Sometimes
  feels redundant for "obvious" actions; do it anyway.
- **"What could I do here?" is server-driven.** Power users cannot
  preview what permissions would unlock without authenticating.
- **"Sign in to use" UX needs explicit modeling.** Either return a
  capability with an `auth_required: true` flag, or a link with the
  same flag. Pick a convention before scattering ad-hoc shapes.

## Server-side ergonomics

For typed capabilities, the existing pattern is to leave the pointer
nil unless the role allows it:

```go
caps := Capabilities{}
if user.Can("edit", target) {
    caps.Edit = &EditCap{URL: editURL(target)}
}
```

For generic links, prefer the fluent helper (added when the first such
link lands):

```go
links.IfCan(user, "share", article).Add(resp, "share", shareURL(article))
```

If the helper feels tedious, fix the helper — do **not** add
client-side fallbacks.

## Renderer enforcement

The invariant is enforced by lint rules in `renderer/`:

- No `if (user.role === ...)` style checks. Permission is encoded in the
  schema, not in identity props.
- No `fetch(` outside the link-handler dispatcher. All requests go
  through capability URLs or link relations emitted by the server.
- No state that isn't derived from props or the schema.

These rules are not advisory. They are the architecture.

## Fixture variants as the authorization spec

Every resource that supports multiple permission levels has multiple
fixtures under [`schema/examples/`](../schema/examples/):

```
schema/examples/
├── article-anonymous.json
├── article-reader.json
├── article-editor.json
└── article-admin.json
```

The diff between these fixtures **is** the authorization spec — both
which capabilities go nil and which links disappear. It is also the
conformance suite for any alternative renderer.

## When you are tempted to break the rule

- "I just need to disable this button when the user can't use it." —
  No. Don't emit the capability or link.
- "I need a permission table to show 'this action requires editor
  role'." — Add an `auth_required` annotation to the (still emitted)
  capability or link.
- "But the renderer already knows the role." — Bad smell. The renderer
  should not have been told the role in a way it can branch on.
  Roles flow through capability presence and link emission, not through
  identity props.
