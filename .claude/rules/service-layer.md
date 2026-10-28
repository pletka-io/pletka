# Service Layer — Design Contract

Read `docs-oss/architecture/slices.md` before implementing or modifying services, handlers, or data access code.

## Core Principle: A Slice Is One Directory, Wired by `pkg/app`

A feature lives entirely in `pkg/weave/<name>/` — routes, handler, business logic, data access, schema. Pick a shape before creating the directory; write the answer in `doc.go`.

| Shape | When to use | What it owns |
|-------|-------------|--------------|
| **Full slice** | Entity with its own table and lifecycle | `store.go` + `store_postgres.go`, `service.go`, `handler.go`, `formschema.go`, `routes.go` (Host + Validate + Mount) |
| **Handler-only module** | Cross-cutting page composer/dispatcher | `handler.go` + optional `routes.go` — no Store, no Service, no table |
| **Shared infrastructure slice** | A full slice imported concretely by peer entity slices (only `override`, per ADR-0006) | Same layout as a full slice, plus a narrow, deliberate exception to peer isolation |

Full-slice roster: `actoradmin, attribution, category, collection, example, field, gitrestoreadmin, members, model, namespacebinding, ontology, organization, orgmembers, override (no HTTP), project, projectontologyversion, release, settings, vocabulary, errortracking`. Handler-only roster: `projectpage, entityschema, detailview, exports, pages, workspace, visualization, admin, drafts, content, search, authpages, health, version`.

## Wiring: Host / Validate / Mount, assembled by `pkg/app`

Most slices expose a typed `Host` struct (its dependency surface), a `Host.Validate() error`, and a package-level `Mount(chi.Router, Host)`. A few slices deviate from this literal shape (the `Slice.Descriptor()` pattern, or multiple Hosts/Mounts) — see [`docs-oss/architecture/slices.md`](../../docs-oss/architecture/slices.md) for the full exception list. **`pkg/app` is the assembly layer**: it constructs every slice's concrete `Store` from the pgx pool, builds each `Host`, wires the `EventBus`, and calls `router.Mount(...)` to attach every slice. A slice never constructs its own store from a raw pool — store construction happens in `pkg/app` wiring, not in the slice.

> **Resolved** (compliance backlog item 3): sanctioned exceptions are
> `health` (DB ping), `pathaudit` (single raw-SQL audit sweep, lifted from the
> verify-paths CLI), and tx-owning services (`organization`, `release`) —
> they hold the pool by design. Everything else goes through a slice
> `Store`; `actoradmin` was refactored accordingly (`store.go` /
> `store_postgres.go`, no `*pgxpool.Pool` field on its `Service`). This is a
> closed list, not license for new pool holders.

Entry chain: `cmd/serve.go` → `pkg/app` (hosts + stores + EventBus) → `pkg/weave/router` (mounts each `Host` via `Mount`) → slices.

Constructor pattern (typed dependencies, not an aggregate bag):

```go
func NewService(store Store, overrides override.Store, projects ProjectReader, log *slog.Logger) *Service
```

## Peer Isolation and the `override` Exception

No peer entity slice imports another peer entity slice directly (`field` must never import `model`) — use a reader interface instead:

```go
// in pkg/weave/field/service.go
type ProjectReader interface {
    GetByID(ctx context.Context, id string) (*domain.Project, error)
}
func NewService(store Store, projects ProjectReader, ...) *Service
```

`override` is the one named exception (ADR-0006): `field`, `model`, `collection`, and `project` import `override.Store` / `*override.Service` / `override.Diff` concretely so writes can persist the base override in the same transactional boundary as the entity write. This is scoped to `override` alone — a second concrete peer import needs its own ADR, not a citation of this one.

> **Resolved** (compliance backlog item 8): `orgmembers` no longer imports
> `organization`. The `RequireEdit`/`RequireRead` auth middleware moved out
> of `organization` into `pkg/auth` (`auth.RequireOrgEdit`/`auth.RequireOrgRead`,
> per ADR-0005 — request-plumbing middleware belongs in `pkg/auth`). The
> change is done; the planning doc it cited does not live in this repo.

Generators follow the reader pattern at scale: six narrow readers (`ProjectReader`, `ModelReader`, `CollectionReader`, `FieldReader`, `NamespaceReader`, `ConceptEnumReader`) plus a `*Registry` — never the `domain.WeaveStore` aggregate.

## Dispatcher Rule

