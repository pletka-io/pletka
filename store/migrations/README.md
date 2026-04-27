# Migrations

Goose SQL migrations for the Pletka PostgreSQL schema. Files are
embedded into the binary via `//go:embed` and applied through a
`pletka migrate` subcommand (TBD; lands when the first store
implementation does).

Migration 001 is the baseline (full schema as captured at v0.1).
Every subsequent change is a new numbered migration file. Once a
migration has been applied to any environment, it is immutable —
forward-only changes from then on.

Conventions:

- Filename: `NNN_<short-description>.sql`. Three-digit zero-padded
  number, lowercase snake_case description.
- Always include both `-- +goose Up` and `-- +goose Down` sections.
- Multi-statement blocks (functions, triggers) wrap in
  `-- +goose StatementBegin` / `-- +goose StatementEnd`.
- Never `DROP`-then-`CREATE` an existing column; use `ALTER`.

The runner (in `store/postgres/`) tracks applied migrations in the
standard `goose_db_version` table.
