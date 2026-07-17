# Plan 4: Convert Fields Tab to Entity-List Island

**Goal:** Replace the vanilla JS fields tab on the project detail page with the same `entity-list` Svelte island pattern used by models and collections.

**Scope:** Fields tab only. The same `entity-list` island component is reused — only the schema builder and template wiring are new.

---

## What Changes

### 1. New: `pkg/formschema/field_entity_list.go`

`BuildFieldEntityListSchema(projectID, lang string, languages []LanguageInfo) *EntityListSchema`

Follows the exact pattern of `BuildModelEntityListSchema` in `model_list.go`:

- `EntityType`: `"field"`
- `DataURL`: `/projects/{pid}/fields` (existing `FieldHandler.ProjectFieldsAPI`)
- `DataKey`: `"fields"`
- `DetailURLTemplate`: `/projects/{pid}/fields/{id}`
- `ProjectID`: passed in
- `Search`: placeholder "Search fields..."
- `SortOptions`: Name, System Name, Ontology Scope, Expected Value Type
- `DefaultSort`: `"ui_name"`
- `Filters`: Type (All / Direct / Indirect)
- `RowLayout`:
  - `TitleField`: `"ui_name"`
  - `SubtitleField`: `"description"`
  - `IdentityFields`: semantic_id (mono), system_name (code)
  - `SemanticBadges`: ontology_scope (purple), expected_value_type (blue)
  - `ProcessBadges`: status, ownership
- `CreateAction`: "Add Field" modal
- `EmptyState`: "No fields yet"

### 2. Modify: `pkg/handlers/projects.go`

Add `"field"` case to `EntityListSchemaAPI` switch:

```go
case "field":
    schema = formschema.BuildFieldEntityListSchema(projectID, lang, languages)
```

### 3. Modify: `pkg/assets/templates/pages/projects/detail.gohtml`

Replace the `#content-fields` div contents (search bar, filter rows, vanilla JS list container) with:

```html
<div data-island="entity-list"
     data-prop-schema-url="/projects/{{.Project.ID}}/entity-list-schema/field">
    <div class="animate-pulse space-y-4 p-6">
        <div class="h-10 bg-gray-200 rounded w-full"></div>
        <div class="h-16 bg-gray-200 rounded"></div>
        <div class="h-16 bg-gray-200 rounded"></div>
        <div class="h-16 bg-gray-200 rounded"></div>
    </div>
</div>
```

### 4. Clean up dead vanilla JS

Remove the following JS functions from `detail.gohtml` if they're only used by the fields tab:
- `fetchProjectData` for fields
- `renderComponentList` for fields
- Field-specific search/filter handlers
- Field type filter, scope filter, path class/property filters

Keep any JS that's still used by overview or activity tabs.

### 5. Verify data format

The `FieldHandler.ProjectFieldsAPI` returns field data. Verify its response format matches what the `entity-list` island expects (same shape as the models API). The entity-list component reads items from `response[dataKey]` where `dataKey` is `"fields"`.

---

## Files

| File | Action |
|------|--------|
| `pkg/formschema/field_entity_list.go` | Create |
| `pkg/handlers/projects.go` | Modify (add field case to EntityListSchemaAPI) |
| `pkg/assets/templates/pages/projects/detail.gohtml` | Modify (replace fields tab, remove dead JS) |

## Success Criteria

1. `/projects/LA#fields` tab renders using the entity-list Svelte island
2. Fields are searchable, sortable, and filterable through the island UI
3. Field detail links work (`/projects/LA/fields/LAF.6`)
4. Models and collections tabs continue working unchanged
5. No vanilla JS field rendering code remains in detail.gohtml
