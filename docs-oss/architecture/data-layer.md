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
generates Go into `pkg/database/sqlcgen/`. The bound-reader pilot has separate
SELECT-only sources in `queries/read_draft/` and `queries/read_archive/`, generated
into `pkg/database/readsql/draft` and `pkg/database/readsql/archive`. The main
configuration's `*.sql` glob excludes these subdirectories deliberately.

**Always run `go tool sqlc generate`** — never a globally installed `sqlc`. The
correct version is pinned in `go.mod` as a tool dependency; a different binary can
emit incompatible types (e.g. `pgtype.Text` instead of `*string`). The config is
`sqlc.yaml`.

**Never hand-edit `pkg/database/sqlcgen/` or `pkg/database/readsql/`.** They are
generated. Change the `.sql` query and regenerate.

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

Three kinds of pool holder are sanctioned (compliance backlog item 3, resolved):
`pkg/weave/health/handler.go` (a DB liveness ping, not domain data access),
`pkg/weave/pathaudit` (single raw-SQL audit sweep, lifted from the verify-paths CLI),
and tx-owning services (`organization`, `release`) that open `pool.Begin(ctx)`
transactions spanning their own store plus other writes. Each blessed field
carries a `// sanctioned pool holder: <reason>` comment. This is a closed
list — don't add new pool holders; `actoradmin` held the pool the same way
and was refactored into a standard `Store`-backed slice instead of being
blessed.

## Release archives and read sessions

Version strings are local to a project. Existing archive rows commonly join by
entity ID and textual `version_number`; a matching string in another project is
not a release identity. Shared/global state must have membership owned by the
consuming release. Foreign project content is read using its source release,
never copied into a source archive key stamped with the consumer's version.
Migration 021 adds `weave_release_namespace_bindings_archive`, keyed by
`(release_project_id, version_number, id)` with a foreign key to the consuming
release. The original namespace archive is retained as historical evidence and
is no longer a content read source. New cuts archive only their own project,
inheritance links and ontology links; foreign metadata keeps its source version.

`weave_releases.archive_ownership_version = 1` records successful capture under
this model, in the same transaction. Legacy releases remain at zero. The shared
`pkg/database/releasearchive` checks reject unverified ownership; they never
substitute live metadata or shared rows selected only by a version string.
An archive key already populated without release metadata blocks a new cut
instead of silently retaining or overwriting that row.

Release creation uses a repeatable-read transaction covering validation, archive
writes, release metadata and outbox. `pkg/database/readsession.Factory` now
binds project, concrete selection and a shared transaction before exposing
readers. Archive readers receive only archive query capabilities. Multi-query
draft snapshots use read-only repeatable read. Its callback exposes bound slice
readers; callback return closes the transaction, even on failure or cancellation.
Collection and override services receive this factory through slice-specific
`ReadSessions` adapters, separate from their write stores. Their archive readers
hold only the generated archive SELECT interface, with no pool or full query set;
draft readers hold the corresponding draft interface. Conformance checks restrict
raw query access to binding infrastructure and reject SQL crossing states.
Serialization happens after values are assembled, without retaining database
access. See ADRs
[0010](../decisions/0010-versioned-reads-through-a-scope-bound-reader.md),
[0011](../decisions/0011-cross-project-reads-resolve-at-the-source-release.md) and
[0012](../decisions/0012-whole-project-reads-belong-to-an-assembler.md).

The bound session is implemented for the collection/override pilot. Legacy
adoption/aggregate paths and the complete dependency closure remain pending.
Migration 021 does not certify historical releases or complete content coverage. Run
`pletka release audit-ownership` for a read-only JSON inventory of
`unverified_release` and `orphan_project_archive` entries. Collision indicators
are investigation leads, not proof of corruption. Historical membership requires
retained artifacts or other verifiable evidence; filling it from current drafts
would invent history. A downgrade is refused while verified release snapshots
exist, because the old key cannot represent them without loss.

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
