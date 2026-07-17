# Plan 5(a): Unified WeaveStore List Query Interface

**Goal:** Port the `WeaveProjectStore.List` query pattern (`opts ...QueryOption` → `([]*T, int64, error)`) to `WeaveFieldStore`, `WeaveModelStore`, and `WeaveCollectionStore`, then wire the handler APIs through so the models and collections tabs on the project detail page show real data. Breaking change — no backwards compatibility.

**Why now:** After Plan 4b migrated `ProjectFieldsAPI` to WeaveStore, fields show data but `ProjectModelsAPI` and `ProjectCollectionsAPI` still use GORM and return 0 rows. The `worktree-projects-list` merge added the reference implementation (`WeaveProjectStore.List`) with search/sort/pagination. This plan unifies the four stores under that interface so every entity list works the same way.

---

## Core contract

**All four Weave stores expose the same `List` signature:**

```go
List(ctx context.Context, opts ...QueryOption) ([]*T, int64, error)
```

- `opts` accepts any combination of `WithSearch`, `WithSearchColumns`, `WithFilter`, `WithLimit`, `WithOffset`, `WithOrderBy`, `WithProjectID`
- Returns `(items, total, error)` — `total` is the number of matching rows across all pages (not just the returned slice)
- Existing `projectID string, limit int, offset int` positional arguments on `WeaveFieldStore`/`WeaveModelStore`/`WeaveCollectionStore` are removed entirely — no overloads, no compatibility shims
- Callers use `WithProjectID(id)` when they need project scoping

This is the non-negotiable contract. `go build` surfaces every caller that needs to update.

---

## Signature changes

| Store | Before | After |
|---|---|---|
| `WeaveFieldStore` | `List(ctx, opts ...QueryOption) ([]*Field, error)` | `List(ctx, opts ...QueryOption) ([]*Field, int64, error)` |
| `WeaveModelStore` | `List(ctx, projectID string, limit, offset int) ([]*Model, error)` | `List(ctx, opts ...QueryOption) ([]*Model, int64, error)` |
| `WeaveCollectionStore` | `List(ctx, projectID string, limit, offset int) ([]*Collection, error)` | `List(ctx, opts ...QueryOption) ([]*Collection, int64, error)` |
| `WeaveProjectStore` | `List(ctx, opts ...QueryOption) ([]*Project, int64, error)` | (unchanged — already the target) |

---

## Store implementations

Each store's `List` follows `pkg/weave/weave_project_store.go` as the reference. The SQL shape is:

```sql
SELECT <columns> FROM <table>
WHERE <conditions from options>
ORDER BY <order clause from option>
LIMIT $n OFFSET $m
```

With a parallel `SELECT COUNT(*) FROM <table> WHERE <conditions>` for total.

**Option handling rules:**

- **`WithSearch` + `WithSearchColumns`**: if `WithSearchColumns` is empty, default to `["ui_name"]`. For JSONB columns (`ui_name`, `description`), emit `EXISTS (SELECT 1 FROM jsonb_each_text(col) jt WHERE jt.value ILIKE $n)`. For text columns (`system_name`, `semantic_id`), emit `col ILIKE $n`.
- **`WithProjectID`**: emit `project_id = $n`.
- **`WithFilter(key, value)`**: emit `key = $n`. Only safe for a known allow-list of columns; unknown keys are silently dropped (not errored) to match the projects store behavior.
- **`WithOrderBy("ui_name"|"description", desc)`**: emit `ORDER BY COALESCE(col->>'en', '') ASC|DESC`. For other columns, emit `ORDER BY col ASC|DESC`.
- **`WithLimit` + `WithOffset`**: emit `LIMIT/OFFSET`. `Limit = 0` (default from `ApplyOptions`) means unlimited — same as projects store.

**Default search columns when `WithSearchColumns` is absent:** all three stores behave the same — default to `["ui_name"]`. Callers that want broader search pass `WithSearchColumns("ui_name", "description")` explicitly.

---

## Handler changes

### `ProjectModelsAPI` (rewrite)

`GET /projects/{projectID}/models`

Query params: `search`, `sort_by`, `sort_dir`, `page`, `per_page`

Response:

```json
{
  "models": [...],
  "page": 1,
  "per_page": 30,
  "total": 11
}
```

Implementation: build `opts` from query params, call `h.weave.WeaveModels().List(ctx, opts...)`, shape the response. Uses `WithSearchColumns("ui_name", "description")`.

### `ProjectCollectionsAPI` (rewrite)

