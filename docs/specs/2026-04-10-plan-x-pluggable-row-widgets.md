# Plan X: Pluggable Row Widget Registry

**Goal:** Enable the schema-driven entity list to render different row widgets per entity type, with per-widget view modes (compact/detailed/etc.). Start with `FieldCard` for the fields list; later widgets for models, collections, and domain-specific SaaS views plug into the same registry.

**Why:** The project detail page's fields tab currently shows fields with the generic `EntityListRow` which can't display ontology paths, value type badges, or other field-specific information. Users get less detail in the browse list than in the in-context field view. More generally, different entity types benefit from different row layouts, and some customers will want their own custom presentations without touching the open-source core.

---

## Core Contract

**The backend is the sole authority on visibility and widget selection. The frontend renders what the API gives it.**

This is the non-negotiable contract for the schema-driven pattern:

- **Visibility:** If a user lacks permission to see an entity, field, or attribute, the server **omits it from the response**. No `hidden: true` flags, no client-side filtering.
- **Widget selection:** The server writes the `row_widget` name into the schema. The frontend looks it up in a registry and renders. No `if (user.role === "admin")` branches in Svelte.
- **View modes:** The schema's `view_modes` array lists the modes available to this user. If a mode isn't exposed, the user can't see it.
- **Security boundary:** Authorization is enforced at the API. The frontend is trust-free. Leaking data via frontend logic is impossible because the data was never sent.

This contract makes the frontend dumb by design. "If it's in the response, render it." SaaS deployments can ship custom schema builders without touching frontend code, because schema is the sole coupling point.

---

## Schema Shape

Extend `EntityListSchema` (Go) and its TypeScript mirror with two new fields:

```go
type EntityListSchema struct {
    // ... existing fields ...
    RowWidget string      `json:"row_widget,omitempty"` // name looked up in frontend registry
    ViewModes []ViewMode  `json:"view_modes,omitempty"` // modes the chosen widget supports
}

type ViewMode struct {
    ID      string              `json:"id"`      // "compact", "detailed", ...
    Label   domain.Translations `json:"label"`
    Default bool                `json:"default,omitempty"`
}
```

`RowWidget` is optional. When absent or empty, the frontend uses `"default"` which maps to the current `EntityListRow`. Existing schemas (models, collections) continue to work without modification.

`ViewModes` is optional. When absent, no view mode toggle is shown. When present, the list of modes comes from the server — the frontend renders exactly those and nothing else.

Example for fields:

```json
{
  "entity_type": "field",
  "row_widget": "field-card",
  "view_modes": [
    {"id": "detailed", "label": {"en": "Detailed"}, "default": true},
    {"id": "compact", "label": {"en": "Compact"}}
  ],
  "row_layout": {
    "title_field": "ui_name",
    "subtitle_field": "description",
    "identity_fields": [...],
    "semantic_badges": [...],
    "process_badges": [...]
  }
}
```

`row_layout` stays as the shared data-shape contract — which fields to read off each item. It's reusable across widgets that want to use the same conventions.

---

## Frontend Registry

```typescript
// frontend/src/lib/components/entity-list/row-widget-registry.ts
import type { Component } from 'svelte';
import EntityListRow from './EntityListRow.svelte';
import FieldCard from './widgets/FieldCard.svelte';

export interface RowWidgetProps {
  item: Record<string, any>;
  rowLayout: EntityRowLayout;
  viewMode: string;
  detailUrlTemplate: string;
  editUrlTemplate?: string;
  projectId: string;
  lang: string;
}

const registry = new Map<string, Component<RowWidgetProps>>();

export function registerRowWidget(name: string, component: Component<RowWidgetProps>): void {
  registry.set(name, component);
}

export function getRowWidget(name: string): Component<RowWidgetProps> {
  return registry.get(name) ?? registry.get('default')!;
}

// Built-in widgets
registerRowWidget('default', EntityListRow);
registerRowWidget('field-card', FieldCard);
```

**SaaS extension point:** a separate entry file (only bundled in SaaS builds) can call `registerRowWidget('custom-saas-view', ...)` additional times. The open-source build ships with `default` and `field-card` only.

**Registration happens at module load time.** Importing `row-widget-registry.ts` populates the registry. `EntityListView` imports the registry once and calls `getRowWidget(schema.row_widget ?? 'default')` per render.

No role parameter on `getRowWidget`. Role logic happens server-side.

---

## Shared Field Utilities

Extract reusable display logic into `frontend/src/lib/utils/field-display.ts`:

- `valueTypeColor(type: string): BadgeVariant` — maps expected value type to badge color
- `statusVariant(status: string): BadgeVariant` — maps status to badge color
- `parseScope(scope: string | PathElement): {prefix, localName}` — extracts prefix:localName from either shape

Existing utilities stay where they are:
- `parseOntologyPath`, `pathElementsToDisplay`, `formatPathForCopy` → `$lib/utils/ontology-path.ts`
- `tr`, `getUILang` → existing locations

`FieldCard` consumes the shared utilities. `FieldOverride` (used by the entity view tab) can adopt them in a follow-up.

