# ADR-0001: Two module shapes inside pkg/weave/

Date: 2026-04-29
Status: Accepted

---

## Why this decision was needed

The vertical-slice migration evolved two parallel patterns that briefly
overlapped on the same routes (`/projects/data`, `/projects/entity-list-schema`,
`/projects/filters/institutions`, `/projects/form-schema/project` were
implemented in **both** `pkg/weave/project/` and `pkg/weave/projectpage/`+
`pkg/weave/entityschema/`). chi did not panic — one silently shadowed the
other — but two implementations of the same endpoint is dead code waiting
to drift. The trigger was the post-rebase realisation that `projectpage/`
+ `entityschema/` (extracted on main) and `project/` (built on the slice
branch) both had handlers for the same routes.

A choice was unavoidable: collapse to one pattern, or scope the two
patterns and document the rule.

## What we decided

`pkg/weave/` contains **two module shapes**, distinguished by intent:

1. **Full slice** — for entities (Field, Model, Collection, Project,
   Category, NamespaceBinding, Override, ProjectOntologyVersion). Owns
   its own data layer.

   Layout:
   ```
   pkg/weave/<entity>/
     doc.go            // Package preamble + slice-specific TODOs
     store.go          // Store interface
     store_postgres.go // pgx + sqlc impl
     service.go        // Permissions, validation, business logic
     handler.go        // HTTP — depends only on *Service
     formschema.go     // Slice-owned list/form schema builders
     routes.go         // Mount + MountWithDeps
   ```

   Slice constructs its own `Store` from `deps.Pool` and never reaches
   into `deps.Weave` for its own entity. Cross-slice reads go through
   reader interfaces defined in the slice that consumes them.

   > **Amendment (2026-07-06):** the `deps.Pool`/`MountWithDeps` wiring
   > described above was replaced in June 2026 by typed per-slice `Host`
   > structs assembled in `pkg/app`. The two module shapes remain the
   > decision; the wiring mechanism is historical — see
   > `../architecture/slices.md` for the current model.

2. **Handler-only module** — for cross-cutting dispatchers and page
   composers that don't own data (the page-schema endpoint composes
   schemas from multiple slices; the entity-view detail composes data
   from multiple slices; the entity-list-schema dispatcher routes by
   `entityType`). Layout: handler.go + (optional) routes.go. Uses
   `deps.Weave.*` directly. No own store, no own service layer.

**Dispatcher rule:** when a handler-only module needs a list-schema or
form-schema for an entity, it MUST call `<slice>.BuildXSchema(...)`
rather than reimplement the schema. The slice is the single source of
truth for its own schema.

| Module | Shape | Owns |
|---|---|---|
| `category/` | Full slice | Category CRUD + lifecycle + schemas |
| `field/` | Full slice | Field CRUD + schemas |
| `model/` | Full slice | Model CRUD + schemas |
| `collection/` | Full slice | Collection CRUD + schemas |
| `project/` | Full slice | Project read CRUD + schemas |
| `namespacebinding/` | Full slice | Binding CRUD + schemas |
| `override/` | Full slice (no HTTP) | Override data layer + service |
| `projectontologyversion/` | Full slice | Link CRUD + schemas |
| `projectpage/` | Handler-only | `/projects/{pid}/page-schema` + `/overview-schema` |
| `entityschema/` | Handler-only | Cross-entity dispatchers (`form-schema/{entityType}`, `entity-list-schema/{entityType}`, model/collection options) |
| `detailview/` | Handler-only | `/projects/{pid}/entity-view/{type}/{id}` and stats |
| `pages/` | Handler-only | gohtml page renderers |

## What we considered and rejected

- **One pattern: collapse all slices into handler-only modules.**
  Rejected. Loses the architectural boundary the slice template buys
  (testable Store interface, swappable impl, validation/permission
  boundary). The full-slice slices have already paid the cost of their
  layout; throwing it away to match `projectpage/`'s simpler shape
  would re-entangle data access with HTTP.

- **One pattern: promote `projectpage/`/`entityschema/` to full
  slices.** Rejected. Page-schema and overview-schema compose data
  across multiple slices (project metadata + stats + linked ontologies
  + warnings). They have no own table, no own writes, no own
  permission rules beyond what the underlying entities already
  enforce. Wrapping them in a Store/Service would be ceremony with no
  payoff. Same for entity-view (detailview) and the dispatcher.

- **Reorganise `pkg/weave/` into `pkg/weave/slices/` +
  `pkg/weave/dispatchers/` to make the distinction physical.**
  Rejected for now. The split is real but a folder rename touches every
  import in the tree. Revisit after Field/Model/Collection slices
  land — the value of physical separation grows when ~10 modules are
  in pkg/weave/.

- **Allow handler-only modules to reimplement schemas inline.**
  Rejected. Already burned us once when both `projectpage.ProjectsData`
  and `project.Data` produced the same JSON via parallel code paths.
  The dispatcher rule is what prevents that recurrence.

## Consequences

- **Easier:**
  - Future contributors have a clear answer for "where does this go?"
    — does it own data, or does it compose? Two-tier flowchart.
  - Dispatcher modules stay small (a few hundred lines) and obviously
    don't own state. Pull-request review is faster.
  - Schemas have one canonical home (the slice). Cross-cutting
    dispatchers can't drift from it because they call into it.

- **Harder:**
  - Slight ceremony when adding a new entity-shaped concern: must
    write the full slice template (7 files) even when the first
    landing is small. The Project slice's "read-only first landing"
    pattern is still the escape hatch — store + service in place,
    handler ships only what's ready.
  - Handler-only modules can be tempted to grow data access logic
    inline. Code review must catch direct sqlc usage outside slice
    stores.

- **Constrained:**
  - No new `pkg/weave/<X>/` directory without first deciding "full
    slice or handler-only?" — answer goes in the package doc.go.
  - `entityschema/`, `projectpage/`, `detailview/`, `pages/` cannot
    grow Store interfaces of their own. If they need entity data,
    they consume the relevant slice's Service.
  - When a slice exposes a list/form schema builder, dispatchers
    delegate. No parallel `formschema.BuildProjectXSchema()` calls
    from dispatchers when `project.BuildXSchema()` exists.

## References

- Architecture: [`../architecture/slices.md`](../architecture/slices.md) — the
  slice layout and cross-slice reader-interface convention this ADR refines.
