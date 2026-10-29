# ADR-0017: Legacy release completeness is a separate contract

Status: Accepted (2026-10-29)

## Context

A retained legacy ontology snapshot preserves evidence but does not prove that
the project's structural archives are complete, or that the snapshot describes
the same cut. The existing content receipt also promises verified ontology
publication and uniquely resolved semantic references. Issuing that receipt for
preserved ambiguous references would change its meaning.

## Decision

Keep verified content capture as contract 1. Introduce contract 2 for complete
structural capture with preserved legacy ontology evidence. Both validate the
same 22 owned archive sections, project dependencies and output descriptors.
Contract 2 additionally binds the exact legacy snapshot digest. Storage derives
the snapshot's local context again at finalization and requires it to match the
archived cut. Snapshot presence alone is insufficient.

Use one structural capture pipeline with private evidence implementations chosen
by distinct lifecycle entry points. Ordinary release creation and rebuild retain
contract 1. The explicit legacy rebuild API selects contract 2; request handlers
cannot select it through a permissive boolean or a failed-publication fallback.

## Consequences

- Contract 2 retains `ontology_dependencies_version = 0`, with no verified
  ontology pins or semantic resolution receipt. Its content receipt records
  `format_version = 2`, `evidence_kind = legacy_ontology_snapshot`, the exact
  snapshot digest and all structural section counts/hashes.
- `Session.Content` remains contract-1-only. `Session.LegacyContent` returns a
  distinct receipt with an exact project/release/digest snapshot reference.
  Neither method falls back to live data or treats preservation as resolution.
- Both contracts require a new repeatable-read cut and storage-owned receipts.
  Completed archives, dependency evidence and capture identity are immutable;
  callers cannot change a completed release's contract. Source releases must
  have the same completion contract as their consumer. Mixed-contract source
  adaptation requires a separately specified boundary.
- `PlanLegacyRebuild` and `RunLegacyRebuild` use a distinct request type and plan
  format 2. They share source ordering, the maintenance lease, input checks,
  atomic capture and durable publication recovery with ordinary rebuilds.
  Plan fingerprints include unpublished ontology bodies as well as publications.
- The existing Git publisher does not implement the legacy evidence contract.
  Operator wiring must wait for portable serialization and exact restore; the
  core lifecycle alone does not make a legacy rebuild operationally complete.
- The assembler must carry this distinction into detached values. Semantic
  generators require checked definitions and report unresolved inputs;
  preservation exports can retain those inputs and their assessments. This
  decision does not claim the generator migration is complete.

## Rejected alternatives

Setting verified markers on legacy evidence would misrepresent its meaning.
Checking only for an ontology snapshot could certify stale evidence. Duplicating
the entire release pipeline would let structural guarantees diverge. Per-reader
compatibility flags or live fallbacks would make the caller responsible for
preserving the boundary.

## Related decisions

- [ADR-0012: Whole-project assembler](0012-whole-project-reads-belong-to-an-assembler.md)
- [ADR-0015: Project semantic references](0015-project-semantic-references-at-release-cut.md)
- [ADR-0016: Legacy ontology snapshots](0016-legacy-ontology-snapshots-preserve-evidence.md)