---

## FieldCard Component

New file: `frontend/src/lib/components/entity-list/widgets/FieldCard.svelte`

Implements the `RowWidgetProps` contract. Supports two view modes:

**Compact mode:** single row. Title + semantic ID + value type badge + scope badge + date + edit button. Matches the layout of `EntityListRow` but adds field-specific badges.

**Detailed mode:** card with title + semantic ID, description, full ontology path as chained colored badges (classes blue, properties green), resource/collection model ref links, status and ownership badges. Similar visual density to `FieldOverride` in the entity view.

The component reads `item` (the field data from the API) and `viewMode` (the current mode) and renders accordingly. No logic about which mode to show — the toggle lives in `EntityListToolbar`.

The data shape expected on `item`:
- `id`, `semantic_id`, `system_name`
- `ui_name`, `description` (Translations)
- `ontology_scope` (string, already flattened by the backend)
- `expected_value_type` (string)
- `path_elements` (array of PathElement — for detailed mode path rendering)
- `status`, `updated_at`
- `resource_model_refs`, `collection_model_refs` (arrays of EntityRef)

The backend handler flattens complex fields to strings where the widget expects strings. Widget never has to handle `[object Object]`.

---

## View Mode Toggle

`EntityListView` tracks `currentViewMode` as `$state(initialViewMode)` where the initial value is the `view_modes` entry with `default: true`, or the first entry.

`EntityListToolbar` receives `viewModes` and `currentViewMode` as props. If `viewModes` has at least 2 entries, render a segmented toggle. On change, emit a callback that updates the state in `EntityListView`.

The active mode is persisted to `localStorage` keyed by `entity-list:{entity_type}:view_mode`. On mount, restore from localStorage if the stored mode exists in the current `view_modes` list.

The active mode is passed to every row widget instance via the `viewMode` prop.

---

## Backend Changes

1. **`pkg/formschema/entity_list.go`** — Add `RowWidget` and `ViewModes` fields to `EntityListSchema`.
2. **`pkg/formschema/field_entity_list.go`** — Set `RowWidget = "field-card"` and `ViewModes = [detailed, compact]`.
3. **`pkg/handlers/fields.go`** — The `fieldListItem` projection already flattens `ontology_scope`. Extend it if `FieldCard` needs more fields (e.g., `path_elements`, `resource_model_refs`, `collection_model_refs`).

Models and collections are untouched in this plan. Their schemas have no `row_widget` field, so the frontend falls back to `default` (`EntityListRow`) — no behavior change.

---

## Frontend Changes

1. **`frontend/src/lib/types/entity-list-schema.ts`** — Add `row_widget`, `view_modes` to the `EntityListSchema` type.
2. **`frontend/src/lib/components/entity-list/row-widget-registry.ts`** — New file, creates the registry and registers built-ins.
3. **`frontend/src/lib/components/entity-list/widgets/FieldCard.svelte`** — New file.
4. **`frontend/src/lib/utils/field-display.ts`** — New file for shared helpers.
5. **`frontend/src/lib/components/entity-list/EntityListView.svelte`** — Dispatch rows through `getRowWidget(schema.row_widget)`. Pass `viewMode` prop.
6. **`frontend/src/lib/components/entity-list/EntityListToolbar.svelte`** — Render view mode toggle when `view_modes` has 2+ entries.

---

## Migration Path for Other Entity Types

Once the pattern is proven with fields, subsequent plans can:

- Add `ModelCard` with model-specific compact/detailed views (e.g., show ontology scope + field count)
- Add `CollectionCard` similarly
- Add `PointerCard` for the pointer lists the customer mentioned
- Register any number of SaaS-specific widgets without touching the open-source frontend

Each new widget follows the same recipe: implement the `RowWidgetProps` contract, register in the registry, update the schema builder to reference it by name.

---

## Files

### Create

- `pkg/formschema/entity_list.go` — modify (add `RowWidget`, `ViewModes`)
- `frontend/src/lib/types/entity-list-schema.ts` — modify
- `frontend/src/lib/components/entity-list/row-widget-registry.ts` — new
- `frontend/src/lib/components/entity-list/widgets/FieldCard.svelte` — new
- `frontend/src/lib/utils/field-display.ts` — new

### Modify

- `pkg/formschema/field_entity_list.go` — set `RowWidget` and `ViewModes`
- `pkg/handlers/fields.go` — extend `fieldListItem` if needed for detailed mode
- `frontend/src/lib/components/entity-list/EntityListView.svelte` — dispatch through registry, track view mode
- `frontend/src/lib/components/entity-list/EntityListToolbar.svelte` — view mode toggle

---

## Success Criteria

1. Fields tab on `/projects/LA#tab=fields` renders with `FieldCard` — rich detail visible by default
2. Compact/detailed toggle in the toolbar works, state persists to localStorage
3. Models and collections tabs continue working without any schema changes
4. The registry can accept a new widget registration from an external module without touching core files
5. Backend controls widget choice — tested by verifying the widget name appears verbatim in the JSON response
6. No frontend code branches on `user.role` for widget or view mode selection
