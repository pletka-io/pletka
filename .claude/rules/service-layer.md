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
selections fail. Current `Store.At(scope)` implementations are transitional;
collection and override still retain access to live stores/generated queries.
Do not treat their signatures as proof of isolation or copy that capability leak.

- Read construction belongs in the session factory, wired by `pkg/app`. Services
  receive bound readers; request policy belongs at HTTP/MCP/CLI boundaries.
- Archive readers hold narrow archive query capabilities, not a full write store,
  pool or unrestricted `sqlcgen.Queries`. Released existence checks use archives.
- Mutation destinations are live. Clone/adopt sources may be released; name the
  source selection explicitly instead of automatically substituting `Draft()`.
- Multi-query draft assembly uses one read-only repeatable-read transaction.
  Release cuts atomically capture metadata, content and outbox at repeatable read.
- Latest-release lookup errors stop the request. A successful empty lookup is a
  different policy case. Use `auth.ResolveProjectVersion` at request boundaries.
- The current conformance ratchet checks signatures and selected files; it cannot
  prove query isolation. The target includes an explicit monotonic conversion
  registry, capability/import checks and edit/delete-after-cut tests.

### Reading another project's content

[ADR-0011](../../docs-oss/decisions/0011-cross-project-reads-resolve-at-the-source-release.md)
requires foreign readers to resolve through the dependency boundary. Caller-owned
reads retain the caller's selection. A draft session may resolve an unpinned
source's latest release once; released consumers use concrete pins recorded at
cut time. No release or failed lookup is an error, never a foreign draft read.

Never compose `auth.Release(recordedVersion)` for a foreign source: empty currently
means draft. All content readers must bind project identity, not only foreign
scopes. Cuts must freeze the transitive dependency closure, including refs/picks,
ontologies, defaults and aliases. Explicit draft-following is a draft workflow
only; unresolved/draft dependencies cannot be cut into a release. Shared archive
membership belongs to the consuming release, never a global textual version key.

### Whole-project reads belong to the assembler

The assembler of
[ADR-0012](../../docs-oss/decisions/0012-whole-project-reads-belong-to-an-assembler.md)
receives the bound session and emits complete domain values. It is planned, not
yet implemented. Consumers serialize these values without database access.

- A slice never grows whole-project assembly or its own export-shaped data model.
- Transitional adapters must share the transaction and be registered for removal.
- Foreign content retains its source's status and provenance.
- Entity exports may assemble a smaller graph through the same session.
- Publication dirty-state semantics and search remain explicit separate concerns.
- Writer parity is insufficient: inventory coverage and test immutability after
  live edits, deletions and new source releases.

## Red Flags

- A full slice importing another full slice directly, other than the `override` exception
- A slice constructing its own `Store` from a pool instead of receiving it via `Host` from `pkg/app`
- A handler-only module growing a `Store` interface — promote it to a full slice or read through an existing slice's service
- Two packages building the same entity's schema — collapse to the slice
- Business logic inside a store method (stores query, services decide)
- Generators depending on the `domain.WeaveStore` aggregate instead of narrow readers
- A handler emitting `http.Error`/`http.NotFound` instead of `weaverouter.Error`/`apierror.Write`
- A read method outside a `Reader` in a versioned slice
- A SQL string literal anywhere but `pkg/database/queries/*.sql` — see `database-patterns.md`
- `ReadScopeFromContext` called anywhere but a handler or an MCP tool
- A paired `X` and `XVersion` on a public surface — a caller that can choose is a caller that can choose wrong
- `auth.Release(v)` built from a version recorded on an adoption, without the error path — an empty `v` is `auth.Draft()`, pointed at someone else's project
- A whole-project or export-shaped read on a slice — that belongs to the assembler
- A whole-project consumer reading an `_archive` table directly instead of taking a snapshot
- A new `_archive` table whose slice is not registered in `pkg/conformance/slices.go`
