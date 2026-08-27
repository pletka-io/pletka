# Schema UI Core

This directory captures the reusable core of the schema-driven UI approach used
in Pletka. It is intentionally separate from Pletka-specific implementation
details so it can be used to:

- design new features consistently
- audit existing implementations
- identify design weaknesses that limit reuse
- guide extraction into a reusable library
- help agents implement against the same conventions

## What Schema UI Is

Schema UI is a Go + Svelte composition pattern:

1. A backend slice owns domain logic, storage, permissions, and schema builders.
2. The backend emits JSON contracts that describe UI structure, data,
   capabilities, and mutation URLs.
3. Generic Svelte renderers consume those contracts without domain knowledge.
4. Domain-specific behavior stays in slices, read models, DTOs, or custom
   widgets.

The goal is not to hide application logic behind a framework. The goal is to
make the reusable machinery stable while keeping domain code explicit.

## Documents

- [principles.md](principles.md) — the non-negotiable design rules.
- [contracts.md](contracts.md) — backend/frontend contract types and boundaries.
- [slice-architecture.md](slice-architecture.md) — the reusable vertical-slice
  shape.
- [portability.md](portability.md) — the three tiers (contract / renderers /
  slice-store) and which ports to which target app.
- [verification.md](verification.md) — audit and implementation checklists.
- [adoption-roadmap.md](adoption-roadmap.md) — how to integrate the pattern into
  existing or new projects.

## Relationship To Pletka Docs

This directory describes the reusable core — the design as it ports to **other**
applications. The current, code-grounded description of how the pattern is built
**in Pletka itself** is the curated core set under [`../../docs-oss/`](../../docs-oss/):

- [`../../docs-oss/architecture/schema-driven-ui.md`](../../docs-oss/architecture/schema-driven-ui.md) — how the backend drives the frontend.
- [`../../docs-oss/architecture/schema-driven-api.md`](../../docs-oss/architecture/schema-driven-api.md) — the JSON contract and error envelope.
- [`../../docs-oss/architecture/slices.md`](../../docs-oss/architecture/slices.md) — the vertical-slice / service-layer shape in the codebase.
- [`../../docs-oss/architecture/svelte-islands.md`](../../docs-oss/architecture/svelte-islands.md) — how interactive UI mounts into server-rendered pages.
- [`../../docs-oss/architecture/ui-platform-boundaries.md`](../../docs-oss/architecture/ui-platform-boundaries.md) — the `formschema` vs `detailview` boundary.

The 2026-05-16 SchemaUI library-extraction spec has been **superseded** and moved
to the host archive (`pletka-platform/docs/core-src-archive/superseded/`); it is
no longer part of this repo.

When these docs disagree, prefer this directory for reusable design principles
that must hold in any host, and prefer `docs-oss/` for current Pletka codebase
mechanics.
