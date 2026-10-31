# Vertical slices

A feature lives in one directory under `pkg/weave/<name>/`. Open it and you see
the whole feature: its routes, its HTTP handler, its business logic, its data
access, and the schema it emits. Nothing about the feature is scattered across
`handlers/`, `services/`, `models/` trees.

Decision of record: [ADR-0001](../decisions/0001-weave-module-shapes.md).

## Three shapes

Pick one before creating a `pkg/weave/<X>/` directory. Write the answer in the
package `doc.go` so the next contributor knows (enforced convention — see
below).

### Full slice — owns a table

For an entity with its own table and lifecycle.

```
pkg/weave/<entity>/
  doc.go             // package preamble + slice-specific TODOs
  store.go           // Store interface
  store_postgres.go  // pgx + sqlc implementation
  service.go         // permissions, validation, business logic
  handler.go         // HTTP — depends only on *Service
  formschema.go      // list/form schema builders (canonical home for this entity)
  routes.go          // Host + Validate() + Mount(r, host)
```

The slice defines its own `NewPostgresStore(pool)` constructor; `pkg/app` is what calls it during host assembly.

**Current roster:** `attribution, category, collection, example, field,
gitrestoreadmin, members, model, namespacebinding, ontology, organization,
orgmembers, override (no HTTP), project, projectontologyversion, release,
settings, vocabulary, errortracking`.

Named exceptions:
- `vocabulary` — has no `store.go` of its own; it delegates persistence to
  `pkg/weave/vocabconnector`.

`actoradmin` was a named exception (a service with no `store.go`, holding the
pool directly) until the compliance sweep refactored it — see the pool-holder
policy below. It's now a standard full slice with `store.go` /
`store_postgres.go`.

### Handler-only module — composes, owns nothing

For cross-cutting page composers and dispatchers. Layout is just `handler.go`
plus an optional `routes.go`. No store, no service, no table.

**Current roster:** `projectpage, entityschema, detailview, exports, pages,
workspace, visualization, admin, drafts, content, search, authpages, health,
version`.

**Dispatcher rule:** when a handler-only module needs a list- or form-schema for
an entity, it calls `<slice>.BuildXSchema(...)` — it never reimplements the
schema. The slice is the single source of truth. (This rule exists because two
modules once produced the same JSON via parallel code paths and drifted.)

### Shared infrastructure slice — imported directly by peer entity slices

`override` is a full slice (it owns the `overrides`/junction tables) but it
also functions as shared infrastructure: entity slices import its `Store`
type concretely instead of going through a reader interface. For example,
`field`'s `Service` holds an `overrides override.Store` field
(`field/service.go:56`) so write paths can persist the field's base override
in the same `ChangeLogRunner.Run` boundary as the field write. `model`,
`collection`, and `project` do the same.

This is a deliberate, narrow exception to peer isolation. The hard rule is:

> No peer entity slice imports another peer entity slice (e.g. `field` must
> never import `model`). Shared infrastructure slices (`override`) may be
> imported directly.

See [ADR-0006](../decisions/0006-override-shared-infrastructure-slice.md) for
the rationale.

## Wiring model

Each slice exposes:

- a typed `Host` struct — the narrow dependency surface the slice needs
  (stores, readers, logger, i18n, etc.)
- a `Validate() error` method on `Host` that fails fast on missing required
  dependencies
- typically a `Mount(r chi.Router, host Host)` function (or method) that
  wires the slice's routes onto the given router using that host — but see
  below, this shape has named exceptions

`pkg/weave/category/routes.go:18` declares the `Host` struct and its
`Validate()` method; the pattern repeats per slice (see `field/routes.go`,
`model/routes.go`, `collection/routes.go`, and friends).

Three shapes of "Mount" actually exist:

- **22 slices** expose a package-level `Mount(<r|parent|h> chi.Router, host Host)`
  function — the parameter names vary (`r`, `parent`, `h`) but the shape is
  the same.
- **Three slices use the `Slice.Descriptor()` pattern instead of a
  package-level `Mount`:** `category` (`Slice.Descriptor()` at
  `pkg/weave/category/routes.go:64`, mounted via the router-local wrapper
  `mountCategory(r, host)` at `pkg/weave/router/category.go:9`),
  `attribution` (`Slice.Descriptor()` at `pkg/weave/attribution/routes.go:50`,
  wrapper `mountAttribution` at `pkg/weave/router/attribution.go:9`), and
  `namespacebinding` (`Slice.Descriptor()` at
  `pkg/weave/namespacebinding/routes.go:50`, wrappers `mountNamespaceBinding`
  / `mountNamespaceBindingAdmin` at `pkg/weave/router/namespacebinding.go:9,13`).
