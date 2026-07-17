# Entity List Pagination

**Goal:** Add pagination controls to the entity-list island so large projects (like Linked Art with 600+ fields) don't load all rows in a single request. Schema-driven — the backend decides page size, the frontend renders controls.

**Prerequisite:** Plan 5(a) — already done. All three endpoints (fields, models, collections) return `{items, total, page, per_page}`.

---

## What exists today

- **Backend:** `ProjectFieldsAPI`, `ProjectModelsAPI`, `ProjectCollectionsAPI` accept `page` and `per_page` query params and return `total`, `page`, `per_page` in the response. Default page sizes: fields=50, models=30, collections=30. Max cap: 1000.
- **Schema type:** `PaginationConfig` exists in `pkg/formschema/entity_list_types.go` with `PageSize` and `ParamName` fields. The `EntityListSchema.Pagination` field is declared (`json:"pagination,omitempty"`) but never populated by the schema builders.
- **Frontend:** `EntityListView.svelte` fetches `schema.data_url` with sort/filter params but does not send `page`/`per_page`. The `loadData` function reads `data[schema.data_key]` but ignores `total`/`page`/`per_page` from the response.

## Design

### Schema changes

Populate `Pagination` in all three entity-list schema builders:

```go
Pagination: &PaginationConfig{
    PageSize:    50, // fields: 50, models: 30, collections: 30
    ParamName:   "page",
    PerPageName: "per_page",
},
```

Add `PerPageName` and `PageSizeOptions` to `PaginationConfig`:

```go
type PaginationConfig struct {
    PageSize        int    `json:"page_size"`
    ParamName       string `json:"param_name"`        // query param for page number
    PerPageName     string `json:"per_page_name"`     // query param for page size
    PageSizeOptions []int  `json:"page_size_options"`  // e.g., [25, 50, 100]
}
```

### Frontend changes

**`EntityListView.svelte`** — add pagination state and wire it to `loadData`:

- Track `currentPage` (default 1) and `perPage` (from `schema.pagination.page_size`)
- In `loadData`, append `page` and `per_page` params to the fetch URL
- Read `total`, `page`, `per_page` from the response alongside items
- Reset `currentPage` to 1 on search or filter change

**`PaginationBar.svelte`** — new component, rendered below the entity list:

- Shows: `Showing 1-50 of 617` text
- Page navigation: `< 1 2 3 ... 13 >` (truncated for many pages)
- Optional page-size selector: `Show: 25 | 50 | 100` (from `PageSizeOptions`)
- Emits `onPageChange(page)` and `onPerPageChange(perPage)` events

**Visibility rules:**
- If `schema.pagination` is null/absent, no pagination controls render (backwards compatible)
- If `total <= perPage`, hide pagination (everything fits on one page)
- If `total` is 0, the empty state renders as before (no pagination bar)

### Interaction flow

1. User loads `/projects/LA#tab=fields`
2. `EntityListView` fetches schema → sees `pagination.page_size: 50`
3. First data fetch: `GET /projects/LA/fields?per_page=50&page=1`
4. Response: `{fields: [...50 items], total: 617, page: 1, per_page: 50}`
5. Pagination bar shows: `Showing 1-50 of 617` with 13 pages
6. User clicks page 3 → fetch: `GET /projects/LA/fields?per_page=50&page=3`
7. User searches "birth" → resets to page 1, fetches with `search=birth&page=1`
8. User changes sort → resets to page 1

### URL state

Update the browser URL hash to include page: `#tab=fields&page=3`. On page load, read the page from the hash (same pattern as tab routing). This way, linking to a specific page works.

---

## What this does NOT cover

- Infinite scroll (explicit pagination is simpler and more predictable for cultural heritage data)
- Server-side cursor-based pagination (offset/limit is sufficient for our data sizes)
- Pagination for the projects list (separate schema, separate work)
- Pagination in entity detail views (model fields, collection fields)

---

## Files to modify

| File | Change |
|---|---|
| `pkg/formschema/entity_list_types.go` | Extend `PaginationConfig` with `PerPageName`, `PageSizeOptions` |
| `pkg/formschema/field_entity_list.go` | Populate `Pagination` config |
| `pkg/formschema/model_list.go` | Populate `Pagination` config |
| `pkg/formschema/collection_list.go` | Populate `Pagination` config |
| `frontend/src/lib/types/entity-list-schema.ts` | Add pagination types (if TypeScript schema types exist) |
| `frontend/src/lib/components/entity-list/EntityListView.svelte` | Add page state, wire to loadData, read total from response |
| `frontend/src/lib/components/entity-list/PaginationBar.svelte` | New component |

## Success criteria

1. `/projects/LA#tab=fields` loads 50 fields (not all 617) with pagination bar
2. Clicking page 2 loads the next 50 fields
3. Search resets to page 1
4. Sort resets to page 1
5. `total` in the pagination bar matches the actual count
6. Models and collections tabs also paginate
7. Projects with few items show no pagination bar
8. Page persists in URL hash — reloading `#tab=fields&page=3` goes to page 3
