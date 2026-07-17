# Contributing to Pletka

Pletka is an open-source core for schema-driven semantic data applications.

## Quick Start

```sh
git clone git@github.com:pletka-io/pletka.git
cd pletka
cp .env.example .env
make frontend-install   # one-time: install frontend dependencies
make air                # build frontend + run server with hot reload
```

Requires Go 1.26+, PostgreSQL, and Node.js (for the Svelte/Vite frontend).
See [`docs-oss/contributing/README.md`](docs-oss/contributing/README.md) for
the full prerequisites list and Makefile command reference, and
[`docs-oss/contributing/ai-workflow.md`](docs-oss/contributing/ai-workflow.md)
for the AI-assisted development scaffolding this repo ships.

## Architecture

This repo does not restate its own layout here — it drifts. Read
[`docs-oss/architecture/`](docs-oss/architecture/) for the current package
boundaries (`pkg/domain`, `pkg/weave/<slice>`, `pkg/app`, `pkg/database`,
`pkg/auth`, `pkg/session`, `pkg/integrations`, `cmd/`, `frontend/`) and
[`docs-oss/decisions/`](docs-oss/decisions/) for why they are shaped that
way. In short:

- Domain and schema contracts stay platform-neutral.
- Slices under `pkg/weave/` depend on narrow interfaces for the stores and
  services they need.
- Application-specific wiring belongs at the host application boundary
  (`pkg/app/`), not inside a slice.
- Server-side schema builders decide available actions. Clients render
  capabilities and links they receive; they do not infer permissions.

## DCO Sign-Off

Every contribution must include a Developer Certificate of Origin sign-off:

```text
Signed-off-by: Random J Developer <random@developer.example.org>
```

Use `git commit -s` to add it automatically.

## Coding Standards

Go code is formatted with `gofmt` and verified with `go test ./...`
(`make test` runs this). New reusable packages should include focused tests
for their public contracts and boundary behavior. Frontend changes should
pass `npm run check` in `frontend/` (svelte-check).

## Security

See [`SECURITY.md`](SECURITY.md). Do not open public issues for
vulnerabilities.

## Code of Conduct

This project follows the [Contributor Covenant 2.1](CODE_OF_CONDUCT.md).
