# Draft project patches

`projectsnapshot.PatchDraft` applies explicit before/after row images to an
existing `CompleteProject` draft. It is a pure value boundary. It performs no
database reads and accepts no `EntityFragment`: a fragment's referenced bodies
are validation context, not permission to replace those entities.

## Row ownership and failure

`DraftPatch.Target` must equal the baseline's explicit draft identity. Released
baselines and zero values fail. `Before` and `After` use the same lossless typed
records as complete values, across all 22 captured sections:

- A row in both is an update, matched by the section's full composite key.
- A row only in `Before` is a deletion; only in `After` is an insertion.
- Empty or nil sections mean no changes. They never imply a bulk deletion.
- Every prior row must equal the baseline exactly. An insertion cannot overwrite
  an existing row. Duplicate identities fail. SQL NULL, JSON null, authored JSON
  bytes, exact decimals and date/timestamp strings stay distinct.

The patcher preserves every other baseline row. It does not expand ownership
through references, reread current children or infer cascades. The operation must
supply every deleted or changed child row. The resulting complete graph must
validate before any value is returned; a missing parent or required body fails.
Inputs, sources and the original baseline stay unchanged on success or failure.

Replaying stale before images is an error, not a guessed no-op. A durable writer
must distinguish an already committed change set from a conflicting edit using
its own checkpoint. No helper creates a patch by diffing the baseline against
today's whole project.

## Interpretation and source changes

Without `Context`, the patch retains the baseline's descriptors, publications,
semantic resolutions, dependencies and memberships. A semantic edit whose
existing evidence no longer matches fails validation.

`CompleteProject.DraftContext()` returns detached interpretation inventories.
`DraftContextChange` is an explicit compare-and-swap of that entire context. Its
`Before` must match exactly, including inventory order and unused descriptors;
its `After` must be loaded and bound to the same draft. Actor, namespace policy,
ontology selection and source-pin operations must carry attributable context
changes, not acquire unrelated current metadata while materializing another edit.
Complete validation checks the merged structural rows against the new evidence.

`Sources` accepts only complete immutable releases. An already retained identity
cannot be replaced with different rows or evidence. Repinning requires explicit
dependency/membership changes, affected row pins and the exact new source closure.
Unselected additions and missing selected sources fail. Old source contexts are
removed only when no longer selected; every retained source stays exact.

These checks prove value consistency, not attribution, authorization or the
authenticity of a storage capture receipt. The mutation boundary must produce
the row and context images atomically with the writes they describe.

## Current write-path limitation

The pure patcher is implemented and tested; automatic draft materialization,
initialization and reconciliation still use the legacy writer. Switching them
requires coordinated baseline and delivery checkpoints. Atomic capture is now
available as an explicit opt-in boundary; see [atomic capture](atomic-draft-capture.md).

The reviewed [write inventory](../../pkg/service/gitmaterializer/testdata/draft_write_inventory.json)
records all captured sections, representative write paths, observed audit kinds
and child/delete effects. The existing recorder emits seven kinds: field, model,
collection, category, override, namespace binding and project ontology version.
That vocabulary is not complete storage coverage:

- `weave.NewChangeLogRunner` now joins store writes and log flushing in one
  transaction. Its domain payloads still do not provide complete storage images.
- Example updates delete/reinsert value rows in their own transaction, without
  emitting the row changes. Vocabulary/concept, attribution, project metadata
  and relationship operations also lack complete attributable row images.
- `override.Service.SetRefs` explicitly omits old references from its previous
  payload. Model/collection deletion removes overrides; refs cascade from those
  rows. Category deletion can affect collection defaults and other placements.
- Global namespace/default policy and actor changes can affect draft context
  without any project content audit entry.

Conformance checks ratchet the section review and compare actual literal audit
producers with the legacy writer's accepted vocabulary. Unknown kinds, operations
or ownership fail before a tree rewrite and remain pending with an error. This
detects unsupported entries; it cannot detect a mutation that emits no entry.

`draftcapture.Receipt.Patch` now derives patches from complete snapshots retained
atomically by the writing transaction. Storage effects cover all 22sections and
cascades, while explicit context refresh separates external policy drift. Default
writers still await coordinated baseline initialization/reconciliation and
isolated Git publication. A full current-state upgrade must never be attributed
to an individual user's change-set message.

## Verification

Pure tests exercise edits in every section, whole-graph creation/deletion,
schema3 byte parity, referenced-entity isolation, stale row/context replay,
incomplete cascades, exact null/numeric values, semantic evidence, immutable
source substitution and explicit repinning. The PostgreSQL materializer test
retries an unsupported audit kind and checks that HEAD, staged index bytes and
unstaged content are unchanged while the failure remains pending.
