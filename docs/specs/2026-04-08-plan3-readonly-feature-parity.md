# Plan 3: Read-Only Feature Parity

**Goal:** Bring the new entity view to feature parity with the legacy model editor for read-only use. Add tabs (stats, metadata, diagram), field search, sticky header, and pull in the mermaid visualization code.

**Scope:** Read-only features only. No edit mode. Visual parity with the legacy editor — same Tailwind styling, same UX patterns, same icons.

---

## Principles

1. **Adapt, don't rewrite** — Copy legacy components (`ModelStatsTab`, `ModelMetadataTab`, `ModelDiagram`) and swap data sources from `ModelEditorState` to `EntityViewState`/`EntityViewResponse`. The Tailwind layout stays identical.
2. **Single API response** — Avoid multiple fetches. Add `created_at`/`updated_at` to `EntityViewMeta` so metadata tab renders from the existing response.
3. **Lazy-load tabs** — Stats, metadata, and diagram content only render when the tab is opened. Fields tab is the default.
4. **Match the legacy editor exactly** — Go through each UI element during implementation and ensure visual parity: sticky header, icon buttons, badge styles, scroll context.

---

## API Changes

### Extend EntityViewMeta

Add timestamps to the existing response — no new endpoints needed.

```go
type EntityViewMeta struct {
    // ... existing fields
    CreatedAt   time.Time `json:"created_at"`
    UpdatedAt   time.Time `json:"updated_at"`
}
```

TypeScript type updated to match:

```typescript
interface EntityViewMeta {
    // ... existing fields
    created_at: string;
    updated_at: string;
}
```

Golden file tests updated to reflect the new fields.

---

## Mermaid Branch Integration

Copy these files from the `mermaid-criteria-unification` worktree into this branch (single commit):

### New files (copy as-is)
- `pkg/visualization/mermaid/generator.go` — CRITERIA-compatible mermaid generator
- `pkg/visualization/mermaid/generator_test.go`
- `pkg/visualization/mermaid/mermaid.go` — package entry
- `pkg/visualization/mermaid/mermaid_test.go`
- `pkg/visualization/mermaid/style.go` — CRITERIA style config
- `pkg/visualization/mermaid/style_test.go`
- `pkg/visualization/turtle.go` — Turtle RDF generation (replaces old turtle_mermaid.go)

### Files to delete (replaced by new code)
- `pkg/visualization/mermaid.go` — replaced by `mermaid/` subpackage
- `pkg/visualization/mermaid_test.go`
- `pkg/visualization/turtle_mermaid.go` — replaced by `turtle.go`
- `pkg/visualization/turtle_mermaid_test.go`

### Overlapping files (manual merge)
- `pkg/handlers/visualization.go` — take mermaid branch version, verify WeaveStore integration
- `pkg/handlers/export.go` — merge both changes (mermaid adds Turtle export, our branch has weave changes)
- `pkg/router/router.go` — merge both route registrations
- `frontend/src/lib/api/client.ts` — merge API client additions

### Resolver update
- `pkg/visualization/mermaid/resolver.go` — already in this branch, take mermaid branch version (rewritten with SuperClassLookup interface)
- `pkg/visualization/mermaid/resolver_test.go` — same

### RDF bridge
- `pkg/generators/rdf/` or equivalent — bridge converting WeaveStore fields to `rdf.ModelField` for graph building (from mermaid branch)

---

## Component Reuse Strategy

### Copy and adapt (swap data source)

| Legacy component | New component | Changes needed |
|---|---|---|
| `ModelStatsTab.svelte` | `entity/StatsTab.svelte` | Replace `editorState.stats` → `state.response.stats`, `editorState.categoryGroups` iteration → `state.response.sections` iteration |
| `ModelMetadataTab.svelte` | `entity/MetadataTab.svelte` | Replace `editorState.model` → `state.response.entity`. Fields map: `model.ui_name` → `entity.name`, `model.status` → `entity.status`, `model.ontology_scope` → `entity.ontology_scope.prefix:entity.ontology_scope.local_name`, `model.created_at` → `entity.created_at`. Drop `version_number`. |
| `ModelDiagram.svelte` | `entity/DiagramTab.svelte` | Strip to ontology diagram + RDF source view only (drop instance and model diagram tabs). Replace `modelId` prop with `entityId`/`entityType`. |

### Copy pattern (more adaptation)

| Legacy pattern | Where it lives | What to adapt |
|---|---|---|
| Sticky header | `ModelEditor.svelte` lines ~38-50, template lines ~200-300 | Same scroll-tracking and compacted state logic. Adapt to show EntityView data (entity name, scope, status). |
| Scroll context | `ModelEditor.svelte` — `scrollCategoryName`, `scrollCollectionName` state | Same IntersectionObserver pattern. Wire to `ViewSection`/`ViewItem` elements. |
| Field search | `ModelEditor.svelte` — `fieldSearchUpdate()`, `fieldSearchNav()`, `fieldSearchClear()` | Adapt traversal from `categoryGroups[].collections[].fields[]` to `sections[].items[].fields[]`. Same highlight/auto-expand/navigation pattern. |
| Expand/collapse icons | `ModelEditor.svelte` header toolbar | Copy the SVG icon buttons for expand-all, collapse-all, compact/detailed toggle. |
| Tab bar | `ModelEditor.svelte` tab navigation | Copy the tab styling. Tabs: Fields, Statistics, Metadata, Diagram. |

