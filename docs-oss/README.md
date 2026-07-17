# Pletka documentation

Pletka is a collaborative platform for building semantic data models — "GitHub
for semantic models". Cultural-heritage institutions use it to create, adopt, and
customise the semantic patterns — **fields**, **models**, and **collections** —
that describe their collections, grounded in shared ontologies such as CIDOC-CRM.

This is the curated core documentation set. It is deliberately small: one concept
per file, kept honest against the code. Working plans, specs, and historical
material live elsewhere and are not part of this set.

## Reading order

New to the codebase? Read in this order:

1. [`VISION.md`](VISION.md) — what Pletka is and why it exists.
2. [`ZEN.md`](ZEN.md) — the design philosophy in one page.
3. [`architecture/README.md`](architecture/README.md) — the four ideas that carry
   the codebase.
4. [`architecture/slices.md`](architecture/slices.md) — how a feature is structured.
5. [`architecture/schema-driven-ui.md`](architecture/schema-driven-ui.md) — how the
   backend drives the frontend.
6. [`architecture/svelte-islands.md`](architecture/svelte-islands.md) — how
   interactive UI mounts into server-rendered pages.
7. [`architecture/data-layer.md`](architecture/data-layer.md) — pgx + sqlc + goose.
8. [`architecture/errors.md`](architecture/errors.md) — the error envelope and the
   explicit-over-heuristic rule.
9. [`architecture/domain-model.md`](architecture/domain-model.md) — entities,
   identity, and overrides.

## Map

| Path | What it holds |
|---|---|
| [`VISION.md`](VISION.md) | Product vision and a worked example (Appendix A). |
| [`ZEN.md`](ZEN.md) | Design philosophy. |
| [`principles.md`](principles.md) | The normative rules — the constitution. |
| [`architecture/`](architecture/) | How the system is built. |
| [`decisions/`](decisions/) | Architecture Decision Records (ADRs). |
| [`reference/`](reference/) | Naming conventions, Go style, CIDOC-CRM. |
| [`contributing/`](contributing/) | Build, test, and the AI-assisted workflow. |

## Architecture pages

| Page | Topic |
|---|---|
| [`architecture/slices.md`](architecture/slices.md) | Vertical slices and their shapes. |
| [`architecture/domain-model.md`](architecture/domain-model.md) | Entities, identity, and overrides. |
| [`architecture/schema-driven-api.md`](architecture/schema-driven-api.md) | Schema-driven API contracts. |
| [`architecture/schema-driven-ui.md`](architecture/schema-driven-ui.md) | How the backend drives the frontend. |
| [`architecture/svelte-islands.md`](architecture/svelte-islands.md) | Svelte islands and the mount system. |
| [`architecture/data-layer.md`](architecture/data-layer.md) | pgx + sqlc + goose. |
| [`architecture/ontology-autocomplete.md`](architecture/ontology-autocomplete.md) | CRM path suggestions. |
| [`architecture/ui-platform-boundaries.md`](architecture/ui-platform-boundaries.md) | formschema vs. detailview. |
| [`architecture/override-editor.md`](architecture/override-editor.md) | Override editor design and contracts. |
| [`architecture/composition-and-extension.md`](architecture/composition-and-extension.md) | How a host extends core — the extension-point catalog. |
| [`architecture/errors.md`](architecture/errors.md) | The error envelope. |

## Reference pages

| Page | Topic |
|---|---|
| [`reference/naming-conventions.md`](reference/naming-conventions.md) | Go, JSON, URL, and schema naming conventions. |
| [`reference/go-style.md`](reference/go-style.md) | Go style rules and practices. |
| [`reference/cidoc-crm-reference.md`](reference/cidoc-crm-reference.md) | CIDOC-CRM 7.1.3, Linked Art, and ontology tools. |

## Status

Active development. The Go module is `github.com/pletka-io/pletka` and the dev
database is `pletka_weave`.

> This `docs-oss/` tree is the clean-room basis for the open-source repository.
> At the core/platform split it becomes the canonical `docs/`.
