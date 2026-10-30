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
Migration 033 adds the broader content guards described below.

`readsession.Session.Metadata` returns detached values through archive SELECTs on
the existing session. Its model lookup is restricted to captured own membership;
use a source session for foreign content. Draft, zero and closed sessions fail.
Missing historical evidence returns `not_in_release`, even if an older archive
contains a defaulted model type or someone fills a namespace column afterward.

## Owned-content capture inventory

The reviewed [capture ledger](../../pkg/weave/release/testdata/capture_inventory.json)
covers all 22 owned archive sections and their 272 persisted columns. Each section
records live/archive columns, exact capture expressions, ownership selection and
deduplication rules, full archive identity, exclusions, and current domain/portable
dispositions. Separate capture evidence, ontology artifacts, derived indexes and
operational tables have explicit classifications too.

[`capture_inventory_test.go`](../../pkg/weave/release/capture_inventory_test.go)
checks generated schema shapes, INSERT/SELECT projections, ownership predicates
and named-query wiring against that ledger. The
[catalog test](../../pkg/weave/release/capture_inventory_integration_test.go)
checks actual migrated columns and unique keys, and rejects any unclassified
application table. Adding a table/column or changing its capture expression now
requires an inventory decision; stale sqlc output cannot hide a schema addition.
Do not regenerate this ledger mechanically from the SQL under test.

| Fields | Capture disposition |
| --- | --- |
| Fields listed as copied in the inventory | Direct copies unless an explicit transformation is recorded |
| Entity `status` | Existing release-cut publication transformation where specified; example assessment status is preserved verbatim |
| `version_number` | Concrete owning release version |
| `effective_concept_namespace` | Derived once by the versioned namespace policy |
| Project `is_core_weave` | Excluded: live administrative parent-picker/discovery policy |
| Project `enforce_concept_lists` | Authored setting, copied by migration 032's capture path |
| Override `set_value_entry_id` | Copied reference; foreign entries also require captured source-release membership |
| Scheme hierarchy `created_at` | Copied timestamp; global edges have no scheme/project ownership and remain outside scheme release content |
| Vocabulary `config_encrypted` | Excluded: live secrets, never portable content |
| Adoption/fork surrogate `id` | Excluded: local row identifier; logical composite identity and provenance are retained |

Migration 032 adds the three previously omitted fields without defaults or
historical backfill. A historical null cannot prove a false enforcement setting,
an absent fixed entry, or an edge creation time. Downgrade refuses to discard
captured values. The removed `weave_project_vocabularies` table is not a gap:
migration 013 replaced shared membership with direct vocabulary ownership.

The ledger proves classification and capture projections. The storage contract
below separately proves complete immutable owned rows. Metadata contract 1 remains
narrow. The domain DTOs and current
Git format also omit values: override supplements, concept/vocabulary sections,
explicit placements, credits, model type and project settings need assembler and
portable-format support. Section dispositions record these gaps; exact assembler
field mappings and serialization/restore coverage remain future work. This batch
does not change existing tag bytes or migrate export/browser consumers.

Validation includes real cuts of auxiliary/example/core models, null/blank/custom
namespaces, subsequent live edits/deletion and a second cut, concurrent edits,
capture failures, empty model membership, raw mutation rejection, old database
snapshots on either side of release creation, and migration
round trips that preserve unknown historical values.
Additional real-cut tests cover true/false enforcement, fixed-entry and hierarchy
timestamp preservation across a second cut, foreign entries after live deletion
and a newer source release, unavailable dependencies rolling back the cut, and
unknown historical fields surviving migration round trips.

## Owned-content contract 1

Migration 033 adds `content_capture_version` and a storage-owned JSON receipt on
the release. `WeaveFinalizeReleaseCapture` requests all final markers together,
after content, dependency evidence and the outbox have been written. The database
compares the full expected and archived row multisets for all 22 sections against
the same repeatable-read snapshot. Missing, extra or altered values abort the cut.
The versioned storage projection is checked against the reviewed capture ledger.
Adding a field requires updating capture, validation and its inventory decision.

