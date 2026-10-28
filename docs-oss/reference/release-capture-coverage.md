# Release capture coverage

The complete-project assembler is still planned. Capture evidence has separate
contracts; archive ownership, dependency pins, ontology references and descriptors
do not certify that every project field was copied or can be restored from Git.
See [ADR-0012](../decisions/0012-whole-project-reads-belong-to-an-assembler.md).

## Project/model metadata contract 1

Migration 031 adds `weave_releases.metadata_capture_version`. Zero means the
release lacks this evidence; migration does not backfill it from current data.
Version 1 certifies:

- The project's raw `concept_namespace`, preserving null, empty and whitespace.
- Its effective concept namespace, including the default chosen at cut time.
- Complete owned model membership and each model's `model_type`.

The named snapshot queries run in the existing repeatable-read release transaction.
Finalization checks raw/effective namespace against that snapshot and compares the
model IDs/types in both directions. Missing, extra or changed models reject the
cut and roll back its archives and outbox. Model snapshotting now uses sqlc rather
than another inline SQL statement.

`weave_effective_concept_namespace_v1` centralizes normalization and the default
`https://vocab.pletka.io/`. It preserves the prior Unicode whitespace behavior.
Live vocabulary reads use the same policy and propagate lookup failures. A
missing project or failed query is not an instruction to select the default.
Future policy changes need a new version; released reads use the stored value.

Certified project/model archive rows reject inserts, updates, key moves, deletes
while their owning release is retained. Project/model archive truncation is always
rejected because an old database snapshot can miss newly finalized releases.
Both archive writes and release creation touch a persistent project/version
coordination key before checking the release record. A stale repeatable-read
snapshot then aborts, including when the release did not exist at snapshot start.
The certificate cannot be reset or
retargeted, and downgrade cannot discard it. Normal release deletion permits
subsequent archive cleanup. Coordination keys remain after cleanup to protect
reuse of the same project/version; they are operational state, not content.
Other archive tables are not protected by these guards.

`readsession.Session.Metadata` returns detached values through archive SELECTs on
the existing session. Its model lookup is restricted to captured own membership;
use a source session for foreign content. Draft, zero and closed sessions fail.
Missing historical evidence returns `not_in_release`, even if an older archive
contains a defaulted model type or someone fills a namespace column afterward.

## Initial column inventory

[`capture_inventory_test.go`](../../pkg/weave/release/capture_inventory_test.go)
classifies every persisted project/model column. Unit checks compare generated
schema shapes and each INSERT column's SELECT expression. Integration checks
compare the inventory against the migrated PostgreSQL catalog, so forgetting sqlc
regeneration cannot hide a schema addition.

| Fields | Capture disposition |
| --- | --- |
| Project/model fields listed as copied in the inventory | Direct copies, including model type and raw concept namespace |
| `status` | Existing release-cut publication transformation |
| `version_number` | Concrete owning release version |
| `effective_concept_namespace` | Derived once by the versioned namespace policy |
| Project `is_core_weave` | Excluded: live administrative parent-picker/discovery policy |
| Project `enforce_concept_lists` | Deferred gap: authored vocabulary enforcement setting; classify/capture during the remaining coverage audit |

This is the initial two-table inventory, not the completed domain/restore matrix.
The remaining audit must cover every required table and field, archive mutation
paths and portable serialization/restore dispositions. The existing Git format
still omits model type, concept namespace and other domain content. Metadata
capture does not change existing tag bytes or migrate export/browser consumers.

Validation includes real cuts of auxiliary/example/core models, null/blank/custom
namespaces, subsequent live edits/deletion and a second cut, concurrent edits,
capture failures, empty model membership, raw mutation rejection, old database
snapshots on either side of release creation, and migration
round trips that preserve unknown historical values.
