# ADR-0016: Legacy ontology snapshots preserve evidence without certifying meaning

Date: 2026-10-29  
Status: Accepted

## Why this decision was needed

Imported projects can select multiple versions of an ontology and contain
references with no recorded version provenance. Requiring a unique semantic
interpretation before retaining those projects would discard evidence or invent
intent. A sealed local ontology body already preserves bytes independently of
the stronger publication contract.

## What we decided

Use a separate `ontologysnapshot` capability for explicit legacy preservation.
Capture concrete body IDs, digests, original selections and semantic inputs from
one archived project context, with resolved, ambiguous, missing or invalid
assessments. Source projects retain their own captured contexts. Storage derives
the evidence; callers cannot supply assessments. A bound reader has access only
to that immutable snapshot. Semantic lookup succeeds only for a unique retained
candidate. Existing ontology publications and release receipts keep their
stronger guarantees.

## What we considered and rejected

- **Choose the primary or newest version:** convenient for browsing, but invents
  provenance and changes results when policy changes.
- **Treat a body seal as a publication:** preserves local content but does not
  establish dependency closure or validate references within the ontology.
- **Add an allow-legacy flag to individual readers:** distributes policy and
  permits accidental hot-table fallback.
- **Flatten inherited choices by ontology owner:** loses source context and
  cannot represent distinct revisions, including revisions sharing a label.
- **Accept caller-supplied successful resolutions:** allows incomplete or
  fabricated evidence to acquire the appearance of a storage receipt.

## Consequences

- Inventories identify bodies by owner, concrete version ID and digest, never
  the human version label. Original primary flags remain evidence, not tie breakers.
- Capture uses repeatable read and the existing body seals. The snapshot retains
  exact canonical body bytes, raw semantic elements, full captured bindings and
  source context digests. Transitive contexts are retained once in a flat inventory;
  entity lookup follows the recorded source automatically. No inferred dependency is added from the live catalog.
- Assessments concern project references within their own selected bodies.
  Ontology dependency declarations and relations remain in the sealed bodies;
  their presence does not certify a consistent ontology dependency closure.
- Snapshot storage is immutable and retained, including against raw DML,
  truncation, parent deletion and downgrade. No historical rows are backfilled.
- This first boundary captures ontology evidence from existing archives. It
  does **not** certify completeness of those archives, finish a project release,
  change ordinary creation, or provide a portable restore format. The explicit
  rebuild/finalization and export/restore integrations must preserve this distinction.
- New or changed editor references still require ordinary validation. Carrying
  unchanged legacy inputs forward will require exact source/value evidence.

## References

- [ADR-0004](0004-namespace-resolution-boundary.md)
- [ADR-0011](0011-cross-project-reads-resolve-at-the-source-release.md)
- [ADR-0013](0013-ontology-drafts-and-immutable-releases.md)
- [ADR-0015](0015-project-semantic-references-at-release-cut.md)
