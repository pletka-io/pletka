# Naming Conventions

A reference for naming patterns across all layers of Pletka. This is a living document — update it as new patterns are established.

---

## Design Philosophy

1. **One convention per layer.** Each layer (Go, database, JSON, URLs, frontend) follows one naming convention. No mixing within a layer.

2. **Predictable boundary crossings.** When data crosses a boundary, the convention changes in a predictable way: Go PascalCase → JSON snake_case → TypeScript camelCase → DOM kebab-case.

3. **Names carry meaning, not type information.** `ProjectID` not `strProjectID`. `categories` not `tbl_categories`. `ui_name` not `json_ui_name`.

---

## Go

### Packages

Lowercase, concatenated (no underscores or hyphens). **Entity slices are
singular** (`category`, `field`, `model`, `collection`) — one package per
entity type, matching the domain concept it owns. Collective or
infrastructure packages that don't correspond to a single entity type may be
plural: `generators`, `members`, `orgmembers`, `exports`, `routes`, `pages`,
`views`, `templates`.

| Package | Purpose |
|---------|---------|
| `domain` | Domain types, interfaces, `Translations` |
| `weave/<entity>` | A vertical slice: store, service, handler, schemas |
| `weave/router` | Wires the slices together and mounts routes |
| `middleware` | HTTP middleware |
| `formschema` | Shared schema types + builders for form/list schemas |
| `database` | pgx pool, goose migrations, sqlc queries + generated code |
| `ontology` | Ontology parsing and caching |
| `router` | Route registration |
| `templates` | Template management |
| `i18n` | Internationalization |
| `session` | Session management |

Multi-word packages: `formschema` (concatenated), not `form_schema` or `form-schema`.

CLI tools in `cmd/` use kebab-case: `airtable-export`, `airtable-import-v2`, `ontology-import`.

### Files

snake_case. Test files use `_test.go` suffix.

```
category.go              # Model or single-entity logic
category_list.go         # List schema builder for categories
category_test.go         # Tests for category
field_stats.go           # Field statistics queries
field_discovery.go       # Field discovery logic
list_types.go            # List schema type definitions
store.go                 # Slice Store interface
store_postgres.go        # pgx + sqlc Store implementation
```

### Types and Structs

PascalCase. No prefixes or suffixes encoding type information.

```go
// All domain types embed Entity (pkg/domain/entity.go:42)
type Entity struct {
    ID            string
    CreatedAt     time.Time
    UpdatedAt     time.Time
    SemanticID    string
    SystemName    string
    UIName        Translations
    Description   Translations
    Status        Status
    VersionNumber string
    ProjectID     string
    Deprecated    bool
}

// Domain model types embed Entity
type Project struct {
    Entity
    // ...
}
type Field struct {
    Entity
    // ...
}
type Category struct {
    Entity
    // ...
}

// Slice store interfaces
type FieldStore interface { ... }
type CategoryStore interface { ... }

// Schema types
type FormSchema struct { ... }
type ListSchema struct { ... }
type FieldDef struct { ... }

// Type aliases with semantic meaning
type Translations map[string]string
type OntologyType string
type SemanticType string
```

### Functions and Methods

PascalCase. Verb-first for actions, noun-first for getters.

**Constructors:** `New` prefix.
```go
NewPostgresStore(pool)
NewService(store, projects, log)
NewHandler(service)
```

**Handlers:** Action verb + entity. Suffix `API` for JSON endpoints, `Page` for HTML.
```go
// JSON API handlers
ProjectsListAPI(w, r)
ProjectDetailAPI(w, r)
ListSchemaAPI(w, r)
FormSchemaAPI(w, r)
CategoryStatsAPI(w, r)

// HTML page handlers
ProjectsPage(w, r)
ProjectDetailPage(w, r)
NewProjectPage(w, r)

// Mutation handlers
CreateProject(w, r)
UpdateCategory(w, r)
DeleteModel(w, r)
ReorderCategories(w, r)
```

**Store methods:** Standard CRUD + domain-specific.
```go
// Standard
Create(ctx, entity)
GetByID(ctx, id)
Update(ctx, entity)
Delete(ctx, id)
List(ctx, opts...)
Count(ctx, opts...)
Exists(ctx, id)

// Domain-specific
GetByEmail(ctx, email)
GetFieldsGroupedByCategory(ctx, modelID)
GetProjectCategoriesWithCounts(ctx, projectID)
```

