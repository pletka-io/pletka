# CLAUDE.md

This file provides guidance for agents working in the Pletka core repository
(`github.com/pletka-io/pletka`).

Read `docs-oss/VISION.md` for the product vision, then prefer the current
source tree over older documents.

## Documentation Structure

This repository's documentation is the canonical, publishable set:

**Reference layer (canonical):**
- `docs-oss/architecture/` — Architecture pages (slices, domain model, schema-driven API/UI, data layer, ontology autocomplete, etc.)
- `docs-oss/reference/` — Reference documents (Go style, naming conventions, CIDOC-CRM)
- `docs-oss/decisions/` — ADRs (Architecture Decision Records) with numbered scheme
- `docs-oss/VISION.md`, `docs-oss/ZEN.md` — product vision and principles

**In-repo working docs (still tracked here):**
- `docs/specs/` — BDD contracts and behavioural specifications
- `docs/schema-ui-core/` — schema-driven UI contracts and verification notes

**Relocated to the `pletka-platform` host repo (not in this repository):**
Private implementation plans, design/planning history, archive, reports, and
agent-workflow skill docs were moved out of core to keep it publishable. If you
need that history, it lives under `pletka-platform`'s
`docs/core-src-archive/`. Do not reintroduce `docs/plans/`, `docs/archive/`, or
`docs/superpowers/` here.

All `.claude/rules/` files link to their corresponding `docs-oss/` pages.

## Current Boundary

This repository is the reusable core application:

- `cmd/` contains the core CLI surfaces such as `serve`, `db`, `migrate`,
  `weave`, and development utilities.
- `pkg/app` owns application assembly and server runtime wiring.
- `pkg/weave` contains the vertical slices.
- `frontend/` contains the core Svelte islands and schema-driven renderers.

Customer operational tooling now belongs in `pletka-platform`, not this repo:

- Airtable export
- Airtable import waves
- ontology manifest loading and project ontology config
- full local reload/reset pipelines
- deployment scripts and instance config
- Arches fleet management, the Arches integration, and the Arches
  RDM vocabulary loader (`arches` CLI, `weave arches-push`)
- the arches, cytoscape, and shacl generator renderers (core keeps the
  turtle/jsonld/mermaid/exportgraph/sparql/x3ml/researchspace/csv set;
  platform host wiring appends the rest)

Do not reintroduce those platform-only commands into core unless a concrete
public core use case exists and the reusable API is designed deliberately.

Import provenance columns (`airtable_ref`, `airtable_raw`, `airtable_context`)
and the staging resolve helpers are intentionally retained in core schema — they
record where migrated data came from and are consumed by the platform-owned
importer. Keeping them in core is deliberate, not boundary leakage.

## Common Commands

```bash
make dev              # Start the core app with hot reload
make build            # Build bin/pletka
make test             # Run Go tests
make frontend-build   # Build Svelte islands
make typegen          # Generate TypeScript types
```

The app reads runtime config through the normal server config bootstrap
(`configs/config.yaml`, env vars, and CLI flags). It does not read platform
import/export config.

## Development Notes

- Keep edits scoped to the current task.
- Prefer schema-driven UI and generic frontend registries over domain logic in
  islands/widgets.
- Prefer explicit app wiring and typed slice hosts over global god objects.
- Do not add agent attribution to commit messages.
