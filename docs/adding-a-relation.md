# Adding a Link Relation

A link relation (`rel`) is the verb of Pletka's open-ended hypermedia
vocabulary. Use this recipe when you are adding a navigation link, a
breadcrumb verb, a pagination cursor, a related-entity pointer, an
alternative-view link, or any open-ended relation.

> **Choosing the shape.** If the action is a mutation with a structured
> request body or sub-schema (form submit, reorder payload, inline
> rename URL template), it is a **typed capability** instead — see
> [`authorization-model.md`](authorization-model.md) and the relevant
> schema's existing `Capabilities` struct. This recipe is for generic
> links.

## The three files

For a relation called `bar`:

| File                                  | Purpose                                        |
|---------------------------------------|------------------------------------------------|
| `schema/relations/bar.md`             | Spec: meaning, HTTP method, request / response shape. |
| `server/relations/bar.go`             | Emission helper: `func EmitBar(...) Link`.    |
| `renderer/src/relations/bar.ts`       | Handler: what happens when the user follows the link. |

## Step-by-step

### 1. Write the spec

In `schema/relations/bar.md`, document:

- The `rel` string (e.g. `"bar"`).
- The HTTP method.
- The request body shape (or `none`).
- The response body shape (typically a new page schema, or `204`).
- The auth contract: which roles can see this link, and why. Lifting
  this into the spec keeps it visible to alternative renderers.
- Which envelope(s) the link can appear on (page-level `links`,
  widget-level `links`, entity-level `links`).

### 2. Add the emission helper (server)

In `server/relations/bar.go`:

```go
package relations

// EmitBar appends the "bar" link to dst if the current user is permitted.
func EmitBar(dst *[]Link, user User, target Target) {
    if !user.Can("bar", target) {
        return
    }
    *dst = append(*dst, Link{
        Rel:    "bar",
        Method: "GET",
        Href:   barURL(target),
    })
}
```

The fluent helper `links.IfCan(user, "bar", target).Add(dst, "bar", barURL(target))`
is preferred once available — it makes the auth check visible at the call
site.

### 3. Add fixtures with auth variants

For every page where the new relation can appear, add fixtures showing
the link's presence and absence at different roles. The fixture diff
documents the auth rule for the relation.

### 4. Implement the renderer handler

In `renderer/src/relations/bar.ts`:

```ts
import type { Link } from "../types/schema";

export async function handleBar(link: Link, ctx: HandlerContext) {
  // Issue the request the server told us to issue.
  const res = await fetch(link.href, {
    method: link.method ?? "GET",
    headers: { "Content-Type": link.type ?? "application/json" },
    body: ctx.body ?? null,
  });
  if (!res.ok) {
    ctx.handleError(res);
    return;
  }
  // For navigation rels: replace the current page schema and re-mount.
  ctx.replacePage(await res.json());
}
```

The handler MUST NOT:

- Construct the URL itself. `link.href` is canonical.
- Decide whether to issue the request based on user identity. If the link
  is in the schema, the user may follow it.
- Hardcode `bar`-specific URL paths.

### 5. Register dispatch

The renderer's `LinkHandler` (TBD — added with the first real relation)
maps `link.rel === "bar"` to `handleBar`. One place to register; nothing
else dispatches.

### 6. Update the changelog

- Add an entry under `Unreleased` in `schema/CHANGELOG.md`.
- Bump `schema/version.txt` (minor for additive rels).

## What NOT to do

- **Do not** add a relation that only one widget will ever use, and that
  carries a structured request body. That is a typed capability on the
  widget's `Capabilities` struct, not a vocabulary item.
- **Do not** name relations after server-internal concepts (`do_db_update`).
  Use the verb a renderer would expose to a user (`share`, `archive`,
  `republish`).
- **Do not** change the meaning of an existing relation. That is a major
  schema bump.
- **Do not** duplicate a typed capability as a link relation. Pick one
  shape per action.