A handler-only module that needs a list- or form-schema calls `<slice>.BuildXSchema(...)` — it never reimplements the schema. The slice is the single source of truth (two modules once drifted by doing this in parallel).

## Versioned Slices

A slice owning an `_archive` counterpart is versioned. The target contract in
[ADR-0010](../../docs-oss/decisions/0010-versioned-reads-through-a-scope-bound-reader.md)
binds **project, explicit draft/release selection and transaction** before exposing
readers. Reader methods cannot retarget another project. Zero/empty release
selections fail. Collection and override implement this through `ReadSessions`
callbacks. Their `Store.At(scope)` returns `LegacyReader` for registered migration
paths, which retain unrestricted capabilities. New content reads use the bound
`Reader` API; never extend the legacy API for a newly converted method.

- Read construction belongs in `pkg/database/readsession`, wired by `pkg/app`.
  Services receive a separate `ReadSessions` dependency and use its callback.
  Collection and override readers in one callback share a transaction; never
  retain a reader after return. Request policy belongs at HTTP/MCP/CLI boundaries.
- Archive readers hold narrow archive query capabilities, not a full write store,
  pool or unrestricted `sqlcgen.Queries`. Released existence checks use archives.
- Mutation destinations are live. Clone/adopt sources may be released; name the
  source selection explicitly instead of automatically substituting `Draft()`.
- Multi-query draft assembly uses one read-only repeatable-read transaction.
  Release cuts atomically capture metadata, content and outbox at repeatable read.
- Latest-release lookup errors stop the request. A successful empty lookup is a
  different policy case. Use `auth.ResolveProjectVersion` at request boundaries.
- The pilot conformance registry checks all production files in the converted
  slices, held capabilities, raw query imports and cross-state/write SQL. Keep
  converted methods in that registry; adding an exception cannot remove them.
  Actual-cut tests cover foreign IDs, live deletion and snapshot consistency.
  Remaining slices still have the weaker signature ratchet.

### Reading another project's content

[ADR-0011](../../docs-oss/decisions/0011-cross-project-reads-resolve-at-the-source-release.md)
requires foreign readers to resolve through the dependency boundary. Caller-owned
reads retain the caller's selection. A draft session may resolve an unpinned
source's latest release once; released consumers use concrete pins recorded at
cut time. No release or failed lookup is an error, never a foreign draft read.

`auth.Release("")` and whitespace-only versions are invalid. Only resolved request
policy uses `auth.ScopeForResolvedVersion` to deliberately select draft on empty.
Foreign reads still need the dependency resolver: strict scope construction alone
does not validate a source pin or authorize access. All content readers must bind project identity, not only foreign
scopes. Cuts must freeze the transitive dependency closure, including refs/picks,
ontologies, defaults and aliases. Explicit draft-following is a draft workflow
only; unresolved/draft dependencies cannot be cut into a release. Shared archive
membership belongs to the consuming release, never a global textual version key.

Project pins and direct membership now live in migration 022's tables. Released
source readers use `Session.Dependency(kind, id)` on the same transaction and
lifetime; they never select latest. Conflicting source versions, cycles and
unverified inventories fail. Source pins retain the referenced release. This
does not migrate all legacy consumers or certify ontology/output completeness.

### Ontology editor boundary