The receipt records a count and SHA-256 for every section, including empty ones.
Hashes use sorted PostgreSQL JSON row values in UTC; they are storage evidence,
not the canonical Git/portable encoding. Only a release inserted in this same
transaction can receive a receipt. Its capture transaction, receipt, identity and
author/time cannot subsequently change. Title, retirement and materialization
policy remain separate catalog state. Source project releases must themselves
have complete owned-content evidence before a new consumer can finalize.

All 22 owned archives and the five project dependency/evidence tables reject
ordinary inserts, updates, deletes and key moves after finalization. Project pins
also freeze as soon as their prerequisite marker is set. Truncation is rejected
unconditionally, including on empty tables: stale snapshots cannot determine
whether a newer release exists. Deleting an unreferenced release permits its
archive cleanup; delete consumers before their retained sources.

Rows without project ownership columns resolve through their archived parent at
the same textual version. Persistent parent-identity coordination keys serialize
both parent and child writers, even when a stale snapshot cannot see the parent.
Both old and new ownership are checked on key moves; override refs also check the
actual parent so a forged project ID cannot bypass protection. Parent and
project/version keys survive cleanup to protect later reuse. Other projects may
use the same version string without sharing ownership or being blocked as sealed.

`readsession.Session.Content` reads detached receipt values on the existing bound
session. Draft, zero and closed sessions fail; an unverified release returns
`not_in_release`. This is the assembler's owned-content prerequisite. It does not
prove that existing consumers serialize every section, or replace the separate
ontology, descriptor and source-resolution contracts.

Detached receipt types and decoding live in `pkg/releasecontent`. Verified and
legacy receipts are distinct types bound to a concrete project/release. Decoding
requires the exact 22 reviewed section names, nonnegative explicit counts and
valid hashes; a map of the right size with a substituted section is rejected.
`CheckCounts` requires every loaded count, including zero. It checks row coverage,
not field mappings, reference closure or portable completeness. The receipt
registry is checked against the independent capture ledger. An imported document
cannot establish capture provenance merely by parsing successfully.

Legacy releases receive no automatic receipt. An explicit rebuild/repoint tool
may replace a controlled legacy snapshot set and recapture dependency pins in
source-first order. Normal readers and cuts have no compatibility switch that
turns current data into proof of an old snapshot. Portable restore will need its
own validated receipt path once the complete portable contract exists.

## Explicit legacy rebuild boundary

Migration 034 adds a durable operation journal for replacing a controlled legacy
set. `release.Service.PlanRebuild` accepts exactly one active, unverified release
per selected project, checks source/retention graphs and fingerprints current draft
inputs. `RunRebuild` requires a super-admin principal and owns an exclusive
maintenance lease across database capture and the supplied publication callback.
Normal runtimes and materializer entry points hold the corresponding shared lease.
An unfinished journal keeps them offline until the operation resumes successfully.
Older binaries and independent SQL/Git writers must be stopped operationally.

Rebuilding is an actual new capture under the existing version name. One locked,
repeatable-read transaction deletes consumers before retained sources, clears
owned archive rows, pins declared inheritance and runs the same capture pipeline
as normal release creation in source-first order. It preserves version/title/text,
records the rebuild actor/time, and creates fresh receipts and outbox rows. A
stale plan or any capture failure rolls the transaction back. Receipt/outbox
identity is checked again before publication, including recovery.

The materializer's explicit `PrepareReleaseRepoint` / `PublishReleaseRepoint` APIs
prepare immutable objects, retain previous branch/tag objects under operation refs,
and compare-and-swap a repository's release branch and annotated tag together.
Call them inside `RunRebuild`'s publication lifecycle, preparing all projects before
publishing any. This is the only intentional retag path; normal materialization
continues to reject conflicting existing tags. Publication is resumable, not an
atomic transaction across Git repositories and PostgreSQL, and needs an offline
window. The command host must persist its exact publication inventory in the
request. It must not restart writers or discard the operation record after a
partial publication.

The current Git format remains incomplete for portable whole-project restore.
A successful rebuild proves the database capture contract; it does not certify
that the current Git DTO serializes all 22 sections. Operational tooling and
rollout policy belong to the host application.
