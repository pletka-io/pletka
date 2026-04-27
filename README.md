# Pletka

Schema-driven web platform for cultural-heritage knowledge: a Go server that emits a hypermedia JSON contract, plus a Svelte renderer that is a pure function of that contract.

> **Status — v0.1.0-dev.** Pletka is in active early development. The schema, the Go API, and the renderer are all expected to change. Pin a release tag if you depend on it.

## What Pletka is

Pletka manages three semantic patterns — **Fields**, **Models**, and **Collections** — that institutions collaboratively create, adopt, and customize. The core data layer (`domain/weave`) is the single source of truth for those patterns; the HTTP layer (`server/`) exposes them through a schema-driven JSON contract; the renderer (`renderer/`) turns that JSON into a UI.

The defining design choice is **hypermedia-driven authorization**: the server emits a link for an action only if the current user is permitted to take it. The renderer never asks "is the user allowed to do X" — it renders what was returned. See [`docs/authorization-model.md`](docs/authorization-model.md).

## Layout

```
cmd/             binaries — cmd/pletka is the HTTP server
domain/          pure types + Store interfaces. No HTTP, no DB drivers.
                 Subpackages: domain/weave, domain/ontology.
                 Shared primitives (Translations, ULID, BaseModel) live
                 at the root and are imported by both subpackages.
store/           PostgreSQL persistence — pgx + sqlc.
                 Subpackages: store/postgres (conn pool, migration
                 runner), store/migrations (goose SQL), store/sqlcgen
                 (generated), store/weave, store/ontology.
gitsync/         bidirectional git ↔ domain sync (per-subsystem plugins)
server/          HTTP layer (library) — handlers, schema builders,
                 link emitters, middleware, auth, embedded frontend
                 bundle. No binary; the binary lives in cmd/pletka.
renderer/        Svelte 5 source — built once, embedded into the
                 server binary
schema/          JSON contract docs + example fixtures, no code
docs/            architecture, protocol, decisions, plans, specs
```

The boundary contract (no cycles):

- `domain/` (root) imports nothing else inside Pletka.
- `domain/weave/` and `domain/ontology/` import `domain/` only — **never each other**.
- `store/` imports `domain/` and the subsystem it implements.
- `gitsync/` imports `store/` + `domain/`.
- `server/` imports `store/` + `domain/` + `gitsync/`.
- `cmd/<tool>/` imports whichever subset it needs — `cmd/pletka/` pulls `server/`; other CLIs (importers, one-shot tools) typically depend only on `store/` + `domain/` + `gitsync/`.
- `renderer/` consumes only the JSON schema (no Go imports).

See [`docs/architecture.md`](docs/architecture.md) for the full DAG and rationale.

## Building from source

Requires Go 1.26+, Node 22+, and npm 10+.

```sh
make build       # builds renderer + Go binary into bin/pletka
make test        # Go + renderer tests
make lint        # golangci-lint + svelte-check
make dev         # local dev with hot reload
```

Single-binary deploy: `bin/pletka` ships the renderer bundle embedded via `//go:embed`.

## Documentation

- [`docs/architecture.md`](docs/architecture.md) — high-level overview
- [`docs/protocol.md`](docs/protocol.md) — schema contract (normative)
- [`docs/authorization-model.md`](docs/authorization-model.md) — hypermedia-driven authz
- [`docs/adding-a-widget.md`](docs/adding-a-widget.md) — how to add a widget
- [`docs/adding-a-relation.md`](docs/adding-a-relation.md) — how to add a link relation
- [`docs/building-a-renderer.md`](docs/building-a-renderer.md) — implementing an alternative renderer
- [`schema/`](schema/) — protocol docs + example fixtures

## Contributing

See [`CONTRIBUTING.md`](CONTRIBUTING.md). All contributions require a [DCO](https://developercertificate.org/) sign-off (`git commit -s`).

## License

Apache License 2.0 — see [`LICENSE`](LICENSE) and [`NOTICE`](NOTICE).
