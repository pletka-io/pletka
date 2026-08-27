# Portability Boundary

Schema UI is not one reusable thing. It is three layers stacked on each other,
and each ports to a different set of target applications. A target app almost
never adopts all three; it adopts the layers whose stack it shares and
reimplements or drops the rest.

Draw this boundary before porting. Most "can I use Schema UI in app X" questions
are answered by asking which tiers X can consume.

## The Three Tiers

| Tier | What it is | Concrete pieces | Ports to |
|---|---|---|---|
| **A — Contract** | The JSON on the wire | form / list / entity-list / page schemas, error shape, capability entries, mutation + data URLs | **anything that speaks HTTP + JSON** — any language, any frontend framework, mobile, another backend |
| **B — Renderers** | The generic frontend that consumes Tier A | `FormRenderer`, `ListManager`, `FormSection`, `WidgetDispatcher`, base widgets, widget registry, island mount, tygo-generated types | **Svelte + Vite islands only** |
| **C — Slice/store** | The Go backend that produces Tier A | slice contract (routes/handler/service/store), `store_postgres` (pgx + sqlc), `apierror.FromDBError` pg classifier, goose migrations, conformance lint | **Go + chi + pgx/sqlc + Postgres** |

The tiers depend downward only: B consumes A, C produces A. A knows nothing about
B or C. That is the whole reason the contract ports so widely — it is inert JSON
with no runtime.

## Tier A — Contract (ports anywhere)

The schema types and the error shape are plain data. A React, Vue, SwiftUI, or
Go-template frontend can render them; a non-Go backend can emit them. This tier
is the durable, framework-agnostic core of Schema UI.

What you get for free in any target:
- One error contract (`{error, message, errors}`) keyed by field name.
- Server-owned capabilities and URLs — the frontend renders presence, never
  constructs routes.
- The read-model → schema flow (principle 7), which is language-independent.

What you must supply in a non-Tier-B target:
- Your own renderer that walks the schema (a Tier-B equivalent). The contract
  tells you *what* to render; it does not render.
- A widget-name → component mapping in your framework.

**Rule:** if the target app can accept "the backend sends JSON describing the
form," Tier A ports. Everything else is optional.

## Tier B — Renderers (ports to Svelte)

The generic renderers, widget registry, and island mount are Svelte + Vite.
They consume Tier A and nothing else, so they are portable *within the Svelte
ecosystem* but not outside it.

Ports cleanly to: another Svelte + Vite app, regardless of that app's backend
language — because it only needs Tier A JSON, which any backend can emit.

Does **not** port to: React, Vue, Angular, native mobile, server-rendered
templates. Those reimplement the renderer against Tier A. Budget for that: the
renderer layer is the second-largest chunk of code after the slice layer.

The widget contract is the stable seam inside this tier: every widget receives
`{field, value, formValues, lang, languages, errors}`. A reimplemented renderer
in another framework should keep the same widget input shape so widgets stay
conceptually portable even when the component code is rewritten.

## Tier C — Slice/store (ports to Go + Postgres)

The slice shape (routes/handler/service/store split), the schema builders, and
the mount specs are portable Go. The persistence and DB-error pieces are welded
to Postgres.

Splits into two sub-parts with different reach:

**C1 — DB-agnostic Go** (ports to any Go backend):
- slice file layout and boundaries
- `service.go` business rules and permission checks
- `formschema.go` builders (pure contract assembly)
- mount specs, scope helper, schema-provider registry
- the `apierror` domain-error mapping (`FromError` over interfaces)

**C2 — Postgres-coupled Go** (ports only to pgx/sqlc/goose/Postgres):
- `store_postgres.go` (pgx + sqlc row mapping)
- `apierror.FromDBError` — the pg SQLSTATE classifier (23505, 23503, 23502,
  23514)
- goose migrations and the migration-table parametrization
- the conformance lint rule "no pgx/sqlc imports outside `store_postgres.go`"

A target app on a different SQL stack keeps C1, rewrites C2: implement `store.go`
against its driver, and replace `FromDBError` with an equivalent classifier for
its database's constraint-violation errors. The `store.go` interface speaks
domain terms (slice-architecture.md), so C1 does not change when C2 is swapped —
that is the boundary working as designed.

A non-Go backend keeps none of Tier C. It reimplements the whole backend and
emits Tier A.

## What Crosses Each Boundary

```text
Tier C  --emits-->  Tier A (JSON)  --consumed by-->  Tier B
  Go                  wire format                      Svelte
  Postgres            (framework-agnostic)             Vite islands
```

Only Tier A crosses the network. B and C never share types except through the
generated TypeScript (tygo turns Tier-A Go structs into Tier-A TS types) — and
that generation step is itself a Tier-B concern that a non-Svelte target drops.

## Porting Decision Table

| Target app | Reuses | Reimplements / drops |
|---|---|---|
| Go + Postgres + Svelte | A, B, C — all of it | nothing (the home stack) |
| Go + Postgres + React | A, C | B (React renderer against A) |
| Go + MySQL + Svelte | A, B, C1 | C2 (store + DB classifier) |
| Go + non-SQL + Svelte | A, B, C1 (slice shape) | C2 (store), parts of the error contract |
| Non-Go backend + Svelte | A, B | C entirely |
| Non-Go backend + non-Svelte | A only | B and C entirely |

Read the table as: **the contract (A) always ports; the renderer (B) ports with
Svelte; the store (C2) ports with Postgres.** Everything else is a rewrite you
should budget for up front, not discover mid-migration.

## Two Decisions To Make Before Porting

Two properties are baked into the home stack as defaults. Neither is wrong, but a
target app must decide about each *explicitly* — see the adoption roadmap,
Phase 1.

- **Multilingual (i18n).** The contract carries multilingual field values
  (`Translations` maps) as a core default, not an add-on. A single-language
  target either carries that weight (every text field is a language map) or
  strips it and diverges from the contract. Decide which, and record it.
- **Database stack.** Tier C2 assumes pgx + sqlc + goose + Postgres. A target on
  another database keeps C1 and rewrites C2 (store + DB-error classifier). Decide
  the stack before extraction, because it sets how much of Tier C ports.
