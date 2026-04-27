# OpenWolf

@.wolf/OPENWOLF.md

This project uses OpenWolf for context management. Read and follow .wolf/OPENWOLF.md every session. Check .wolf/cerebrum.md before generating code. Check .wolf/anatomy.md before reading files.


# CLAUDE.md

Guidance for AI assistants (and human contributors) working in this repository. Written for humans first; the AI inherits the conventions by reading the same file.

## What Pletka is

A Go server that exposes a hypermedia JSON contract, plus a Svelte renderer that is a pure function of that contract. Three things matter more than anything else in this repo:

1. **The schema is the contract.** Domain knowledge lives in Go schema builders (`server/widgets/`). The renderer is a generic dispatcher.
2. **Authorization is hypermedia-driven.** The presence of an action in the response — either as a typed capability (default for mutations) or as a generic link relation (for open-ended navigation) — is the user's permission to take it. The renderer never decides. See `docs/authorization-model.md`.
3. **The boundary is mechanical.** `domain/` imports nothing else. `domain/weave/` and `domain/ontology/` never import each other — shared primitives live at `domain/` root. `store/` implements the Store interfaces declared in `domain/<subsystem>/`. `gitsync/` and `server/` consume `store/` + `domain/`. `cmd/<tool>/` picks whichever subset it needs. `renderer/` consumes only the JSON. Violations are caught by `go vet`, lint, or import-graph checks — not by discipline.

## Layout

```
cmd/         binaries — cmd/pletka is the HTTP server; siblings
             (importers, exporters, one-shot tools) added as needed
domain/      pure types + Store interfaces. No HTTP, no DB drivers.
             domain/                 shared primitives (Translations,
                                     ULID, BaseModel, errors)
             domain/weave/           Fields, Models, Collections,
                                     categories, projects, override
                                     chain + WeaveStore interface
             domain/ontology/        CRM classes, properties, paths,
                                     namespace bindings + OntologyStore
                                     interface
store/       PostgreSQL persistence — pgx + sqlc
             store/postgres/         pool, conn, migrations runner
             store/migrations/       goose SQL files (embedded)
             store/sqlcgen/          generated query code
             store/weave/            implements domain/weave.WeaveStore
             store/ontology/         implements domain/ontology.OntologyStore
gitsync/     bidirectional git ↔ domain sync, per-subsystem plugins
server/      HTTP layer LIBRARY (no binary) — handlers, schema
             builders, link emitters, middleware, auth, embedded
             frontend bundle
renderer/    Svelte 5 source — built once, embedded into the binary
schema/      docs + JSON example fixtures, no Go code
docs/        architecture, protocol, decisions, plans, specs
```

Module path: `github.com/pletka-io/pletka` (single root module, no subpath).

## How to add a widget

1. Document it: `schema/widgets/<name>.md` (fields, semantics, example).
2. Provide fixtures: `schema/examples/<name>-*.json`. At least one. Include auth variants (`<name>-anonymous.json`, `<name>-editor.json`, etc.) so the authorization-via-hypermedia behaviour is explicit.
3. Implement the builder: `server/widgets/<name>.go` exporting `func <Name>(...) Widget`.
4. Implement the renderer: `renderer/src/widgets/<Name>.svelte` consuming the schema shape.
5. Both server tests (round-trip the fixture) and renderer tests (snapshot the fixture) run automatically.

Same pattern for link relations under `server/relations/` + `renderer/src/relations/` + `schema/relations/`. See `docs/adding-a-widget.md` and `docs/adding-a-relation.md`.

## Coding standards

### Go

