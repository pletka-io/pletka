# Plan 2: Entity View Frontend — Read-Only Svelte Widgets

**Goal:** Replace the model editor page and collection detail page with a unified read-only Svelte island that consumes the EntityView API (Plan 1). Fresh components that own their types — no transformation layers.

**Scope:** Read-only view mode only. No edit mode, no change tracking, no save. Edit mode is Plan 3.

---

## Principles

1. **Fresh components, no mapping layers** — New `entity/` components consume API types directly. No adapters, no duck-typed state objects bridging old and new worlds. The existing `editor/` components stay untouched for the legacy editor path.
2. **Use existing editor as visual inspiration** — Same Tailwind styling, same UX patterns (collapsible categories, field badges, ontology path rendering, compact/detailed toggle). But the new components own their types and state.
3. **One template for both entity types** — Same gohtml template and Svelte island for models and collections. The API response shape is identical; the island doesn't know which type it's rendering.
4. **API-driven** — The island fetches data from `GET /projects/{pid}/entity-view/{type}/{id}`. No server-side data prefetching in the handler.
5. **No technical debt** — We're in greenfield territory on this branch. Zero transformation layers, zero compatibility shims. Components speak the API language directly.

---

## Routing

| Route | Handler | Purpose |
|---|---|---|
| `/{pid}/models/{mid}` | `EntityViewPage` | New entity view (model) |
| `/{pid}/collections/{cid}` | `EntityViewPage` | New entity view (collection) |
| `/{pid}/models/{mid}/legacy-editor` | `ModelEditorIsland` (existing) | Old model editor, temporary fallback |

The old `ModelEditorIsland` handler moves to `/legacy-editor`. The old `CollectionDetail` handler is replaced entirely (it was server-rendered Go HTML, no Svelte).

---

## TypeScript Types

New file `frontend/src/lib/types/entity-view.ts` with types matching the Go `pkg/domain/entity_view.go` response types directly. No intermediate layer.

```typescript
// Matches Go domain.EntityViewResponse exactly
interface EntityViewResponse {
  entity: EntityViewMeta
  capabilities: ViewCapabilities
  view_mode: string
  sections: ViewSection[]
  refs: ViewRefs
  stats: ModelViewStats
}

interface ViewSection {
  widget: string        // "category-group"
  id: string
  name: Translations
  canonical_order: number
  items: ViewItem[]
}

interface ViewItem {
  widget: string        // "collection-group" | "field-group"
  id: string
  name: Translations
  ontology_scope: string
  field_count: number
  fields: ViewField[]
  shared_path_prefix: PathElement[]
}

interface ViewField {
  widget: string        // "field-override"
  override_id: number
  field_id: string
  field_semantic_id: string
  field_system_name: string
  position: number
  display_name: Translations
  description: Translations
  expected_value_type: string
  set_value: string
  is_required: boolean
  is_hidden: boolean
  ontology_path: string
  path_elements: PathElement[]
  resource_model_refs: EntityRef[]
  collection_model_refs: EntityRef[]
  category_id: string
  collection_id: string
}

// ... EntityViewMeta, ViewCapabilities, ViewRefs, ViewRefEntry, ModelViewStats
```

Components import these types directly. No mapping to legacy editor types.

---

## EntityViewState

A lightweight Svelte 5 reactive state class. Holds the API response as-is and manages UI state.

```typescript
class EntityViewState {
  // Data (from API, stored as-is)
  response: EntityViewResponse | null = $state(null)
  
  // Props
  projectId: string
  entityType: string
  entityId: string
  
  // UI state (persisted to localStorage)
  expandedCategories = $state(new Set<string>())
  collapsedCollections = $state(new Set<string>())
  expandedFields = $state(new Set<string>())
  viewMode: 'compact' | 'detailed' = $state('compact')
  
  // Loading
  loading = $state(false)
  error: string | null = $state(null)
  
  // Methods
  async init(): Promise<void>              // Fetch from EntityView API, restore UI state
  toggleCategory(id: string): void
  toggleCollection(key: string): void
  toggleField(id: string): void
  expandAllCategories(): void
  collapseAllCategories(): void
  
  // Ref resolution (uses response.refs maps)
  resolveRefName(id: string, type: 'model' | 'collection' | 'category', lang: string): string
}
```

### UI state persistence

Expand/collapse state and view mode are persisted to localStorage, keyed by entity:

- Key: `entity-view:{entityId}` (e.g., `entity-view:LAM.1`)
- Value: `{ expandedCategories: string[], collapsedCollections: string[], viewMode: string }`
- Restored on `init()`, saved on every toggle
- No URL hash — expand state is a UI preference, not shareable state

Components access `state.response.sections`, `state.response.entity`, etc. directly. No intermediary data structures.

---

## Components

All new components in `frontend/src/lib/components/entity/`.

### EntityView.svelte

Entry component mounted by the island.

- Creates `EntityViewState`, calls `init()`
- Renders loading skeleton / error state
- Renders entity header: name (translated), ontology scope badge, status badge, description
- Renders stats bar: total fields, required count, categories, collections
- Renders toolbar: expand-all / collapse-all, compact/detailed toggle
- Iterates `response.sections` and renders `CategorySection` for each
- Uses `response.refs` for name resolution in header