**Internal helpers:** unexported camelCase.
```go
buildQuery(ctx, opts...)
applyFilters(query, opts...)
applySearch(query, opts...)
hasField(fieldName)
```

### Constants

PascalCase with a type prefix. String values are lowercase.

```go
// Status
const (
    StatusDraft      = "draft"
    StatusPublished  = "published"
    StatusDeprecated = "deprecated"
)

// Roles
const (
    RoleSuperAdmin  = "superadmin"
    RoleAdmin       = "admin"
    RoleOwner       = "owner"
    RoleMaintainer  = "maintainer"
    RoleContributor = "contributor"
    RoleViewer      = "viewer"
)

// Widget types (kebab-case strings — used in JSON schemas)
const (
    WidgetMultilingualText     = "multilingual-text"
    WidgetMultilingualTextarea = "multilingual-textarea"
    WidgetSystemNamePreview    = "system-name-preview"
    WidgetSelect               = "select"
)

// Form modes
const (
    ModeCreate   = "create"
    ModeEdit     = "edit"
    ModeView     = "view"
    ModeOverride = "override"
)
```

**Pattern:** `{TypePrefix}{Value}` where the prefix groups related constants and the value is descriptive. String values use the convention of the consuming layer (kebab-case for widget types consumed by JSON/frontend, lowercase for status values consumed by the database).

### Typed Enums

Use a string type alias with typed constants:

```go
type OntologyType string

const (
    OntologyTypeBase      OntologyType = "base"
    OntologyTypeExtension OntologyType = "extension"
)

type SemanticType string

const (
    SemanticTypeClass            SemanticType = "class"
    SemanticTypeObjectProperty   SemanticType = "object_property"
    SemanticTypeDatatypeProperty SemanticType = "datatype_property"
)
```

---

## Database

### Table Names

snake_case, plural.

```
projects
weave_projects
weave_fields
weave_categories
weave_models
weave_collections
weave_actors            # actors + institutions (type-discriminated)
weave_field_overrides   # unified polymorphic field-override table
```

Table names are declared explicitly in the goose migration that creates them
(`pkg/database/migrations/`) and referenced by the sqlc query files
(`pkg/database/queries/`). There is no ORM deriving names from struct names —
the migration is the source of truth.

### `weave_` prefix and table eras

`weave_` marks a current-era table — anything created for the sqlc/pgx data
layer. This is the naming convention for all new tables.

Two things sit outside that pattern, both intentionally:

- **16 `weave_*_archive` history tables** — one per audited entity
  (`weave_fields_archive`, `weave_projects_archive`,
  `weave_models_archive`, `weave_categories_archive`,
  `weave_collections_archive`, `weave_adoptions_archive`,
  `weave_change_log_archive`, `weave_change_set_archive`,
  `weave_concept_lists_archive`, `weave_concept_list_entries_archive`,
  `weave_entity_forks_archive`, `weave_field_overrides_archive`,
  `weave_namespace_bindings_archive`, `weave_override_refs_archive`,
  `weave_project_inheritance_archive`,
  `weave_project_ontology_versions_archive`). Same prefix, `_archive` suffix
  — they hold point-in-time snapshot rows, not live state.
- **Infra tables that predate or sit beside the entity-slice pattern**:
  `sessions` (`023_scs_sessions.sql`), `arches_instances`, `archesctl_servers`,
  and `admin_git_restore_jobs` (`061_arches_fleet.sql` and `044_admin_git_restore_jobs.sql`)
  are bare-named by design — they're operational concerns (session storage, an external
  system's fleet inventory, deployment jobs), not domain entities owned by a
  `pkg/weave/<entity>` slice.

**Historical note, not a live pattern:** migration `001_baseline.sql`
originally created a set of bare-named tables (`projects`, `fields`,
`categories`, `models`, `collections`, `model_fields`, `collection_fields`)
mirroring the pre-refactor GORM schema, alongside the new `weave_`-prefixed
tables created from migration `002` onward. Those bare tables were fully
dropped in `069_drop_legacy_gorm_tables.sql` — the GORM→sqlc strangler
migration's tombstone — and no longer exist in the schema. Do not model a
new table on that old bare-name convention; `weave_` is the only live
pattern for entity tables.

