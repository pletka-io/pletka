# Architecture

A high-level view of how Pletka is shaped. For the protocol details see
[`protocol.md`](protocol.md). For the load-bearing authorization pattern
see [`authorization-model.md`](authorization-model.md).

## Layers

```
┌──────────────────────────────────────────────────────────┐
│                       renderer/                          │  Svelte 5
│  Pure consumer of the JSON schema. No business logic.    │
└──────────────────────────────────────────────────────────┘
                            │
                            ▼  HTTP, JSON schema responses
┌──────────────────────────────────────────────────────────┐
│                        server/                           │  Go (chi)
│  HTTP routing, schema builders, link emitters,           │
│  auth, sessions, embedded renderer bundle, the binary.   │
└──────────────────────────────────────────────────────────┘
        │                                       │
        ▼                                       ▼
┌────────────────────┐               ┌──────────────────────┐
│      gitsync/      │  ── reads ──▶ │       domain/        │
│  Bidirectional     │               │  Pure semantic core: │
│  git ↔ domain      │  ◀── writes ──│  weave, ontology.    │
└────────────────────┘               └──────────────────────┘
```

## Boundary contract

The boundary is mechanical, not aspirational:

- **`domain/`** imports nothing else inside Pletka. It owns the semantic
  patterns (Fields, Models, Collections), categories, projects, ontology
  types, and the override chain. No HTTP, no DB driver glue, no JSON shape.
- **`gitsync/`** imports `domain/` only. It serialises domain state into
  git and rehydrates domain state from git. Per-domain plugins live under
  `gitsync/<domain>/`.
- **`server/`** imports `domain/` and `gitsync/`. It exposes the JSON
  schema contract over HTTP, emits hypermedia links scoped to the current
  user, and embeds the renderer bundle for single-binary deploys.
- **`renderer/`** consumes only the JSON schema. It never touches Go types;
  it never decides authorization; it never constructs API URLs.

If a PR violates any of these arrows, fix the architecture, not the lint
rule.

## The schema is the contract

Pletka's distinctive design choice is that **the JSON schema returned by
the server is the entire contract** with the renderer:

- Domain knowledge lives in Go schema builders (`server/widgets/`).
- The renderer is a generic dispatcher over widget `type`.
- Authorization is hypermedia-driven: the server emits a link for an
  action only if the current user is permitted to take it. See
  [`authorization-model.md`](authorization-model.md).

Concrete consequence: alternative renderers (mobile, terminal, third-party)
can be built in any language and will work, given the same schema.

## Single-binary deploys

The renderer is built once at release time and embedded into the server
binary via `//go:embed`. There is no Node runtime in production. Local
development uses `make dev` to run the renderer in watch mode alongside
`go run`.

## Versioning

- The Go module is versioned by Git tags (`v0.1.0`, `v0.2.0`, ...).
  Pletka stays on `v0.x.y` until the API surface is genuinely stable.
- The schema is versioned independently in `schema/version.txt` and
  `schema/CHANGELOG.md`. Minor schema bumps add fields; major bumps
  remove or rename them. See [`protocol.md`](protocol.md).

## Pointers

- [`protocol.md`](protocol.md) — normative schema contract.
- [`authorization-model.md`](authorization-model.md) — the hypermedia
  authorization pattern.
- [`adding-a-widget.md`](adding-a-widget.md) — the four-file recipe.
- [`adding-a-relation.md`](adding-a-relation.md) — link relation recipe.
- [`building-a-renderer.md`](building-a-renderer.md) — implementing an
  alternative renderer.
- [`decisions/`](decisions/) — ADRs.
- [`plans/`](plans/) — implementation plans, dated.
