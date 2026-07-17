# Architecture Decision Records

An ADR records a decision that cost something to make: it crossed architectural
boundaries, rejected a reasonable alternative, or would be re-debated without a
written record. ADRs are **never edited or deleted** — only superseded or amended
by a new record. They are the layer that keeps the codebase (and AI-assisted work
across sessions) coherent.

Write one when a decision affects multiple packages, rejects a real alternative,
will surprise a future reader, or is the kind of thing that gets reopened. Don't
write one for everyday implementation choices.

The most valuable section is **What we considered and rejected** — it stops a
future session from re-proposing what was already tried.

Format: [`TEMPLATE.md`](TEMPLATE.md). Numbering is zero-padded and sequential.

## Index

| ADR | Decision |
|---|---|
| [0001](0001-weave-module-shapes.md) | Two module shapes inside `pkg/weave/` — full slice vs handler-only. |
| [0002](0002-canonical-node-identity.md) | Canonical node identity for generator output — derived structural ids, not stored UUIDs. |
| [0003](0003-explicit-errors-over-heuristics.md) | Explicit context over runtime heuristics — fail loud, don't guess. |
| [0004](0004-namespace-resolution-boundary.md) | Namespace resolution boundary — `pkg/namespace` is canonical; readers resolve explicitly, never guess. |
| [0005](0005-pletka-auth-boundary.md) | Pletka auth boundary — `pkg/auth` owns identity, capabilities, and role mappings; slices consume, never fork. |
| [0006](0006-override-shared-infrastructure-slice.md) | Override is a shared infrastructure slice — peer entity slices may import it directly; the peer-isolation rule otherwise stays hard. |
| [0007](0007-host-injection-over-global-registration.md) | Host injection over global registration — extensions are explicit constructor arguments through app/CLI options; no package-global registries, no `init()`. |
