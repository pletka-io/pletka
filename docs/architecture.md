# Architecture

A high-level view of how Pletka is shaped. For the protocol details see
[`protocol.md`](protocol.md). For the load-bearing authorization pattern
see [`authorization-model.md`](authorization-model.md).

## Layers

```
                ┌──────────────────────────────────────┐
                │              renderer/               │  Svelte 5
                │   Pure consumer of the JSON schema   │
                └──────────────────────────────────────┘
                                  │ HTTP, JSON schema
                                  ▼
                ┌──────────────────────────────────────┐
                │               server/                │  HTTP library
                │   widgets, relations, handlers,      │  (no binary —
                │   middleware, auth, embedded bundle  │   binaries live
                └──────────────────────────────────────┘   in cmd/)
                          │           │
                          │           │
                          ▼           ▼
              ┌────────────┐    ┌──────────────────┐
              │  gitsync/  │───▶│       store/     │  pgx + sqlc impl
              │ git ↔      │    │  postgres conn,  │  of the Store
              │ domain     │    │  migrations,     │  interfaces below
              │ sync       │    │  sqlcgen, weave, │
              └────────────┘    │  ontology        │
                          \     └──────────────────┘
                           \             │
                            \            ▼
                          ┌──────────────────────────┐
                          │         domain/          │  pure types
                          │   Translations, IDs,     │  + Store
                          │   BaseModel, errors      │  interfaces
                          │  ┌────────┐ ┌─────────┐  │
                          │  │ weave/ │ │ontology/│  │
                          │  └────────┘ └─────────┘  │
                          └──────────────────────────┘

                    cmd/pletka, cmd/<other-tool> — binaries
                         (pick whichever subset they need)
```

## Boundary contract

The boundary is mechanical, not aspirational. The DAG above has no
cycles; preserve that property.

- **`domain/`** (root) imports nothing else inside Pletka. It owns
  the shared primitives — `Translations`, ULID generation, common
  errors, identity types, `BaseModel` / `VersionedEntity`. No HTTP,
  no DB driver glue, no JSON shape.
- **`domain/weave/`** imports `domain/` only. It owns the weave
  semantic patterns — Fields, Models, Collections, categories,
  projects, the override chain — and declares the
  `WeaveStore` interface in `domain/weave/store.go`.
- **`domain/ontology/`** imports `domain/` only. It owns CRM classes,
  properties, paths, namespace bindings, version handling, and
  declares the `OntologyStore` interface.
- **`domain/weave/` and `domain/ontology/` do not import each
  other.** Anything they share lives in `domain/` (root).
- **`store/`** imports `domain/` and the relevant subsystem package
  it implements. It is the persistence layer: pgx pools, goose
  migrations, sqlc-generated queries, and concrete Store
  implementations. `store/weave/` implements `domain/weave.WeaveStore`,
  `store/ontology/` implements `domain/ontology.OntologyStore`.
- **`gitsync/`** imports `store/` and `domain/`. It serialises
  domain state into git via the store and rehydrates state from git
  back into the store. Per-subsystem plugins under
  `gitsync/<subsystem>/`.
- **`server/`** imports `store/`, `domain/`, and `gitsync/`. It is a
  library — handlers, middleware, schema builders, link emitters,
  embedded renderer bundle. It does not contain the main binary.
- **`cmd/<tool>/`** are the binaries. `cmd/pletka/` is the HTTP
  server and imports `server/`. Other CLIs (importers, exporters,
  one-shot maintenance tools) import only what they need —
  typically `store/` + `domain/` + `gitsync/`, without `server/`.
- **`renderer/`** consumes only the JSON schema. It never touches
  Go types; it never decides authorization; it never constructs API
  URLs.

If a PR violates any of these arrows, fix the architecture, not the
lint rule.

### Why this layout

The old shape put the binary under `server/cmd/pletka/`, which
forced any CLI that wanted reuse to drag the entire HTTP layer with
it. Pulling `cmd/` to the top level lets a CSV importer or a git
materialiser depend only on `store/` and `domain/`.

The same logic separates `domain/` (interfaces + types) from
`store/` (persistence). Tests, CLIs, and any consumer that wants
fakes can depend solely on the interfaces in `domain/`.

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
