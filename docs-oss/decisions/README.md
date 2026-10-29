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
| [0008](0008-services-out-seam.md) | Services-out seam — hosts receive narrowed slice interfaces; core never imports the aggregate. |
| [0009](0009-server-errors-through-one-door.md) | Server errors go through one door that takes the cause — 5xx only via functions that require the `error`; lint bans the other paths. |
| [0010](0010-versioned-reads-through-a-scope-bound-reader.md) | Bind project, explicit selection and transaction before exposing narrow readers; invalid selections and missing released content fail closed. |
| [0011](0011-cross-project-reads-resolve-at-the-source-release.md) | Capture source releases and their closure during a cut; released consumers use recorded pins, while draft policy may resolve latest once. |
| [0012](0012-whole-project-reads-belong-to-an-assembler.md) | Whole-project reads use one assembler receiving the bound session; materializer, generators and publication consume complete values. |
| [0013](0013-ontology-drafts-and-immutable-releases.md) | Ontology editing uses drafts; project dependencies, materialization and restore use immutable ontology releases with enforced artifact identity. |
| [0014](0014-release-actor-and-module-descriptors.md) | Capture actor descriptors and module identifiers during release cut; released output uses captured values without live identity or host lookups. |
| [0015](0015-project-semantic-references-at-release-cut.md) | Capture exact project semantic references against retained ontology publications. |
| [0016](0016-legacy-ontology-snapshots-preserve-evidence.md) | Preserve immutable legacy ontology evidence separately from verified semantic meaning. |
| [0017](0017-legacy-release-completeness-is-a-separate-contract.md) | Finalize legacy structural content with an exact snapshot digest, separately from verified semantic publication. |
