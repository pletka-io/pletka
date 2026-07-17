# Contributing

## Prerequisites

- Go 1.26+
- PostgreSQL (dev setup expects it on port **5433**)
- Node.js (for the Svelte / Vite frontend)

## Quick start

```bash
cp .env.example .env       # database settings + per-worktree overrides
make frontend-install      # one-time: install frontend dependencies
make air                   # build frontend + run server with hot reload
```

The server starts on port **3333** (`make air PORT=4000` to override). Database
migrations run automatically at boot via goose.

## Common commands

Everything goes through the Makefile — `make help` lists all targets.

| Command | Purpose |
|---|---|
| `make air` / `make dev` | run with hot reload |
| `make build` | build the `pletka` binary into `bin/` |
| `make test` | run the test suite |
| `make typegen` | regenerate TypeScript types from Go (tygo) |
| `make frontend-build` | build the Svelte islands |
| `make db-status` | show database statistics |

After changing SQL queries in `pkg/database/queries/`, run `go tool sqlc generate`
(never a global `sqlc` — see [`../architecture/data-layer.md`](../architecture/data-layer.md)).

## Where things go

- A feature is a **slice** under `pkg/weave/<name>/` — read
  [`../architecture/slices.md`](../architecture/slices.md) before adding one, and
  decide "full slice or handler-only?" first.
- Domain knowledge lives in Go **schema builders**, not in Svelte —
  [`../architecture/schema-driven-ui.md`](../architecture/schema-driven-ui.md).
- A decision that crosses boundaries or rejects a real alternative gets an
  **ADR** — [`../decisions/`](../decisions/).

## Conventions

- Go style: [`../reference/go-style.md`](../reference/go-style.md)
- Naming across layers: [`../reference/naming-conventions.md`](../reference/naming-conventions.md)
- The rules you must not break: [`../principles.md`](../principles.md)

## Working in parallel: worktrees

Each git worktree gets its own database clone so sibling branches don't collide.
See [`worktree-setup.md`](worktree-setup.md).

## AI-assisted development

This repo ships scaffolding that makes AI pair-programming productive and
consistent. It's optional for human contributors but worth understanding — see
[`ai-workflow.md`](ai-workflow.md).
