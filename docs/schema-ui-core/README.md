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
- [verification.md](verification.md) — audit and implementation checklists.
- [adoption-roadmap.md](adoption-roadmap.md) — how to integrate the pattern into
  existing or new projects.

## Relationship To Pletka Docs

This directory describes the reusable core.

Pletka-specific references remain in:

- `docs/reference/schema-ui-conventions.md`
- `docs/reference/service-layer.md`
- `docs/reference/schema-driven-islands.md`
- `docs/reference/ui-platform-boundaries.md`
- `docs/superpowers/specs/2026-05-16-schemaui-library-extraction-design.md`

When these docs disagree, prefer this directory for reusable design principles
and the Pletka reference docs for current codebase mechanics.
