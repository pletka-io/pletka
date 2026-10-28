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

Migration 022 adds `weave_release_project_dependencies`, keyed by consumer
project/version and source project, and `weave_release_dependency_entities` for
direct foreign membership. A cut captures the transitive project closure in its
transaction, validates referenced bodies against the selected source archives,
and writes concrete source versions into archived adoption receipts. An explicit
pin and its closure constrain unpinned references; otherwise each direct source
selects its latest non-retired release independently before closures are merged.
Conflicting versions of one source project, cycles and unverified sources fail.
Source foreign keys prevent deleting a retained release or its project; retirement
does not break existing pins. Draft rows remain editable.

`project_dependencies_version = 1` certifies this project inventory only. It does
not certify ontology artifacts, frozen actor/module descriptors or complete
serialized output. Old releases stay at zero; `pletka release audit-dependencies`
reports them without guessing pins. New dependent cuts require sources already
captured under this model: cut source projects first, or separately establish
historical evidence. A downgrade refuses to discard captured inventories.

`Session.Dependency(kind, id)` resolves recorded membership and binds a source
session on the parent's transaction and lifetime. Its resolver receives only
captured-membership queries. It cannot look up live owners or latest releases.
Source authorization remains the caller's responsibility. Draft source resolution
and migration of legacy adoption/aggregate callers remain pending.

Migration 021 does not certify historical releases or complete content coverage. Run
`pletka release audit-ownership` for a read-only JSON inventory of
`unverified_release` and `orphan_project_archive` entries. Collision indicators
are investigation leads, not proof of corruption. Historical membership requires
retained artifacts or other verifiable evidence; filling it from current drafts
would invent history. A downgrade is refused while verified release snapshots
exist, because the old key cannot represent them without loss.

Ontology editing follows
[ADR-0013](../decisions/0013-ontology-drafts-and-immutable-releases.md): separate
editable drafts from immutable releases, freeze the complete artifact and its
dependencies, and enforce sealing in storage for every writer. Existing ontology
version IDs alone are not proof of immutability: unsealed versions remain editable.

Migration 023 adds explicit `weave_ontology_body_seals`. The database captures
version metadata/source bytes, terms, relations, companions, catalog descriptors
and global namespace context, computes a digest and guards all body writers.
Child writes and sealing serialize on the version row, including at repeatable
read. Relations now have version/source constraints; unowned historical edges
remain available through `WeaveAuditOrphanOntologyRelations`, and reusing their
term IDs is blocked pending evidence review. New writes cannot create orphans.

The transaction-owning publication layer calls `ontologybody.Seal`. The explicit
operator command is `pletka weave ontology-seal-body <ontologyID> <versionID>`;
ordinary imports now invoke it inside the publication transaction. `ontologybody.Factory.Read`
binds a validated ontology/version reference to a read-only snapshot and a reader
with only generated seal SELECTs from `queries/read_ontology_body/`. It exposes
verified immutable body values, not a store or retargetable scope. Authorization
remains the caller's responsibility. Mutation conflicts use the canonical API
error adapter, with the database cause preserved.

Migration 024 adds immutable `weave_ontology_releases` and derived
`weave_ontology_release_dependencies`. Ordinary imports seal and publish in one
repeatable-read transaction, capturing concrete base/import/default pins and
validating relation/inverse references. Storage requires repeatable read (or
serializable), checks body identity, complete
requirement inventory and source closure; source FKs enforce retention. No
publication, pin, seal or body mutation, truncate or unsafe downgrade is allowed.
Historical versions are not automatically published.

The format-1 publication envelope preserves exact local JSONB body bytes in a
string plus their digest, sorted pins and requirements. SHA256 of exact UTF-8
envelope bytes is the portable artifact identity; publication timestamps and
active policy are excluded. `ontologyrelease.Factory.Read` binds an owner/version
and uses only `queries/read_ontology_release/` SELECTs. `Dependency(owner)` reads
only the captured source identity/digest, never active/latest. Conformance gates
restrict raw publication queries and the explicit legacy restore staging path.

`weave ontology-publish <ontologyID> <versionID>` explicitly publishes current
stored bytes, requiring already-published dependencies; it does not establish
historical truth. See ADR-0013 for resolution rules, external-reference
classification and operator options. Active selection is mutable policy outside
the artifact; draft storage is described below.

