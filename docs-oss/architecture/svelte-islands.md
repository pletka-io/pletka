# Svelte islands

Interactive UI is built from Svelte "islands" mounted into server-rendered Go
templates. There is no single-page app. A page is HTML rendered by Go; the
interactive parts are small Svelte components that hydrate in place.

## The flow

```
Go template (.gohtml)
  ├─ renders page chrome (breadcrumb, sidebar, layout)
  ├─ embeds <div data-island="…" data-prop-…="…">
  └─ loads island scripts via {{islandScripts "name"}}
        │
        ▼
Vite bundles frontend/src/islands/<name>.ts
        │
        ▼
mountIsland() finds the data-island element, reads its props,
mounts the Svelte component
        │
        ▼
component fetches its JSON schema from the API and renders
```

Go templates own page layout and navigation. Islands own interactive,
data-driven sections. The JSON schema is the contract between them — see
[`schema-driven-ui.md`](schema-driven-ui.md).

## The pieces

**Island entry** — `frontend/src/islands/<name>.ts`, one call to `mountIsland`:

```typescript
import { mountIsland } from '../mount';
import ListManager from '$lib/components/list/ListManager.svelte';

mountIsland('list-manager', ListManager);
```

`mountIsland(name, Component)` queries `[data-island="<name>"]`, skips
already-mounted elements, reads `data-prop-*` attributes (kebab-case →
camelCase), and parses each value (`mount.ts`): `"true"`/`"false"` → boolean,
digit strings → number, and values starting with `{` or `[` → `JSON.parse`
(falling back to the raw string if parsing fails) — so a template can pass a
whole object or array through a single `data-prop-*` attribute. It then clears
the placeholder, mounts the component, and registers it so dynamically added
elements can be lazy-mounted later.

**Go template side:**

```gohtml
{{define "content"}}
<div data-island="list-manager"
     data-prop-schema-url="/projects/{{.Project.ID}}/list-schema/category">
</div>
{{end}}

{{define "scripts"}}
{{islandScripts "list-manager" "entity-form"}}
{{end}}
```

- `data-island="<name>"` selects the component to mount.
- `data-prop-<key>` passes props (`data-prop-schema-url` → `schemaUrl`).
- `{{islandScripts "a" "b"}}` emits the `<script type="module">` and stylesheet
  tags resolved from the Vite manifest.

**Build** — Vite auto-discovers entries from `frontend/src/islands/*.ts` and
outputs to `pkg/assets/static/dist/` with a manifest. `IslandResolver`
(`pkg/assets/islands.go`) reads the manifest and emits the right hashed paths; in
dev it re-reads on each request for hot reload. Run with `make frontend-build`
(or `make air`, which builds the frontend then runs the server).

Styling is Tailwind with exactly two brand colors, `pletka-primary` and
`pletka-secondary` (`frontend/tailwind.config.ts`) — the only brand palette in
the codebase.

## Templates

Only 6 `.gohtml` files remain in the whole repo:
`pkg/weave/templates/{layout,header,footer,error_body,island_page}.gohtml` and
`pkg/weave/project/csvexport/downloads.gohtml`. `layout.gohtml` is the one
shared shell (there is no separate top-level layout file). Multilinguality is
handled entirely by Svelte widgets (`multilingual-text` /
`multilingual-textarea` in the widget registry below) — there is no dedicated
gohtml partial for it. Everything below the shell is island-driven.

## Widgets: a registry, not a branch chain

A form field's `widget` string selects a component through a plain `Map`
lookup, `resolveWidget(field.widget)`
(`frontend/src/lib/components/form/widget-registry.ts`) — not a growing chain
of type comparisons. `resolveWidget` looks the kind up in a registry built by
one `registerWidgets({...})` call. Adding a widget means:

1. add the component to the `registerWidgets({...})` entry in
   `widget-registry.ts`
2. add a matching `Widget*` constant in `pkg/formschema/types.go`
3. add a `frontendrefs.FormWidget("<kind>")` marker call next to the constants
   — a conformance test checks every marker resolves in the frontend registry

19 widget kinds are marker-verified today (the registry carries 3 more — `date`, `url`, `multi-select` — registered as aliases of TextInput/MultiSelect without Go constants or markers; see the compliance backlog): `multilingual-text`,
`multilingual-textarea`, `system-name-preview`, `select`, `search-select`,
`vocabulary-entry-picker`, `pill-multi-select`, `ontology-path`,
`subfield-paths`, `prefix-input`, `slug-input`, `text`, `textarea`, `number`,
`radio-group`, `readonly-stat`, `readonly-table`, `checkbox`, `password`.
There are 20 `Widget*` Go constants — the extra one, `WidgetHidden`, has no
frontend component (it marks a field as schema-only, not renderable). None of
the above are "planned" — all are shipped and in active use.

## Islands built today

17 islands exist in `frontend/src/islands/`: `admin-shell`, `arches-fleet`,
`arches-instance`, `autocomplete-playground`, `build-badge`, `confirm`,
`content`, `entity-form`, `entity-list`, `entity-view`, `list-manager`,
`ontology-page`, `ontology-path`, `project-detail`, `project-settings`,
`toast`, `user-menu`. Of the surfaces once planned alongside these, only Side
Panel and Global Search remain unbuilt — treat those two as aspirational, not
in progress.

## Layout singletons

Some islands are mounted once in the shared layout and used everywhere:
`toast` (`islands/toast.ts`, notifications) and `confirm` (`islands/confirm.ts`
+ `lib/stores/confirm.ts`, the confirmation dialog — call `confirmAction()`
from the store to get a `Promise<boolean>`). Both mount in
`pkg/weave/templates/layout.gohtml:65-68`. Reach for the singleton — do not
build a per-page modal.

## Where domain knowledge lives

The component is a generic renderer. Domain knowledge — entity names, field
relationships, validation rules, mutation URLs — lives in the Go schema builders
(`pkg/weave/<slice>/formschema.go`), not in Svelte. A component that only works
for one entity type, hardcodes a field key, or constructs an API URL has leaked
domain knowledge back into the frontend. When you need new behaviour, prefer a new
schema capability or a new widget type before writing a bespoke component.