### Column Names

snake_case. Match the JSON field name.

```
id                    # ULID, char(26)
created_at            # Timestamp
updated_at            # Timestamp
project_id            # Foreign key
semantic_id           # Human-readable identifier
long_semantic_id      # Full-form identifier
system_name           # Non-translatable slug
ui_name               # JSONB (Translations)
description           # JSONB (Translations)
canonical_order       # Sort position (integer)
```

Override columns in `weave_field_overrides` are plain property names, scoped by
the row's `entity_type` (`model` or `collection`):
```
display_name           # overridden field name (Translations/JSONB)
description            # overridden description
expected_value_type   # narrowed value type
set_value             # pinned concept URI
is_required           # cardinality
```

### Index Names

`idx_{table}_{columns}` pattern:

```sql
idx_categories_project_system_name     -- Partial unique on (project_id, system_name)
idx_my_entities_project                -- Simple on (project_id)
```

---

## JSON / API

### Field Names

snake_case. Match the database column name. Domain structs carry only JSON tags;
the column mapping lives in the sqlc query files, not in struct tags:

```go
type Category struct {
    ID             string       `json:"id"`
    SemanticID     string       `json:"semantic_id,omitempty"`
    SystemName     string       `json:"system_name,omitempty"`
    UIName         Translations `json:"ui_name,omitempty"`
    CanonicalOrder int          `json:"canonical_order"`
    ProjectID      string       `json:"project_id,omitempty"`
}
```

Use `omitempty` for optional fields. Use `json:"-"` for internal-only fields.

### Schema Field Names

Schema types also use snake_case in JSON:

```json
{
  "entity_type": "category",
  "form_schema_url": "/projects/{pid}/form-schema/category",
  "url_template": "/projects/{pid}/categories/{id}",
  "count_field": "field_count",
  "order_field": "canonical_order",
  "primary_language": "en",
  "submit_label": {"en": "Save"},
  "immutable_after_create": true,
  "unique_within": "project_entity_type"
}
```

### Entity Types

Singular lowercase in schema contexts:

```
"category"    # Not "categories"
"field"       # Not "fields"
"model"       # Not "models"
"collection"  # Not "collections"
```

Used in: `entity_type` fields, URL path segments (`/list-schema/category`), handler switch cases.

---

## URLs

### Path Segments

kebab-case for multi-word resources:

```
/list-schema/category
/form-schema/category
/settings/categories
/model-editor/{modelID}
```

Single-word resources stay lowercase:
```
/projects
/categories
/fields
/models
```

### Path Parameters

camelCase in curly braces:

```
{projectID}
{categoryID}
{entityType}
{modelID}
{fieldID}
```

### API Prefix

Versioned API routes use `/api/v1/`:

```
/api/v1/projects
/api/v1/ontology/autocomplete
```

Project-scoped routes omit the API prefix:

```
/projects/{projectID}/list-schema/{entityType}
/projects/{projectID}/categories/{categoryID}
```

---

## Frontend

### Svelte Components

PascalCase file names. One component per file.

```
ListManager.svelte
ListRow.svelte
FormRenderer.svelte
FormSection.svelte
WidgetDispatcher.svelte
DeleteConfirm.svelte
StatsModal.svelte
ToastContainer.svelte
MultilingualText.svelte
SystemNamePreview.svelte
SelectWidget.svelte
```

### Island Entry Points

kebab-case TypeScript files. Each maps to a `data-island` attribute:

```
list-manager.ts       → data-island="list-manager"
entity-form.ts        → data-island="entity-form"
toast.ts              → data-island="toast-container"
model-editor.ts       → data-island="model-editor"
```

### TypeScript Type Files

kebab-case files in `frontend/src/lib/types/`:

```
form-schema.ts        # FormSchema, FieldDef, Section, etc.
list-schema.ts        # ListSchema, Capabilities, Column, etc.
generated.ts          # Auto-generated types from Go models
```

### TypeScript Interfaces

PascalCase. Mirror Go struct names:

```typescript
interface FormSchema { ... }
interface ListSchema { ... }
interface FieldDef { ... }
interface Capabilities { ... }
interface SchemaEndpoint { ... }
```

### Stores

kebab-case or descriptive names with `.ts` or `.svelte.ts` extension:

