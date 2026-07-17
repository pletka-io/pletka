# Domain model

Pletka manages three kinds of semantic pattern — **Fields**, **Models**, and
**Collections** — that institutions create, adopt, and customise. Everything else
in the model exists to support them. The user-facing story is in
[`../VISION.md`](../VISION.md); this is the structural reference.

## Entities

Every entity has a project scope, multilingual display names, and a stable
identity chain. All of them embed one flat `Entity` struct
(`pkg/domain/entity.go:42-54`) — there is no two-level hierarchy.

| Entity | Table | Purpose |
|---|---|---|
| Project | `weave_projects` | Workspace that owns patterns. `ID` doubles as the project's ID prefix (e.g. `LA`, `SRD`), used as PK and in URL routing. Has `ParentProjectID`, `Visibility`. |
| Field | `weave_fields` | Atomic semantic unit — one ontology property/edge with an expected value type. |
| Model | `weave_models` | Entity definition — a scope class plus fields (e.g. "Person" = E21_Person). |
| Collection | `weave_collections` | Grouped fields sharing a root ontology context (e.g. "Birth Event" = E67_Birth). |
| Category | `weave_categories` | Display grouping for fields within a project. Carries no ontological meaning. |

(The former two-level base/versioned embed hierarchy was flattened into the
single `Entity` struct in the June 2026 purge — see git history for details.)

## Collection placement constraints

`weave_collection_placements` (migration 071) records the
constraints of one collection group placed in a model's category:
`is_required`, `min_occurs`, `max_occurs` (NULL = unbounded), `is_hidden`.
Two constraint layers apply, and they compose:

- **Field constraints inside a collection** (`weave_field_overrides` rows
  with `part_of_collection_id`) are **per collection instance**: "each Name
  has exactly one `name_content`".
- **Placement constraints** are **collection-per-model**: "a Person has
  1..n Names".

Absence of a placement row means the defaults — optional, 0..unbounded,
visible — so legacy models need no backfill. A placement with
`is_hidden = true` removes the whole group from the read-only detail view
and its stats (same policy as hidden fields); the override editor keeps
showing it, flagged, so it can be un-hidden.

## The `Entity` embed

`Entity` (`pkg/domain/entity.go:42-54`) carries every field shared across
`Project`, `Field`, `Model`, `Collection`, and `Category`:

- `ID` — ULID, the primary key.
- `CreatedAt` / `UpdatedAt` — timestamps.
- `UIName` / `Description` — both `Translations`.
- `SystemName` — snake_case slug.
- `SemanticID` — human-readable, project-scoped identifier.
- `ProjectID` — owning project.
- `VersionNumber` — a string, present on every entity (not gated behind a
  separate versioned type).
- `Status` — workflow stage; see below.
- `Deprecated` — retirement flag; see below.

There is no "VersionNumber only exists on the versioned subset" rule — every
entity has one.

### Status and Deprecated are two separate axes

`Status` (`entity.go:8-32`) is two-valued: `StatusDraft` / `StatusPublished`.
`Deprecated bool` (`entity.go:53`) is an orthogonal retirement flag, added in
migration `026_entity_deprecated_lifecycle.sql`: `false` means active, `true`
means soft-retired (existing references stay intact, pickers exclude it, no
new connections are allowed). Hard delete is only permitted once an entity is
unreferenced and, by convention, already deprecated.

Do not conflate the two: an entity can be `draft` + not deprecated (still
being authored), `published` + not deprecated (live), or `published` +
deprecated (retired). `draft` + deprecated is not a normal state but isn't
blocked at the type level.

## Three-layer identity

Every entity carries three identifiers for three audiences:

- **ULID** (`char(26)`) — the primary key. Machine identity. Generated via
  `ids.GenerateULID()` (`pkg/ids/ids.go:13`). Monotonic, sortable, stable
  across everything.
- **SemanticID** — human-readable, project-scoped. Parsed by
  `domain.ParseSemanticID` (`pkg/domain/semantic_id.go`), which recognizes two
  distinct formats:
  - `{PREFIX}{F|M|C}.{N}` for fields, models, and collections — e.g. `AMEF.309`
    (field 309 in project `AME`), `AMEM.5` (model 5), `AMEC.8` (collection 8).
  - `{PREFIX}.CAT.{N}` / `{PREFIX}.CL.{N}` (dotted, three parts) for
    categories and concept lists — e.g. `LA.CAT.5`, `SEM.CL.6`.
  Immutable after creation.