### Reuse as-is (shared components)

- `Badge.svelte` — already used
- `LoadingState.svelte` — for tab loading states
- `EmptyState.svelte` — for empty diagram/stats

---

## Tab Details

### Fields tab (default)
Already implemented in Plan 2. Add:
- Field search bar (sticky, below header)
- Search state in `EntityViewState`: `searchQuery`, `searchMatchIds`, `searchIndex`
- Auto-expand categories/collections containing matches
- Highlight navigation (Enter = next, Shift+Enter = previous, Escape = clear)

### Statistics tab
Adapted from `ModelStatsTab.svelte`. Data comes from `state.response.stats` (server-provided) + client-side iteration of `state.response.sections` for value type counts and required/optional breakdown.

4 summary cards: Categories, Collections, Fields, Overridden
Required vs Optional progress bar
Value type distribution bar chart

### Metadata tab
Adapted from `ModelMetadataTab.svelte`. All data from `state.response.entity`:
- Identity: semantic ID (`entity.id`), status (badge), ontology scope (badge from `entity.ontology_scope`)
- Multilingual names: iterate `entity.name` entries
- Multilingual descriptions: iterate `entity.description` entries
- Timestamps: `entity.created_at`, `entity.updated_at` formatted

### Diagram tab
Adapted from mermaid branch `ModelDiagram.svelte`. Two sub-views:
- **Ontology diagram** — mermaid flowchart rendered via Mermaid.js CDN. Zoom controls, download SVG, source viewer.
- **RDF source** — Turtle output in a code block with copy button.

Data fetched lazily on first tab open via existing diagram/Turtle API endpoints.

---

## EntityViewState Changes

Add to `EntityViewState`:

```typescript
// Tab state
activeTab: 'fields' | 'stats' | 'metadata' | 'diagram' = $state('fields');

// Field search state
searchQuery = $state('');
searchMatchIds: string[] = $state([]);
searchIndex = $state(-1);

// Methods
setTab(tab): void
fieldSearchUpdate(query): void    // Traverse sections, find matches, auto-expand
fieldSearchNav(direction): void   // Next/previous match
fieldSearchClear(): void          // Clear search and restore expand state
```

Tab selection persisted to localStorage alongside expand/collapse state.

---

## Files

### Create

| File | Purpose |
|---|---|
| `frontend/src/lib/components/entity/TabBar.svelte` | Tab navigation component |
| `frontend/src/lib/components/entity/StatsTab.svelte` | Statistics display (adapted from ModelStatsTab) |
| `frontend/src/lib/components/entity/MetadataTab.svelte` | Entity metadata display (adapted from ModelMetadataTab) |
| `frontend/src/lib/components/entity/DiagramTab.svelte` | Ontology diagram + RDF source (adapted from ModelDiagram) |
| `frontend/src/lib/components/entity/FieldSearch.svelte` | Search bar + navigation for fields tab |

### Modify

| File | Change |
|---|---|
| `pkg/domain/entity_view.go` | Add `CreatedAt`, `UpdatedAt` to `EntityViewMeta` |
| `pkg/handlers/entity_view.go` | Populate timestamps from entity |
| `frontend/src/lib/types/entity-view.ts` | Add `created_at`, `updated_at` to `EntityViewMeta` |
| `frontend/src/lib/stores/entity-view.svelte.ts` | Add tab state, search state, search methods |
| `frontend/src/lib/components/entity/EntityView.svelte` | Add sticky header, tabs, search bar, scroll context |
| `pkg/handlers/entity_view_test.go` | Update golden file assertions for new fields |
| `pkg/handlers/testdata/*.golden.json` | Regenerate with timestamps |

### Copy from mermaid branch (single commit)

| Source | Destination | Action |
|---|---|---|
| `pkg/visualization/mermaid/` | same | Replace (new subpackage) |
| `pkg/visualization/turtle.go` | same | New file |
| `pkg/visualization/mermaid.go` | — | Delete |
| `pkg/visualization/turtle_mermaid.go` | — | Delete |
| `pkg/handlers/visualization.go` | same | Merge |
| `pkg/handlers/export.go` | same | Merge |
| `pkg/router/router.go` | same | Merge |
| `frontend/src/lib/api/client.ts` | same | Merge |
| RDF bridge code | `pkg/generators/rdf/` or equivalent | Copy |

---

## Success Criteria

1. Entity view has 4 tabs: Fields (default), Statistics, Metadata, Diagram
2. Statistics tab shows same cards, charts, and counts as legacy `ModelStatsTab`
3. Metadata tab shows identity, multilingual names/descriptions, timestamps — same layout as legacy
4. Diagram tab shows ontology mermaid diagram with zoom/download + RDF source with copy
5. Sticky header matches legacy: entity name, scope badge, scroll context, icon buttons
6. Field search works: type to filter, highlights match, auto-expands containers, Enter/Escape navigation
7. Tab selection persisted to localStorage
8. All data from EntityView API (single fetch for fields/stats/metadata, lazy diagram fetch)
9. Visual parity with legacy editor confirmed section-by-section