- **`ontology` splits into three Hosts**, each with its own `Mount*` function:
  `AdminHost`/`MountAdmin` (`pkg/weave/ontology/routes.go:113,133`),
  `APIHost`/`MountAPI` (`:146,162`), and `PagesHost`/`MountPages` (`:173,191`) —
  wired separately in `pkg/weave/router/router.go:253,259,265`.

**`pkg/app` is the assembly layer.** It builds every slice's `Host`,
constructs each slice's concrete `Store` (e.g.
`Store: category.NewPostgresStore(pool)` at
`pkg/app/weave_slice_hosts.go:126`), wires the `EventBus` into the services
that need it, and finally calls
[`router.Mount(parent, projects, errors, options...)`](../../pkg/weave/router/router.go)
(`pkg/weave/router/router.go:119`) to mount every slice on the root chi
router.

Entry point chain: `cmd/serve.go` → `pkg/app` (assembles hosts + stores +
EventBus) → `pkg/weave/router` (mounts every slice's `Host` via its `Mount`
function) → individual slices.

## Reader-interface pattern

For everything other than the `override` exception above, when slice A needs
data from slice B, A declares a narrow reader interface and B's service
satisfies it. Neither package names the other.

```go
// in pkg/weave/field/service.go
type ProjectReader interface {
    GetByID(ctx context.Context, id string) (*domain.Project, error)
}

func NewService(store Store, projects ProjectReader, ...) *Service
```

(`field/service.go:37` declares `ProjectReader`; `pkg/app` wires the concrete
`field.NewService(fieldStore, projectService, ...)`.)

Generators follow the same pattern at a larger scale: `pkg/weave/generators`
depends on six narrow readers — `ProjectReader`, `ModelReader`,
`CollectionReader`, `FieldReader`, `NamespaceReader`, `ConceptEnumReader` —
plus a `*Registry`
(`generators/service.go:68-75`). It never depends on the `domain.WeaveStore`
aggregate.

## Versioned slices

A slice with an `_archive` counterpart is versioned. Its write store and content
readers have different capabilities. Collection and override now expose a bound
`Reader` through `ReadSessions`, injected separately from the write store. Their
`Store.At(scope)` adapters return `LegacyReader` for explicitly registered
adoption, aggregate and mutation paths. Other slice conversions remain pending.

The target in
[ADR-0010](../decisions/0010-versioned-reads-through-a-scope-bound-reader.md),
amended 2026-10-26 and implemented by the pilot, binds project, explicit draft/release selection and database
transaction in a read-session factory. Reader methods do not accept an arbitrary
project to retarget. Archive readers hold only archive query capabilities; their
existence checks do not depend on live rows. The factory is wired by `pkg/app`,
while request policy belongs in `pkg/auth`. Services receive a `ReadSessions`
dependency and use its callback; all readers in that callback share one snapshot.
Reader use after callback return fails because the transaction is closed.

`auth.Release("")`, whitespace-only versions and the zero scope are invalid.
Only request policy deliberately maps a successful empty version lookup to draft
through `auth.ScopeForResolvedVersion`. Converted release reads validate archive
ownership and report missing bodies through canonical `not_in_release` errors.
Structural tests enforce converted methods, capability imports and table state;
actual-cut tests verify isolation after live edits/deletes and reject foreign IDs.

Read-only draft assembly uses repeatable read. Release creation uses one
repeatable-read write transaction for release metadata, archived content and
outbox. Mutation destinations are live, but clone/adopt sources may be released.
A scope selects content; it does not grant authorization.

HTTP, page, visualization and MCP boundaries share `auth.ResolveProjectVersion`.
A lookup failure stops the request; a successful result saying there is no
release is a separate visibility-policy case. Optional version capabilities,
paired `X`/`XVersion` methods and ambient service lookups remain migration debt.

### Reading another project's content

Caller-owned content uses the caller's selection. Foreign readers come from a
dependency resolver and bind the source project. A draft session may resolve an
unpinned source to its latest release once. A released project uses concrete
source identities recorded during its cut; it never resolves latest at read time.
Missing/unpublished sources, unresolved pins and draft dependencies reject a cut.

