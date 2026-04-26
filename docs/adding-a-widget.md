# Adding a Widget

A widget is the smallest unit the renderer dispatches on. Each widget
is a four-file pattern: spec + fixtures + server builder + renderer
component. The four files must stay in lockstep, and tests on both sides
load the same fixtures so they cannot drift.

## The four files

For a widget called `Foo`:

| File                                  | Purpose                                        |
|---------------------------------------|------------------------------------------------|
| `schema/widgets/foo.md`               | Human-readable spec: fields, semantics, example. |
| `schema/examples/foo-*.json`          | At least one JSON fixture. Add auth variants where relevant. |
| `server/widgets/foo.go`               | Builder: `func Foo(...) Widget` plus type tests. |
| `renderer/src/widgets/Foo.svelte`     | The renderer. Pure consumer of the schema.    |

## Step-by-step

### 1. Write the spec first

In `schema/widgets/foo.md`, document:

- The `type` discriminator (e.g. `"foo"`).
- Every field on the widget: name, JSON type, required vs optional,
  semantics, example value.
- Which **typed capabilities** the widget can carry (mutation grants
  with a structured shape — e.g. an `edit` capability that points at a
  form schema URL).
- Which **link relations** the widget can carry (open-ended navigation
  / related-entity rels — e.g. `share`, `next-page`).
- A worked example showing the JSON shape end to end.

See [`authorization-model.md`](authorization-model.md) for the rule on
when to use a typed capability versus a generic link relation. The
short version: **typed capabilities for mutations**, **generic links
for navigation**.

### 2. Add fixtures

In `schema/examples/`, drop at least one full page fixture that uses the
widget. If the widget participates in authorization, add multiple variants
showing what changes between roles:

```
schema/examples/
├── page-with-foo-anonymous.json
├── page-with-foo-editor.json
└── page-with-foo-admin.json
```

The diff between these fixtures becomes part of the authorization spec.
Both shapes — typed caps going from non-nil to nil, and links
disappearing from the `links` array — are valid forms of the diff.

### 3. Implement the server builder

In `server/widgets/foo.go`:

```go
package widgets

// Foo builds the JSON for a Foo widget. ...
func Foo(/* args derived from the domain object */) Widget {
    w := Widget{
        Type: "foo",
        // ... populate documented fields
    }

    // Typed capability (mutation with a known shape).
    if user.Can("edit", target) {
        w.Capabilities.Edit = &EditCap{URL: editURL(target)}
    }

    // Generic link relation (navigation / open-ended).
    if user.Can("share", target) {
        w.Links = append(w.Links, Link{
            Rel:  "share",
            Href: shareURL(target),
        })
    }
    return w
}
```

Add a Go test that loads each fixture from `schema/examples/` containing
this widget type, decodes it, and asserts the round-trip matches.

### 4. Implement the renderer component

In `renderer/src/widgets/Foo.svelte`:

```svelte
<script lang="ts">
  import type { Widget } from "../types/schema";

  let { widget }: { widget: Widget } = $props();

  // Typed capability — render a button only if the cap is present.
  const canEdit = widget.capabilities?.edit != null;

  // Generic link — find by rel.
  const shareLink = widget.links?.find((l) => l.rel === "share");
</script>

<!-- render strictly based on widget fields, capabilities, and links -->
```

Hard rules for the renderer:

- It does **not** know what an "editor" or an "admin" is. It only knows
  what capabilities and links it received.
- It does **not** construct API URLs. Capability URLs and link hrefs
  come from the server.
- It does **not** fetch additional data. If it needs more data, the
  schema is missing a field — go back to step 1.

### 5. Wire dispatch

The renderer's `WidgetDispatcher` (TBD — added with the first real widget)
maps `widget.type === "foo"` to your `<Foo widget={...} />`. There is one
place to register; do not branch widget rendering anywhere else.

### 6. Add a snapshot test

For each fixture that contains a `Foo` widget, add a renderer snapshot
test. The snapshot should differ between auth variants — that proves the
permission contract is being honoured for both typed caps and links.

### 7. Update the changelog

If this is a new widget type:

- Bump the schema version in `schema/version.txt` (minor for additive
  widgets).
- Add an entry under `Unreleased` in `schema/CHANGELOG.md`.

## What NOT to do

- **Do not** start with the renderer. The schema is the contract; the
  renderer is downstream.
- **Do not** add an entity-specific widget when a generic one fits.
  Domain knowledge lives in the **builder**, not the renderer.
- **Do not** add a `role` or `permissions` field to the widget. Permission
  is the presence of a capability or a link.
- **Do not** mix typed caps and generic links for the same action. Pick
  one shape per action — typed if there's a structured request body or
  sub-schema, generic if it's open-ended navigation.
- **Do not** skip the fixtures. Lint and tests will let you, but the
  contract becomes uncheckable.
