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
> per ADR-0005 — request-plumbing middleware belongs in `pkg/auth`). See
> `docs/plans/2026-07-06-compliance-debt-backlog.md` item 8.

Generators follow the reader pattern at scale: five narrow readers (`ProjectReader`, `ModelReader`, `CollectionReader`, `FieldReader`, `NamespaceReader`) plus a `*Registry` — never the `domain.WeaveStore` aggregate.

## Dispatcher Rule

A handler-only module that needs a list- or form-schema calls `<slice>.BuildXSchema(...)` — it never reimplements the schema. The slice is the single source of truth (two modules once drifted by doing this in parallel).

## Red Flags

- A full slice importing another full slice directly, other than the `override` exception
- A slice constructing its own `Store` from a pool instead of receiving it via `Host` from `pkg/app`
- A handler-only module growing a `Store` interface — promote it to a full slice or read through an existing slice's service
- Two packages building the same entity's schema — collapse to the slice
- Business logic inside a store method (stores query, services decide)
- Generators depending on the `domain.WeaveStore` aggregate instead of narrow readers
- A handler emitting `http.Error`/`http.NotFound` instead of `weaverouter.Error`/`apierror.Write`