The closure includes inheritance, adoptions, refs without adoption records,
export picks, ontology versions, defaults and namespace aliases. Shared captured
state needs membership owned by the consuming release. Copying parent metadata
under the child's version or joining global rows solely by textual version number
creates collisions. Migration 021 corrects capture ownership for new cuts and
keeps unverified history unavailable. Migration 022 captures project pins and
direct entity membership at cut time, with source-release retention. The bound
session's `Dependencies` enumerates the concrete closure; `ProjectDependency`
and `Dependency(kind,id)` bind project/entity sources on that same snapshot and
lifetime. Release roots use captured membership only. One source version per project is
allowed in a closure; conflicting pins, cycles and unverified history fail.
The marker covers project dependencies, not ontology artifacts or complete output.
Draft sessions now plan dependencies read-only through the same selection and
conflict logic as capture. Configure source authorization once through
`Factory.WithSourceAuthorization(auth.CheckProjectRead)` for interactive reads,
or an explicit job policy. Missing policy denies traversal. Every source,
including transitive and cached sources, uses current access metadata; pins grant
no permission. Explicit follow-source-draft is rejected by this path. Legacy
adoption/aggregate API migration remains pending.
Collection usage labels for foreign fields still use the recorded ID until that
consumer is migrated to the source reader.
See [ADR-0011](../decisions/0011-cross-project-reads-resolve-at-the-source-release.md).

### Ontology editing starts with the release boundary

The ontology editor must expose separate draft writes and immutable release
readers from the outset. The bound owner is an ontology, not a project passed
through a generic string parameter. Publication freezes source files, terms,
relations, metadata, descriptors, namespaces and concrete dependencies together.
Active/default selection is mutable policy outside the artifact. Storage must
prevent every import/admin/restore path from overwriting a published identity;
an identical restore may be a verified no-op. Migration 023 and `ontologybody`
implement explicit local-body sealing and an ontology-bound reader with only seal
SELECT capabilities. All writers reject changes to a sealed body; unsealed
legacy rows remain editable. Migration 024 and `ontologyrelease` add automatic
publication on ordinary import: the same transaction captures concrete dependency
closure, validates referenced terms and freezes a portable artifact identity.
The bound release reader has only publication SELECTs, and `Dependency(owner)`
can only follow recorded pins. Conformance limits legacy staging to old Git
restore and raw publication queries to the publisher. Project cuts now capture
linked/explicit ontology selections and source-project closures. Use
`Session.Ontologies` to bind verified artifacts and semantic references to the
existing project transaction. `ontologyrelease.Factory.ReadProject` is the
standalone wrapper over that binding;
released materializers never use current descriptor/default/alias queries.
Vendor schema 2 preserves exact artifacts and restore verifies existing IDs.
Migration 027 moves active selection to a separate policy table. Live status
projects `is_active`; create/import/restore and admin selection share one atomic
operation, including initialization only when unset. Migration 028 and
`ontology.DraftService` add separate source-backed draft storage. `Read` exposes a
bound `DraftReader`; `Edit` exposes a revision-checked `DraftEditor` with only
lock/save queries. Neither capability can publish, retarget or retain unrestricted
queries. `Publish` owns the transaction and reuses the body writer/seal/closure
pipeline, with an immutable receipt for retry identity. Operator commands and the schema-driven source editor/API are available.
`EditorHost` receives the lifecycle and narrow discovery/release readers from app
wiring; HTTP adapters have no store/pool/query capability. New editor routes authorize
all reads, schemas and mutations, bind explicit draft identity/revision, and return
canonical errors. Structured term editing and complete assembler/consumer migration
remain pending. Project semantic references are now captured at the release cut
under [ADR-0015](../decisions/0015-project-semantic-references-at-release-cut.md).
See [ADR-0013](../decisions/0013-ontology-drafts-and-immutable-releases.md).

### Whole-project reads belong to an assembler

Interactive readers answer for an entity or filtered page. Whole-project consumers
use one assembler receiving the bound session and emitting a complete value for
serializers without database access. Transitional adapters must share that
transaction, remain private and have a removal task. The assembler consumes the
recorded dependency graph; it does not repair missing historical pins.

`pkg/service/projectread` now implements complete-project loading for all 22
sections and the concrete source closure, with actual-cut parity and isolation
after edits, deletions and source publications. Its explicit binding registry
preserves converted readers and names replacement APIs for temporary adapters.
See the [value and loading contract](../reference/project-snapshot-values.md).
Bounded entity loading and pure contextual composition are also implemented.
Entity loading uses explicit selectors on those same readers; interpretation
metadata stays complete and no full-project fallback is allowed. Consumer
migration remains pending. Portable additions still need matching restore support.
An incomplete fragment cannot be returned as a complete project.
Publication's timestamp-based dirty-state queries require
a separate semantic/performance decision;
search retains its separate read shape. See
[ADR-0012](../decisions/0012-whole-project-reads-belong-to-an-assembler.md).

