# Naming Conventions — Design Contract

Read `docs-oss/reference/naming-conventions.md` for the full reference with examples.

## Core Principle: Consistent Boundaries

Each layer has one naming convention. Mixing conventions within a layer (e.g., camelCase JSON in a snake_case API) creates confusion and bugs. When data crosses a boundary (Go → JSON → TypeScript → DOM), the convention changes predictably.

## Go Package Carve-Out

Entity slices are **singular** (`category`, `field`, `model`, `collection`) — one
package per entity type. Collective or infrastructure packages that don't
correspond to a single entity type may be **plural**: `generators`, `members`,
`orgmembers`, `exports`, `routes`, `pages`, `views`, `templates`.

## Quick Reference

| Layer | Convention | Example |
|-------|-----------|---------|
| Go types/functions | PascalCase | `CreateCategory`, `Entity`, `FieldStore` |
| Go packages | singular for entity slices, plural allowed for collective/infra | `category`, `field` vs. `generators`, `members` |
| Go files | snake_case | `category_list.go`, `store_postgres.go` |
| Go constants | PascalCase prefix + value | `StatusDraft`, `RoleViewer`, `WidgetSelect` |
| Database tables | snake_case, `weave_` prefix for current-era tables | `weave_projects`, `weave_fields`, `weave_field_overrides` |
| Database archive tables | snake_case, `weave_` prefix + `_archive` suffix | `weave_fields_archive`, `weave_projects_archive` |
| Database columns | snake_case | `semantic_id`, `ui_name`, `created_at` |
| JSON fields | snake_case | `"entity_type"`, `"ui_name"`, `"project_id"` |
| URL paths | kebab-case | `/list-schema/category`, `/form-schema/category` |
| URL parameters | camelCase | `{projectID}`, `{entityType}`, `{categoryID}` |
| Svelte components | PascalCase files | `ListManager.svelte`, `FormRenderer.svelte` |
| Svelte islands | kebab-case files | `list-manager.ts`, `entity-form.ts` |
| TypeScript types | kebab-case files | `form-schema.ts`, `list-schema.ts` |
| TypeScript interfaces | PascalCase | `FormSchema`, `ListSchema`, `FieldDef` |
| gohtml templates | kebab-case | `model-editor.gohtml`, `settings-sidebar.gohtml` |
| i18n keys | dot.snake_case | `common.save`, `projects.field_count` |
| Widget types | kebab-case strings | `"multilingual-text"`, `"system-name-preview"` |
| Schema entity types | singular lowercase | `"category"`, `"field"`, `"model"` |
| Semantic IDs (field/model/collection) | `{PREFIX}{F\|M\|C}.{NUMBER}` | `AMEF.309`, `AMEM.5` |
| Semantic IDs (category/concept list) | `{PREFIX}.{CAT\|CL}.{NUMBER}` | `LA.CAT.5`, `SEM.CL.6` |
| System names | snake_case | `"existence"`, `"technical_metadata"` |

## Red Flags

- camelCase in JSON fields (should be snake_case)
- Plural entity types in schema endpoints (should be singular: `category` not `categories`)
- PascalCase in URL paths (should be kebab-case)
- snake_case in Svelte component file names (should be PascalCase)
- Hardcoded widget type strings in Svelte (use constants from schema)
- A new table named without the `weave_` prefix (only `sessions`,
  `arches_instances`, `archesctl_servers`, and `admin_git_restore_jobs` are bare
  by design — pre-existing infra tables, not a pattern to extend).
  `arches_instances` and `archesctl_servers` are platform-owned: they stay in
  the shared DB, but their schema is owned by pletka-platform migrations after
  Phase B — do not add or alter them from core.
