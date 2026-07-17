# Data layer — pgx + sqlc + goose

There is no ORM. The data layer is three tools with clear jobs:

- **pgx** — the PostgreSQL driver and connection pool (`*pgxpool.Pool`).
- **sqlc** — generates typed Go from hand-written SQL queries.
- **goose** — versioned schema migrations, applied at boot.

Each slice owns a `Store` that talks to its table through the generated queries.
Handlers and services never see SQL; the frontend never sees the database.

> **Historical note.** Earlier iterations used GORM with a shared
> `pkg/repository/` layer. That layer has been removed. If you find a doc that
> tells you handlers go through `pkg/repository/` or that table names come from
> struct names, it is stale — the truth is below, and in the code.

## Queries: sqlc

Hand-written SQL lives in `pkg/database/queries/*.sql`, one file per area
(`weave_categories.sql`, `fields.sql`, `weave_collections.sql`, …). `sqlc`
generates Go into `pkg/database/sqlcgen/`.

**Always run `go tool sqlc generate`** — never a globally installed `sqlc`. The
correct version is pinned in `go.mod` as a tool dependency; a different binary can
emit incompatible types (e.g. `pgtype.Text` instead of `*string`). The config is
`sqlc.yaml`.

**Never hand-edit `pkg/database/sqlcgen/`.** It is generated. Change the `.sql`
query and regenerate.

The workflow for a new query:

```
1. write the SQL in pkg/database/queries/<area>.sql
2. go tool sqlc generate
3. use the generated method from a slice store
```

## Stores: one per slice

A full slice owns a `Store` interface (`store.go`) and a pgx-backed
implementation (`store_postgres.go`). The implementation wraps `sqlcgen.Queries`:

```go
// pkg/weave/category/store_postgres.go
type postgresStore struct {
    queries *sqlcgen.Queries
    pool    *pgxpool.Pool
}

var _ Store = (*postgresStore)(nil)

func NewPostgresStore(pool *pgxpool.Pool) Store {
    return &postgresStore{queries: sqlcgen.New(pool), pool: pool}
}
```

The store maps between domain types and generated row types: it generates the
ULID, marshals `Translations` to JSONB, applies empty-to-nil conventions, and
returns clean `domain.*` values. The service above it never sees a `sqlcgen` type.

Stores query; they do not decide. Validation, permissions, and business rules
belong to the service — see [`slices.md`](slices.md).

Two kinds of pool holder are sanctioned (compliance backlog item 3, resolved):
`pkg/weave/health/handler.go` (a DB liveness ping, not domain data access) and
tx-owning services (`organization`, `release`) that open `pool.Begin(ctx)`
transactions spanning their own store plus other writes. Each blessed field
carries a `// sanctioned pool holder: <reason>` comment. This is a closed
list — don't add new pool holders; `actoradmin` held the pool the same way
and was refactored into a standard `Store`-backed slice instead of being
blessed.

## Migrations: goose

Schema changes are numbered SQL files in `pkg/database/migrations/`
(`001_baseline.sql`, `002_create_weave_tables.sql`, … through `070` and
growing). They are embedded in the binary and applied automatically at server
boot (`pkg/app/serverruntime/server.go:326-327` calls `database.Migrate`,
implemented in `pkg/database/goose.go`), and on demand via the
`pletka db migrate` subcommand (`cmd/migrate.go`).

Rules:

- Migration `001` is the baseline. Every later change is a new numbered file.
- Always write both `-- +goose Up` and `-- +goose Down` sections.
- Use `-- +goose StatementBegin` / `StatementEnd` around multi-statement blocks
  (triggers, functions).
- **Never modify a migration that has been applied to any environment.** Add a new
  one. `goose_db_version` tracks what has run.

## PostgreSQL features the schema relies on

- JSONB for multilingual fields (`Translations`) and metadata.
- Partial unique indexes (e.g. unique `system_name` where it is non-empty).
- `jsonb_each_text()` for searching across all language values.
- `COALESCE` with JSONB extraction for multilingual sorting.
- Upserts via `ON CONFLICT … DO UPDATE`.
- Triggers maintaining UI lookup tables (project stats, search).

## Connection

Local development expects PostgreSQL on port **5433**, database `pletka_weave`.
Per-worktree clones override it via
`PLETKA_DB_NAME` — see [`../contributing/worktree-setup.md`](../contributing/worktree-setup.md).