`GET /projects/{projectID}/collections`

Same shape as models, just `"collections"` key. Uses `WithSearchColumns("ui_name", "description")`.

### `ProjectFieldsAPI` (rewrite)

`GET /projects/{projectID}/fields`

Same shape:

```json
{
  "fields": [...],
  "page": 1,
  "per_page": 50,
  "total": 617
}
```

Breaking changes from current shape:
- `count` field removed (use `total` instead)
- `page`/`per_page`/`total` added
- Removes the unused `type` filter branch

The existing `fieldListItem` flattening for `ontology_scope` stays — we keep the projection but embed it in the new response shape.

### Other callers

Every other caller of `WeaveFieldStore.List`, `WeaveModelStore.List`, `WeaveCollectionStore.List` must update to the new signature. `go build ./...` will surface them. Expected locations:

- `pkg/handlers/entity_view.go` (if it uses `WeaveModels().List` or `WeaveCollections().List` for something)
- `pkg/handlers/*.go` (any other list callers)
- Anything under `pkg/weave/` that cross-references these stores

Each caller is updated to the new signature with appropriate options. No compatibility shim.

---

## Schema changes

### `BuildFieldEntityListSchema`

Drop the `Filters` block (the non-functional `type` filter). The resulting schema has search + sort + widget + view_modes, no filters. Clean surface — UI toolbar won't render a filter dropdown the backend ignores.

Models and collections schemas: unchanged. They don't declare filters today.

---

## Frontend changes

### Field data flattening

The `fieldListItem` projection keeps flattening `ontology_scope` from `PathElement` to the prefixed string (fixes the `[object Object]` bug we fixed earlier) and keeps emitting `path_elements` for the detailed FieldCard view mode. No change needed there.

### No TypeScript changes

The existing `EntityListView` already calls `data_url` with sort/search params. Response shape already accommodates extra fields. The frontend already reads `data[schema.data_key]`, ignoring `total`/`page`/`per_page` for now. Pagination UI is future work (not in this plan).

---

## What this delivers

1. `WeaveFieldStore.List`, `WeaveModelStore.List`, `WeaveCollectionStore.List` all match `WeaveProjectStore.List` signature
2. `ProjectModelsAPI` returns real model data from WeaveStore (was empty)
3. `ProjectCollectionsAPI` returns real collection data from WeaveStore (was empty)
4. `ProjectFieldsAPI` returns the uniform response shape (breaking: `count` removed)
5. Fields schema drops the orphan `type` filter

## What this does NOT deliver

- Pagination UI (backend returns `total`, frontend doesn't yet render page controls — future plan)
- Edit mode / change tracking (Plan 6)
- Direct/indirect field filter (separate ticket if needed)
- Stats breakdowns for list rows (only counts from `StatsForProjects`, already there)
- Any change to the GORM repositories — they stay, unused by these APIs

---

## Files

### Modify

| File | Change |
|---|---|
| `pkg/domain/weave.go` | Update `WeaveFieldStore`, `WeaveModelStore`, `WeaveCollectionStore` interface `List` signatures |
| `pkg/weave/weave_field_store.go` | Rewrite `List` to match project store pattern (raw SQL + count query) |
| `pkg/weave/weave_model_store.go` | Same |
| `pkg/weave/weave_collection_store.go` | Same |
| `pkg/handlers/projects.go` | Rewrite `ProjectModelsAPI`, `ProjectCollectionsAPI` to use WeaveStore |
| `pkg/handlers/fields.go` | Rewrite `ProjectFieldsAPI` to new response shape; drop `type` filter handling |
| `pkg/formschema/field_entity_list.go` | Drop `Filters` block |
| Any other `*.go` file with a broken caller surfaced by `go build` | Update to new signature |

### Create

None. This is a refactor, not new functionality.

### Delete

None. GORM repos stay available for other code paths.

---

## Success criteria

1. `go build ./...` passes
2. `curl /projects/LA/fields?per_page=3` returns `{fields, page, per_page, total}` with `total > 0` and a 3-item `fields` array
3. `curl /projects/LA/models` returns `{models, page, per_page, total}` with `total > 0` and real model data (was empty)
4. `curl /projects/LA/collections` returns `{collections, page, per_page, total}` with `total > 0` and real collection data (was empty)
5. The fields tab on the project detail page still renders with `FieldCard` and the view mode toggle works
6. The models and collections tabs now render real rows (not the "No X yet" empty state)
7. Search + sort query params still work for all three endpoints
8. No GORM usage in any of the three rewritten handlers