- **SystemName** — a snake_case slug derived from the English `UIName`.
  Immutable, unique within project + entity type.

The project's ID prefix (its `ID`, e.g. `LA`) derives the field/model/collection
prefixes (`LAF`, `LAM`, `LAC`) and the dotted category/concept-list form
(`LA.CAT.*`, `LA.CL.*`).

## Overrides live in one unified table — copy-on-adopt

The same field — same ontology path, same formal meaning — appears in many
contexts under different names. Those context-specific changes are
**overrides**, and they live in a single **polymorphic** table, never on the
base entity: `weave_field_overrides` (migration `005_weave_field_overrides.sql`).

Each row keys a field placement by `entity_type` (`''` for the base row,
`'model'`, or `'collection'`) + `entity_id` + `field_id`, plus
`part_of_collection_id` and `category_id`. It holds the override columns:
`display_name`, `description`, `collection_name`, `set_value`, `is_required`,
`min_occurs`, `max_occurs`, `is_hidden`, `visibility`, `position`. The table
also has an `expected_value_type` column, but the domain `FieldOverride`
struct (`pkg/domain/override.go`) deliberately omits it: expected value type
is a structural property of the `Field` itself, not something resolved as an
override.

**Resolution is copy-on-adopt.** The
resolver (`pkg/weave/resolve.go:272-313`) returns *every* override row for a
model or collection — there is no `DISTINCT ON` / merge step that collapses
base, model, and collection rows into one. Each resolved field carries an
`OverrideSource` label per row; callers see the full set, not a single
flattened value.

The one cascade rule that does exist runs the other direction from what you
might expect: `applyBaseSetValueLock` (`resolve.go:708-765`) enforces that a
non-empty **base** `SetValue` (`entity_type=''`) is locked — model and
collection overrides cannot change it once the base has committed to a fixed
value. An empty base `SetValue` is a no-op; only then do model/collection
`SetValue` overrides apply as usual.

```
base SetValue non-empty  → base value wins, unconditionally
base SetValue empty      → model/collection SetValue override applies (no cascade merge — every row still returned)
```

The earlier per-type junction tables `model_fields` / `collection_fields`
were dropped in migration `069_drop_legacy_gorm_tables.sql`; they no longer
exist in the schema.

## `path_elements` is the source of truth

`PathElement` (`pkg/domain/path_element.go`) is the structured representation
of one step in an ontology path (class, property, or literal). It drives
`Field.PathElements`, `Model.OntologyScope`, `Collection.OntologyScope`,
override resolution, and generator/stats output. `Field.OntologyPath()`
(`field.go:66`) is a *computed* string form derived from `PathElements` — the
string is not independently authored or stored as truth.

`PathElement` also carries:

- `AdditionalTypes []TypeRef` — secondary class instantiations for
  multi-typed nodes (e.g. a node that is simultaneously
  `E29_Design_or_Procedure` and `crmdig:D1_Digital_Object`).
- `SubPropertyOf` — set for CRM `.1` property-class qualifiers (e.g.
  `P14.1_in_the_role_of` qualifies `P14_carried_out_by`); holds the parent
  property code.

This is the current, shipped structure — not a target or aspirational state.

## Translations

`Translations` is a `map[string]string` (language code → text) stored as
JSONB (`pkg/domain/translations.go`). It backs `UIName`, `Description`, and
every multilingual field, and implements `driver.Valuer` / `sql.Scanner` via
`encoding/json`. `t.Get("en")` reads with a fallback chain; `t.Set("nl", …)`
is nil-safe.

## Red flags

- A new entity type that doesn't embed `Entity`.
- A non-ULID primary key (everything is `char(26)`).
- An override column on a base entity — overrides belong in the unified
  `weave_field_overrides` table.
- Reintroducing a merge step in the resolver that collapses base, model, and
  collection rows into one — resolution is copy-on-adopt; only the base
  `SetValue` lock is a real cascade rule.
- Treating `expected_value_type` as override-able — it's a `Field`-level
  structural concept.
- A missing `json:"…,omitempty"` tag on an API-visible field.