## EventBus is active

The `EventBus` is wired in `pkg/app` (`pkg/app/weave_slice_hosts.go:427,457`)
and is in real use: the ontology import path publishes
`OntologyVersionImported`/`ProjectOntologyVersionsChanged` events, and the
autocomplete `IndexCache` subscribes to invalidate itself
(`pkg/weave/ontology/autocomplete/cache.go:156`). This is shipped
infrastructure, not a future plan.

## Constructors

Typed dependencies, not an aggregate bag:

```go
func NewService(
    store Store,
    overrides override.Store,
    projects ProjectReader,
    log *slog.Logger,
) *Service
```

## `doc.go` convention

Every `pkg/weave/*` package should declare its shape (full slice / handler-only
/ shared infrastructure) in its `doc.go` preamble so the next contributor
doesn't have to reverse-engineer it. This is enforced by convention, not by
tooling; backfilling packages that don't yet state their shape is tracked in
the compliance backlog.

## `domain.Service` is vestigial

`pkg/domain/service.go:11-16` defines a `Service` interface
(`Name`/`Routes`/`Start`/`Stop`). Nothing is wired through the interface type —
but note `pkg/service/gitmaterializer/service.go:33-38` structurally satisfies
its method set while being invoked via concrete-type calls (`pkg/app/app.go:130-131`).
The interface remains a removal candidate.

## Red flags

- A **handler never touches the pool** — data access goes through a slice `Store`.

  > **Resolved** (compliance backlog item 3): sanctioned exceptions are
  > `health` (DB liveness ping — not domain data access), `pathaudit` (single
  > raw-SQL audit sweep, lifted from the verify-paths CLI), and tx-owning
  > services (`organization`, `release` — hold the pool to open
  > `pool.Begin(ctx)` transactions spanning their own store plus other
  > writes). Each blessed site carries a one-line
  > `// sanctioned pool holder: <reason> — see docs-oss/architecture/slices.md`
  > comment. `actoradmin` was the fourth pool holder and read as unrouted
  > store access rather than a deliberate exception — it was refactored to a
  > standard `Store`-backed full slice instead of being blessed. This list is
  > closed: it is not a license for new pool holders — new slices go through a
  > `Store` from day one.
- A **full slice importing another full slice directly** (not through the
  `override` shared-infrastructure exception) — e.g. `pkg/weave/field`
  importing `pkg/weave/model` directly. Use a reader interface instead.

  > **Resolved** (compliance backlog item 8): `orgmembers` used to import
  > `organization` (`pkg/weave/orgmembers/routes.go:14`, pre-fix) solely for
  > the `organization.RequireEdit` auth middleware. Per ADR-0005 (auth
  > middleware is `pkg/auth`'s job), `RequireEdit`/`RequireRead` moved to
  > `pkg/auth` as `auth.RequireOrgEdit`/`auth.RequireOrgRead`. `orgmembers`
  > no longer imports `organization` at all; `organization`'s own routes and
  > `workspace` (handler-only) call the new `pkg/auth` functions too.
- A **handler-only module growing a `Store`** — promote it to a full slice or
  pull entity data through the relevant slice's service.
- **Two packages building the same entity's schema** — collapse to the slice
  (dispatcher rule above).
- **Business logic inside a store method** — move it to the service.
- **An API URL constructed in frontend code** — URLs come from the schema (see
  [`schema-driven-ui.md`](schema-driven-ui.md)).

Project semantic references are captured centrally at release creation, not by
individual slice readers. The bound ontology inventory exposes exact resolved
identities for the assembler; source references remain source-owned. New
semantic fields must extend the shared inventory and its storage checks. See
[ADR-0015](../decisions/0015-project-semantic-references-at-release-cut.md).

### Draft mutation ownership

Enrolled project edits enter `service/draftcapture`. Stores route SQL through
`txscope.New(pool)` using the callback context; nested transactions are savepoints.
Bound readers borrow the same transaction. Storage guards enforce coverage across
all 22captured tables, including cascades; no slice is responsible for remembering
to emit a complete portable patch. The existing domain audit remains an intent log.

Delayed writers take retained `Receipt.Patch()` values. Global context refresh has
its own attributed operation. Normal traffic is not enrolled until coordinated Git
baseline and recovery support lands. See
[ADR-0020](../decisions/0020-draft-mutations-own-their-capture.md).