### CategorySection.svelte

One collapsible category group. Visual style matches existing `CategoryAccordion`.

- Props: `section: ViewSection`, `state: EntityViewState`
- Header: category name, collection count, field count, expand chevron
- When expanded: iterates `section.items`, renders `CollectionGroup` or `FieldGroup` based on `item.widget`
- Blue left border for named categories, gray for uncategorized

### CollectionGroup.svelte

One collection within a category. Visual style matches existing `CollectionGroup`.

- Props: `item: ViewItem`, `state: EntityViewState`
- Header: collection name, shared path prefix badges, field count circle
- Collapsible field list
- Green left border, green tinted header

### FieldOverride.svelte

One field row. Visual style matches existing `FieldRow`.

- Props: `field: ViewField`, `state: EntityViewState`
- **Compact mode:** field name, value type badge, required badge, model/collection ref links, expand chevron
- **Detailed mode:** full card with name, semantic ID, description, ontology path badges, all refs
- Expanded detail panel (compact mode): path with copy button, ID, type, model/collection refs
- Ontology path rendering: parse `path_elements` into colored badges (blue for classes, green for properties)
- Ref links use `response.refs` for display names, link to entity view pages

### Shared utilities

Extract from existing `editor/` code into `frontend/src/lib/utils/`:

- `ontology-path.ts` — already exists, `pathElementsToDisplay()` and `parseOntologyPath()` are reusable as-is
- `locale.ts` — already exists, `getUILang()` reusable
- `Badge.svelte` — already exists in `shared/`, reusable

---

## Template

### entity-view.gohtml (new)

```html
{{define "content"}}
<div class="max-w-6xl mx-auto px-4 py-6">
  <div data-island="entity-view"
       data-prop-project-id="{{.ProjectID}}"
       data-prop-entity-type="{{.EntityType}}"
       data-prop-entity-id="{{.EntityID}}">
    <!-- Loading skeleton -->
    <div class="animate-pulse space-y-4">
      <div class="h-8 bg-gray-200 rounded w-1/3"></div>
      <div class="h-4 bg-gray-200 rounded w-1/2"></div>
      <div class="space-y-3 mt-6">
        <div class="h-12 bg-gray-200 rounded"></div>
        <div class="h-12 bg-gray-200 rounded"></div>
        <div class="h-12 bg-gray-200 rounded"></div>
      </div>
    </div>
  </div>
</div>
{{.IslandScriptTags "entity-view"}}
{{end}}
```

---

## Handler

### EntityViewPage (new)

```go
func (h *ProjectHandler) EntityViewPage(w http.ResponseWriter, r *http.Request) {
    projectID := chi.URLParam(r, "projectID")
    entityType := // determined from route context ("model" or "collection")
    entityID := chi.URLParam(r, "modelID") or chi.URLParam(r, "collectionID")
    
    data := map[string]any{
        "ProjectID":  projectID,
        "EntityType": entityType,
        "EntityID":   entityID,
        "PageTitle":  // fetched from entity name for browser tab
    }
    
    h.Render(w, r, "pages/projects/entity-view", data)
}
```

Two route registrations share the same handler, differing only in URL param name and entity type string.

---

## Files

### Create

| File | Purpose |
|---|---|
| `frontend/src/lib/types/entity-view.ts` | TypeScript types matching Go API response |
| `frontend/src/lib/stores/entity-view.svelte.ts` | EntityViewState class |
| `frontend/src/lib/components/entity/EntityView.svelte` | Entry component |
| `frontend/src/lib/components/entity/CategorySection.svelte` | Category group |
| `frontend/src/lib/components/entity/CollectionGroup.svelte` | Collection group |
| `frontend/src/lib/components/entity/FieldOverride.svelte` | Field row |
| `frontend/src/islands/entity-view.ts` | Island entry point |
| `pkg/assets/templates/pages/projects/entity-view.gohtml` | Shared gohtml template |

### Modify

| File | Change |
|---|---|
| `pkg/handlers/projects.go` | Add `EntityViewPage` handler, reroute model/collection detail pages, move old model editor to `/legacy-editor` |

### No changes

| File | Reason |
|---|---|
| `frontend/src/lib/components/editor/*` | Legacy editor stays untouched, accessible via `/legacy-editor` |
| `frontend/src/lib/utils/ontology-path.ts` | Already reusable, imported by new components |
| `frontend/src/lib/components/shared/Badge.svelte` | Already reusable |

---

## Success Criteria

1. `/projects/LA/models/LAM.1` renders the entity view with 7 category sections, 48 fields
2. `/projects/LA/collections/LAC.17` renders the entity view with 1 section, 15 fields
3. Both views support compact/detailed toggle and expand/collapse all
4. Old model editor accessible at `/projects/LA/models/LAM.1/legacy-editor`
5. Zero transformation layers — components consume API types directly
6. EntityView API is the sole data source (no GORM calls in the new page)
7. Existing `editor/` components unchanged