[ADR-0013](../../docs-oss/decisions/0013-ontology-drafts-and-immutable-releases.md)
requires ontology-bound draft and immutable release capabilities from the start.
Freeze metadata, terms/relations, source/companion files, descriptors, namespace
aliases and dependency pins as one artifact. Keep active/default policy separate.
Enforce sealing in storage across importer, admin, editor, CLI and restore;
neither a force flag nor an existing version ID proves immutable content.
Restore must verify content identity before treating an existing ID as a no-op.
Migration 023 now enforces explicit local-body seals across all writers. Use
`ontologybody.Factory.Read` for sealed bodies: its ontology-specific reference
cannot become a project scope, and its reader has only generated seal SELECTs.
The transaction-owning publication boundary calls `ontologybody.Seal`; operators
can explicitly seal current bytes with `weave ontology-seal-body`.
Migration 024 and `ontologyrelease` now publish ordinary imports atomically with
their concrete base/import/default closure and term-reference inventory. Use
`ontologyrelease.Factory.Read` for publications and its captured-only
`Dependency(owner)` method for sources. Raw publication queries belong only to the
publisher. `StageLegacyVersion` is confined to old Git restore; it cannot mint a
published identity. Exact external-reference classification is explicit. Operators
may publish current stored bytes with `weave ontology-publish`; neither operation
certifies historical content. Project cuts capture linked/explicit ontology
selections and source-project closures centrally; use `Factory.ReadProject` for
their immutable artifact values. Released materialization has no legacy fallback.
Vendor schema 2 and restore preserve exact bytes and reject existing-ID conflicts;
never send schema 2 through the legacy RDF importer. Active selection lives in
`weave_ontology_active_versions`, not version content. Use the shared selection
operation; initialization must call `SetActiveVersionIfUnset`, never read then
write. `is_active` is derived live status, not release evidence. Editor drafts use
`ontology.DraftService` with separate draft IDs, source documents and expected
revisions. `Read`/`Edit` expose only bound draft capabilities, with separate generated
SELECT and lock/save packages. `Publish` commits through the shared body writer,
seal and dependency publisher; never delete/reuse a release ID or write version
rows from a draft editor. Keep HTTP authorization at the request boundary. The source editor/API now uses `EditorHost` and the existing schema renderer;
all routes are super-admin-gated and mutations require JSON. Keep schema and form
decoding on the same binding inventory; preserve unexposed draft settings. Expose
current-revision receipt status through the draft reader, never live version policy.
Structured term editing and complete assembler/consumer migration remain open.
Project semantic references are captured at cut time under ADR-0015. See
[draft operations](../../docs-oss/reference/ontology-drafts.md).

### Whole-project reads belong to the assembler

The assembler of
[ADR-0012](../../docs-oss/decisions/0012-whole-project-reads-belong-to-an-assembler.md)
receives the bound session and emits complete domain values. It is planned, not
yet implemented. Consumers serialize these values without database access.

- A slice never grows whole-project assembly or its own export-shaped data model.
- Transitional adapters must share the transaction and be registered for removal.
- Bind ontology/dependency readers to that session; do not nest read factories.
- Complete values require every registered section; fragments have a separate shape.
- Foreign content retains its source's status and provenance.
- Entity exports may assemble a smaller graph through the same session.
- Render captured term identities; use a QName only when its binding matches the
  exact URI. Released composition cannot introduce globally discovered live content.
- Publication dirty-state semantics and search remain explicit separate concerns.
- Writer parity is insufficient: inventory coverage and test immutability after
  live edits, deletions and new source releases.

The first capture prerequisite is implemented: `Session.Metadata` reads certified
model types and raw/effective concept namespace on the existing release session.
Require this evidence when consuming those fields; do not infer historical model
types from archive defaults. This contract is not a complete-project certificate.
Update the [column inventory](../../docs-oss/reference/release-capture-coverage.md)
when changing any table, owned-content column, capture projection, ownership
predicate or archive key. The ledger covers all 22 owned archive sections and
classifies every other table. Domain/portable gaps recorded there still need
explicit assembler values and restore support; do not drop them through a partial
domain DTO or infer whole-project completeness from metadata contract 1.

## Red Flags

- A full slice importing another full slice directly, other than the `override` exception
- A slice constructing its own `Store` from a pool instead of receiving it via `Host` from `pkg/app`
- A handler-only module growing a `Store` interface — promote it to a full slice or read through an existing slice's service
- Two packages building the same entity's schema — collapse to the slice
- Business logic inside a store method (stores query, services decide)
- Generators depending on the `domain.WeaveStore` aggregate instead of narrow readers
- A handler emitting `http.Error`/`http.NotFound` instead of `weaverouter.Error`/`apierror.Write`
- A read method outside a `Reader` in a versioned slice
- A SQL string literal outside named `.sql` files under `pkg/database/queries/` — see `database-patterns.md`
- `ReadScopeFromContext` called anywhere but a handler or an MCP tool
- A paired `X` and `XVersion` on a public surface — a caller that can choose is a caller that can choose wrong
- A foreign read bypassing dependency resolution, or `ScopeForResolvedVersion` used to turn an absent foreign pin into draft
- A whole-project or export-shaped read on a slice — that belongs to the assembler
- A whole-project consumer reading an `_archive` table directly instead of taking a snapshot
- Released output resolving actor labels or module hosts from current state; use `readsession.Session.Descriptors` (ADR-0014)
- Released semantic references resolved through current ontology policy or prefix guesses; capture at the cut and read `ontologyrelease.ProjectReader.References` (ADR-0015)
- A new `_archive` table whose slice is not registered in `pkg/conformance/slices.go`
