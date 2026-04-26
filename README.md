# Pletka

Schema-driven web platform for cultural-heritage knowledge: a Go server that emits a hypermedia JSON contract, plus a Svelte renderer that is a pure function of that contract.

> **Status — v0.1.0-dev.** Pletka is in active early development. The schema, the Go API, and the renderer are all expected to change. Pin a release tag if you depend on it.

## What Pletka is

Pletka manages three semantic patterns — **Fields**, **Models**, and **Collections** — that institutions collaboratively create, adopt, and customize. The core data layer (`domain/weave`) is the single source of truth for those patterns; the HTTP layer (`server/`) exposes them through a schema-driven JSON contract; the renderer (`renderer/`) turns that JSON into a UI.

The defining design choice is **hypermedia-driven authorization**: the server emits a link for an action only if the current user is permitted to take it. The renderer never asks "is the user allowed to do X" — it renders what was returned. See [`docs/authorization-model.md`](docs/authorization-model.md).

## Layout

```
schema/          docs + JSON example fixtures, no code
domain/          pure domain (weave, ontology) — no HTTP, no DB driver glue
gitsync/         bidirectional git ↔ domain sync (per-domain plugins)
server/          HTTP layer, schema builders, link emitters, embedded frontend, the binary
renderer/        Svelte 5 source — built once, embedded into the server binary
docs/            architecture, protocol, decisions, plans, specs
```

The boundary contract:

- `domain/` imports nothing else. `gitsync/` imports `domain/`. `server/` imports both. `renderer/` consumes only the JSON schema.

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
