# Database Patterns — Design Contract

Read `docs-oss/architecture/data-layer.md` before implementing or modifying any
database, store, or query code.

## Core Principle: Slice Stores Encapsulate All Data Access

The data layer is **pgx + sqlc + goose — there is no ORM.** Handlers never touch
the `*pgxpool.Pool` directly. Each full slice (`pkg/weave/<entity>/`) owns a
`Store` interface (`store.go`) and a pgx-backed implementation
(`store_postgres.go`) that wraps sqlc-generated queries. Services call the store;
handlers call the service.

Red flags that the store boundary is leaking:
- Handler or service code holding a `*pgxpool.Pool` or `*sqlcgen.Queries`
- SQL strings, or `sqlcgen` types, used outside a slice's `store_postgres.go`
- Business logic in a store method (stores query; they don't decide — that's the service)
- A handler-only module growing its own `Store` (it must pull through a slice service)
- Any reference to GORM, `*gorm.DB`, `pkg/repository/`, or `BaseRepository` —
  that layer was removed; it does not exist

Sanctioned exceptions (resolved compliance backlog item 3): `pkg/weave/health/handler.go`
(DB ping), `pkg/weave/pathaudit` (single raw-SQL audit sweep, lifted from the
verify-paths CLI), and tx-owning services (`organization`, `release`) hold the pool by
design — see `docs-oss/architecture/data-layer.md`. Everything else goes
through a slice `Store`; this is a closed list, not a pattern to copy for new
code.

## Query Rules (sqlc)

- Hand-written SQL lives in `pkg/database/queries/*.sql` (one file per area).
- **Always run `go tool sqlc generate`** — never a global `sqlc` binary (the
  version is pinned in `go.mod`; a different one emits incompatible types).
- **Never hand-edit `pkg/database/sqlcgen/`** — it is generated. Change the
  `.sql` query and regenerate.
- Stores map between domain types and generated row types; services never see
  a `sqlcgen` type.

## Model Rules

- All domain types embed one flat `Entity` (`pkg/domain/entity.go`) — ULID
  `ID`, `CreatedAt`/`UpdatedAt`, `UIName`/`Description` (Translations),
  `SystemName`, `SemanticID`, `ProjectID`, `VersionNumber`, `Status`,
  `Deprecated`. There is no separate base/versioned embed split — one flat struct.
- IDs are ULIDs (`char(26)`), generated in code via the domain ULID helper.
- Multilingual fields use `Translations` (JSONB map of language code → text;
  implements `driver.Valuer` / `sql.Scanner`).
- Domain structs carry **only JSON tags** — column mapping lives in the sqlc
  queries, not in struct tags. Use `json:"-"` for internal fields, `omitempty`
  for optional API fields.
- No soft deletes — hard deletes + an audit/change log for versioned entities.

## Migration Rules (goose)

- All schema changes are versioned `.sql` files in `pkg/database/migrations/`,
  embedded in the binary and applied automatically at server boot and via the
  `pletka db migrate` subcommand (also `pletka db migrate status` / `pletka db migrate down`).
- Migration 001 is the baseline. Every change after it is a **new numbered file**.
- **Never modify a migration already applied to any environment** — add a new one.
- Always write both `-- +goose Up` and `-- +goose Down`.
- Use `-- +goose StatementBegin` / `-- +goose StatementEnd` for multi-statement
  blocks (triggers, functions).
- Legacy bare-named tables (`projects`, `fields`, `categories`, `models`,
  `collections`, `model_fields`, `collection_fields`) from the pre-refactor
  GORM schema were dropped for good in migration 069 — they do not coexist
  with the `weave_`-prefixed tables. Do not model a new table on that old
  bare-name convention.

## PostgreSQL Features Used

- JSONB for multilingual fields (`Translations`) and metadata.
- Partial unique indexes (e.g. `WHERE system_name <> ''`).
- `jsonb_each_text()` for searching across all language values.
- COALESCE with JSONB extraction for multilingual sorting.
- Upsert via `ON CONFLICT ... DO UPDATE`.
- Triggers maintaining UI lookup tables (project stats, search).