Migration 025 captures project ontology identities in
`weave_release_ontology_dependencies`, with a separate provenance marker on the
release. Archived links, explicit version selectors and captured source-project
inventories contribute their full ontology closures. Storage validates coverage,
retains source artifacts and prevents later pin changes. `Factory.ReadProject`
uses only `queries/read_project_ontologies/`; released materialization serializes
returned values. Historical inventories stay unverified. Migration 030 adds
project QName/URI resolution evidence under ADR-0015. Complete content coverage
and binding these readers to the assembler's existing transaction remain pending;
the standalone `ReadProject` factory currently opens its own transaction.

Vendor schema 2 transports exact artifact bytes plus frozen source files.
Migration 026 adds optional exact `body_text` to seals: local seals retain JSONB
rendering, transported seals retain original bytes. Restore verifies hashes,
ownership and indexed body equality in one transaction without reparsing RDF or
changing destination namespace policy. Existing IDs require identical published
bytes; legacy collisions fail. Schema 1 retains its explicit unverified path.

Migration 027 removes `is_active` from version storage. The separate
`weave_ontology_active_versions` table enforces one owner/version selection per
ontology with a composite foreign key. `weave_set_ontology_active_version` is the
atomic operation used by create/import, explicit selection and restore; initialize
only when unset must use this operation rather than a read followed by a write.
Live `weave_ontology_version_status` projects the API's `is_active` flag. Released
readers do not read that projection or current policy. Migration preserves legacy
choices without publishing them and rejects ambiguous evidence. Downgrade copies
the choices back to flags without changing sealed bytes.

Migration 028 adds separate ontology draft documents and immutable publication
receipts. Draft revisions are storage-managed; edits use expected-revision checks
inside a bounded transaction. `readsql/ontologydraft` can only SELECT drafts;
`writesql/ontologydraft` can only lock/save drafts. `ontology.DraftService.Publish`
locks one revision and commits descriptor/namespaces, fresh version-owned content,
seal, dependency closure, active policy and receipt together. It never deletes an
existing version. The draft SELECT capability may read its own immutable receipt
for the current revision; it cannot query released bodies or active policy.
Receipt replay is idempotent only at the same current revision
and request. No historical rows are converted to drafts; downgrade refuses stored
drafts. See [draft operations](../reference/ontology-drafts.md).

## Release output descriptors

Migration 029 stores immutable actor/module documents in
`weave_release_descriptors`. The cut captures owner, creator, receipt and credit
actors plus concrete project/ontology module identifiers in the same repeatable
read transaction as archived bodies, dependencies and outbox. Source modules come
from their pinned releases. One actor-reference view drives both capture and
storage validation. Mutable actors have no foreign keys into these snapshots.

`readsession.Session.Descriptors` uses the archive SELECT capability and returns
detached values; released materialization never consults current actor labels or
host configuration. Missing historical snapshots return `not_in_release` rather
than reconstructing history. Storage rejects mutations, incomplete inventories
and late insertion into verified cuts. See [ADR-0014](../decisions/0014-release-actor-and-module-descriptors.md).

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

## Captured project semantic references

Migration030 records a complete per-release ontology-reference inventory, including
empty inventories, separately from migration025's link/version pins. A shared SQL
view covers scopes, paths, additional types, datatypes and archived subfields. The
cut resolves against selected publications; storage checks unique term matches and
exact coverage. `ontologyrelease.ProjectReader.References` returns detached
owner/version/digest/URI evidence without current-policy access. Missing historical
evidence is `not_in_release`. Source entities retain their source inventories.
See [ADR-0015](../decisions/0015-project-semantic-references-at-release-cut.md).

## Captured project/model metadata

Migration 031 preserves model types and raw/effective concept namespace at the
release cut. `metadata_capture_version=1` certifies only this defined metadata
contract. Storage validates coverage before finalization, protects its archive
rows afterward and refuses destructive downgrade; historical releases remain
unverified. `readsession.Session.Metadata` reads detached values on the existing
session and reports missing evidence as `not_in_release`.

The initial column inventory checks project/model schema and named capture
projections. It records `enforce_concept_lists` as a remaining gap and
`is_core_weave` as live administrative policy. The broader capture audit and
assembler/serializer migration remain pending. See
[capture coverage](../reference/release-capture-coverage.md).