- **Style.** `gofmt`, `goimports`. `golangci-lint` config is checked in (`.golangci.yml`); the same invocation runs locally and in CI.
- **Errors.** Lowercase messages. Wrap with `fmt.Errorf("context: %w", err)`. Never ignore errors silently. No `panic` in library code.
- **Logging.** `log/slog` with structured fields. No `log.Printf`, no `fmt.Println` for logging.
- **Guard clauses.** Return early. No deep nesting on the happy path.
- **Tests.** `go test`. Use `github.com/google/go-cmp/cmp` for structural comparisons. Table-driven tests where appropriate.
- **Context.** Methods that do I/O take `context.Context` as the first parameter.
- **No globals.** Pass dependencies via constructors. No `init()` for behaviour.
- **Doc comments.** All exported types, funcs, and constants have a doc comment beginning with the symbol name.

### Svelte / TypeScript

- **Svelte 5 runes.** `$state`, `$derived`, `$effect`, `$props`, `$bindable`. No legacy `$:` reactivity, no stores for component state.
- **Renderer is pure.** No business logic, no permission checks, no entity-name conditionals, no constructed API URLs. If a component needs domain knowledge, the schema is missing a field.
- **Type checking.** `npm run check` (`svelte-check`) must pass. CI enforces.
- **Tailwind 4.** Configure in CSS, not `tailwind.config.js`. Use the project's design tokens.

## Hypermedia authorization invariant

The server emits an action only when the current user is permitted to act on it; the renderer renders what it receives. Two shapes encode the grant:

- **Typed capabilities** — the default. Forms, lists, page envelopes embed a `Capabilities` struct of nullable pointers (`Edit`, `Delete`, `Reorder`, etc.). Pointer non-nil = permitted. Each cap carries the URL / payload schema the renderer needs.
- **Generic link relations** — for open-ended hypermedia (navigation, breadcrumbs, related entities, pagination, alternative views, share, etc.). Page / widget / entity envelopes carry `Links []Link`. Presence of a link with `rel == "X"` = permitted to follow X.

Both honour the same invariant: presence is the grant, absence is the denial. See `docs/authorization-model.md` for which shape to pick.

Lint rules in `renderer/` forbid:

- `if (user.role === ...)` permission checks
- `fetch(` outside the link-handler / capability-handler dispatcher
- State that isn't derived from props or schema

Server-side, leave a typed capability nil unless permitted:

```go
caps := Capabilities{}
if user.Can("edit", target) {
    caps.Edit = &EditCap{URL: editURL(target)}
}
```

For generic links, prefer the fluent helper (added with the first such link):

```go
links.IfCan(user, "share", article).Add(dst, "share", shareURL(article))
```

If the helper feels tedious, fix the helper — do not add client-side fallbacks.

## Build commands

```sh
make build       # renderer build → embed → go build → bin/pletka
make test        # go test ./... + renderer tests
make lint        # golangci-lint + svelte-check
make dev         # local dev with hot reload
make gitleaks    # secret scan working tree
make release     # goreleaser
```

Single-binary deploy: the renderer bundle is embedded via `//go:embed` in `server/assets/`.

## Documentation structure

- **`docs/decisions/`** — ADRs. Why is the system shaped this way? Read before proposing architectural change.
- **`docs/plans/`** — Implementation plans. Date-prefixed (`YYYY-MM-DD-topic.md`). Read the most recent when re-entering a session. Move superseded plans to `docs/plans/archive/`.
- **`docs/specs/`** — Behavioural contracts. Check for spec coverage when changing behaviour.

Reference docs (`docs/architecture.md`, `docs/protocol.md`, `docs/authorization-model.md`, etc.) are loaded on demand.

## What does NOT belong in this repo

- Deployment scripts, container manifests, Terraform, helm charts, customer or environment configs. Those live in private deployer repositories.
- TODO comments referencing private context (customer names, internal hostnames, ticket IDs).
- Vendored binaries, generated artifacts, local credentials. Regenerate or fetch fresh.

If you find any of the above in a PR, reject it.

## When in doubt

- Read the most recent plan in `docs/plans/`.
- Check `docs/decisions/` for the architectural rationale.
- Open an issue describing what you want to change before writing the code.