```
toast.ts              # Toast notification store
model-editor.svelte.ts # Model editor state (Svelte 5 runes)
```

### Functions

camelCase:

```typescript
addToast('success', 'Saved')
slugify('My Category')
tr(translations, lang)
```

---

## Templates (gohtml)

### File Names

kebab-case, organized by feature:

```
pages/projects/list.gohtml
pages/projects/detail.gohtml
pages/projects/model-editor.gohtml
pages/projects/settings/categories.gohtml
partials/multilingual-editor.gohtml
partials/settings-sidebar.gohtml
partials/confirm-modal.gohtml
layouts/base.gohtml
```

**Suffixes with meaning:**
- `-island.gohtml` — contains Svelte island mount points
- `-modal.gohtml` — modal dialog partial
- `-editor.gohtml` — editing form partial

### Template Names

Match the file path, referenced via Go template syntax:

```go
{{template "partials/multilingual-editor" .}}
{{template "partials/settings-sidebar" .}}
{{template "layouts/base" .}}
```

---

## Internationalization

### Key Format

Dot-separated hierarchy with snake_case segments:

```json
{
  "common": {
    "save": "Save",
    "cancel": "Cancel",
    "delete": "Delete",
    "edit": "Edit",
    "view_all": "View all",
    "auto_generated": "auto-generated",
    "not_set": "Not set",
    "no_description": "No description provided"
  },
  "projects": {
    "field_count": "Fields",
    "model_count": "Models",
    "new_project": "New Project"
  },
  "categories": {
    "new_category": "New Category",
    "reassign_fields": "Reassign fields to"
  }
}
```

**Top-level sections** group by feature area: `common`, `projects`, `fields`, `models`, `collections`, `categories`, `errors`.

**Keys** are snake_case and describe the UI element's purpose: `view_all`, `field_count`, `auto_generated`.

### Usage in Templates

```gohtml
{{t "common.save"}}
{{t "projects.field_count"}}
```

---

## Entity Identity

### Semantic IDs

Parsed by `domain.ParseSemanticID` (`pkg/domain/semantic_id.go`), which
recognizes two distinct dual forms — the entity code is **concatenated**
onto the project prefix for fields/models/collections, but **dotted** as
its own segment for categories/concept lists:

```
AMEF.309         # Field 309 in project AME    — {PREFIX}{F|M|C}.{NUMBER}
AMEM.5           # Model 5 in project AME
AMEC.8           # Collection 8 in project AME
LA.CAT.5         # Category 5 in project LA    — {PREFIX}.{CAT|CL}.{NUMBER}
SEM.CL.6         # Concept list 6 in project SEM
```

- Project prefix: uppercase letters (e.g. `AME`, `LA`, `SEM`) — the
  project's `ID`, doubling as its URL/routing prefix.
- Entity type code: `F` (field), `M` (model), `C` (collection) — appended
  directly to the project prefix, no separator. `CAT` (category) / `CL`
  (concept list) — a separate dot-delimited segment instead, since a
  concatenated 3+ letter code would be ambiguous against the project prefix.
- Number: sequential within project + entity type.
- Immutable after creation — used as a stable, human-readable reference.
  An optional `_system_name` suffix (e.g. `AMEM.15_person`) may be appended
  for display; it is not part of the canonical ID.

### System Names

snake_case slugs derived from the English UI name:

```
existence            # From "Existence"
technical_metadata   # From "Technical Metadata"
person_e21           # From "Person E21"
```

- Generated once at creation time
- Immutable after creation
- Unique within project + entity type scope
- Used as human-readable identifiers in URLs (future) and cross-project matching

### ULIDs

26-character string IDs, lexicographically sortable:

```
01HQ3KXYZ...
```

- Primary key for all entities
- Generated via `ids.GenerateULID()` (`pkg/ids`)
- Used in current URL scheme (`/projects/{ULID}/...`)
- Will be supplemented by org/project slugs in URLs (see api-design.md)

---

## Tailwind / CSS

### Custom Color Classes

Project-prefixed semantic color names:

```
bg-pletka-primary            # Primary action background
hover:bg-pletka-secondary    # Primary action hover
hover:bg-pletka-primary-dark # Darker primary (hover)
bg-pletka-gradient           # Gradient background
```

Standard Tailwind utilities for everything else. No custom utility classes beyond the color palette.
