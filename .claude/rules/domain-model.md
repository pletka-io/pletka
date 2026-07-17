# Domain Model — Design Contract

Read `docs-oss/architecture/domain-model.md` for the full reference with diagrams and examples.

## Core Principle: One Flat `Entity`, Copy-on-Adopt Overrides

Pletka manages three semantic pattern types — Fields, Models, Collections — that institutions create, adopt, and customize. Every entity embeds one flat `Entity` struct (no two-level hierarchy) and carries a project scope, multilingual display names, and a three-layer identity chain (ULID → SemanticID → SystemName).

## Entities

| Entity | Table | Purpose |
|--------|-------|---------|
| Project | `weave_projects` | Workspace that owns patterns. `ID` doubles as the ID prefix (e.g. `LA`, `SRD`). Has `ParentProjectID`, `Visibility`. |
| Field | `weave_fields` | Atomic semantic unit — one ontology property/edge with an expected value type. |
| Model | `weave_models` | Entity definition — scope class + fields (e.g. "Person" = E21_Person). |
| Collection | `weave_collections` | Grouped fields sharing a root ontology context (e.g. "Birth Event" = E67_Birth). |
| Category | `weave_categories` | Display grouping for fields within a project. No ontological meaning. |

`Entity` (`pkg/domain/entity.go`) carries `ID` (ULID), `CreatedAt`/`UpdatedAt`, `UIName`/`Description` (Translations), `SystemName`, `SemanticID`, `ProjectID`, `VersionNumber` (a string, present on every entity — not gated behind a separate versioned type), `Status` (`draft`/`published`), and `Deprecated` (bool). `Status` and `Deprecated` are independent axes: an entity can be `published` + not deprecated (live) or `published` + deprecated (retired, references intact, excluded from pickers).

## Identity Rules

- **ULID** (`char(26)`): primary key, generated via `ids.GenerateULID()`. Monotonic, sortable.
- **SemanticID**: human-readable, project-scoped, immutable after creation. Two distinct formats:
  - `{PREFIX}{F|M|C}.{N}` for fields/models/collections — e.g. `AMEF.309` (field 309, project `AME`), `AMEM.5` (model 5), `AMEC.8` (collection 8).
  - `{PREFIX}.CAT.{N}` / `{PREFIX}.CL.{N}` (dotted, three parts) for categories and concept lists — e.g. `LA.CAT.5`, `SEM.CL.6`.
- **SystemName**: snake_case slug derived from the English `UIName`. Immutable, unique within project + entity type.

## Overrides — One Unified Table, Copy-on-Adopt

Overrides live in a single polymorphic table, `weave_field_overrides`, never on the base entity. Each row keys a placement by `entity_type` (`''` for base, `'model'`, `'collection'`) + `entity_id` + `field_id`, plus `part_of_collection_id` and `category_id`. Override columns: `display_name`, `description`, `collection_name`, `set_value`, `is_required`, `min_occurs`, `max_occurs`, `is_hidden`, `visibility`, `position`. `expected_value_type` is a `Field`-level structural property, not an override.

**Resolution is copy-on-adopt, not a merge.** The resolver returns every override row for a model/collection — there is no `DISTINCT ON` step collapsing base/model/collection rows into one; each resolved field carries an `OverrideSource` label per row. The one real cascade rule runs the opposite direction from a naive "most-specific-wins" expectation: `applyBaseSetValueLock` locks a non-empty **base** `SetValue` — model/collection overrides cannot change it once the base has committed. An empty base `SetValue` is a no-op; only then do model/collection `SetValue` overrides apply.

The legacy per-type junction tables `model_fields` / `collection_fields` are dropped (migration `069`) — they no longer exist in the schema.

## `path_elements` Is the Source of Truth

`PathElement` (class/property/literal step) drives `Field.PathElements`, `Model.OntologyScope`, `Collection.OntologyScope`, override resolution, and generator output. `Field.OntologyPath()` is a computed string derived from `PathElements` — the string is not independently authored or stored as truth. `PathElement` also carries `AdditionalTypes` (secondary class instantiations) and `SubPropertyOf` (CRM `.1` property-class qualifiers).

## Translations Type

`Translations` = `map[string]string` stored as JSONB. Backs `UIName`, `Description`, and all multilingual fields; implements `driver.Valuer`/`sql.Scanner`. `t.Get("en")` reads with a fallback chain; `t.Set("nl", "value")` is nil-safe.

## Red Flags

- A new entity type that doesn't embed `Entity`
- A non-ULID primary key (everything is `char(26)`)
- An override column added to a base entity — overrides belong in `weave_field_overrides`
- Reintroducing a merge/`DISTINCT ON` step in the resolver — resolution is copy-on-adopt; only the base `SetValue` lock cascades
- Treating `expected_value_type` as override-able
- A missing `json:"…,omitempty"` tag on an API-visible field
