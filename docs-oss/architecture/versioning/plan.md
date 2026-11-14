# Plan

Order of work from today to git as primary storage. Each phase ends in a
state that is correct on its own and that the next phase needs. Rule ids are
from [the rules page](../versioning-and-inheritance.md), decision points from [`design.md`](design.md).

Not a task breakdown: each phase gets its own implementation plan when it
starts.

## Why this order

- The closure resolver comes first because every other rule is stated in
  terms of the closure. Fixing reads or receipts before it exists means
  fixing them twice.
- The link guard comes before read conversion because reads through the
  closure are only safe once no write path can create a closure the resolver
  rejects.
- Receipts and the cut come after reads because a derived receipt version
  needs every reader to ask the closure.
- Git work comes last in code but **first in the spec** (phase 0): the format
  is the target, so its text is settled before code is built against it.

## Phases

### 0. Settle the text and measure

- Decisions in [`decisions.md`](decisions.md).
- Format spec changes (K0), spec only.
- Audit command, first version: walks links and receipts with today's
  queries and lists cycles, diamonds, chains over the depth limit, legacy
  single-parent projects, receipts off the selector, draft links inside
  releases, closure rows shared between projects, ontology version
  conflicts, public projects without a release, copied categories split
  into unchanged and changed (M1). Run on every instance; the counts size the later phases.

Done when: decisions recorded, spec merged, audit output per instance stored.

### 1. One closure resolver

- `closure.Resolve` (design point 2) with problems instead of truncation
  (L4, L5 detection, O1, O2 detection, O5, L7 order).
- Migrate legacy single-parent projects to link rows and drop the column
  (L8). This removes a branch from every walker before they are replaced.
- Replace the existing walkers one by one; each replacement is a small PR
  with the old and new result compared in a test.
- Draft views show closure problems (L10). Nothing is refused yet.

Done when: no chain walk exists outside the resolver; audit uses it.

### 2. One link guard

- `links.Change` (design point 3): L1, L2, L3, L4, L5, L9, O2 on link
  changes.
- Reference set, first version: receipts, overrides, placements, value
  targets (design point 4); L6 for re-pin and removal, re-stamp replaced by
  D1 if accepted.
- Importers, CLI re-pin and restore call the guard.
- Version grammar in one place (R1).

Done when: conformance forbids a link write outside the guard.

### 3. Reads through the closure

- Convert slices to selector-taking readers (I1), in the order of the
  existing ledger; collection first.
- Resolver, pickers, detail view, adoptions tab, generators, exports.
- Scope as an argument; retire the ambient accessor (S2, S4, S5).
- Deprecation as of the selector (I5, I6); visibility through links (I7).

Done when: the read allowlist and the ambient-accessor baseline are zero.

### 4. Receipts, forks, categories

- Receipt version derived (A2), adopt only from the closure at the selector
  (A4, A5, A6), one receipt per entity.
- Concept-list receipts (A7); category copy as fork (A8); forks record their
  version (F1).

Done when: no writer passes a receipt version; audit shows no off-selector
receipts in drafts.

### 5. The cut and release state

- Lock and consistent snapshot (R6); refusal over the whole closure (R5).
- Closure lock keyed by releasing project, including bindings (R3, O5).
- Current-release pointer (R9), withdrawal semantics (R8), delete guard
  (R10).
- Conformance: only the cut writes release rows (R7).
- Legacy reader and release flags (M2, I4).

Done when: a release cut after this phase resolves with zero problems and
zero legacy flags, by construction.

### 6. Ontologies, vocabularies, examples

- Ontology revisions with hash, immutable once linked (O3); defaults as an
  explicit link (O4); one version per closure enforced on writes (O2).
- Released lists and vocabularies read their archives (V2, V4); list
  membership through receipts (V3, V1, V5).
- Examples validated at the selector, status as cache, model in closure
  (X2, X3, X5).

### 7. Git trees carry the closure

- Release trees with `pletka.sum`, `vendor/`, placements, list members,
  examples (K3, K7); receipts without version, forks with (K5); semantic ids
  only (G2); content-only hashes (K9); current and withdrawn in the index
  (K2).
- verify-git uses the closure resolver (K6).
- Restore rebuilds pinned releases from vendored trees and reports (K8).
- Byte-equal exports (I8).

Done when: for every project on every instance, export → verify-git →
restore into an empty database → export gives the same tree, and the
database resolver and the git resolver return the same closure.

### 8. Git as primary storage

- Releases are cut by tagging; the database is hydrated from the tree.
- Commit selectors allowed under the new format version (K4).
- Out of scope for this document beyond the gate above.

## What can run in parallel

- Phase 0 audit and the spec text.
- Phase 6 vocabulary items (V2, V4, V5) do not depend on phases 2–5.
- Small fixes that remove a known wrong read without waiting for its phase
  are fine when they move toward the target shape (take a selector, ask the
  owning slice). Fixes that add another walker or another version fallback
  are not.

## Risks to the plan

| Risk | Handling |
|---|---|
| Existing data violates a rule in volume | audit in phase 0 sizes it before enforcement; M3 |
| Releases already merged closure rows of two projects | flagged `closure-ambiguous`, legacy reader (D5) |
| Read conversion is large (60 ambient sites, allowlist) | slice by slice, conformance baseline only goes down |
| Format spec and rules drift again | K0: spec first; verify-git and server share the resolver |
| A cut during the transition mixes old and new behaviour | phase 5 lands the flags; a release is either fully new or flagged |
