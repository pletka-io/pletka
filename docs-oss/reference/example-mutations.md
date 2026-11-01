# Example mutations

Example saves validate against the same selected definitions as the shared
[example form](project-inventory-reads.md). The public create/update inputs,
`{example, values, validation}` response and `example-form` schema are unchanged.

## Mandatory operation boundary

Construct `example.NewPostgresMutations(pool, source)` with the same authorized
browser source used by forms, then pass it to `example.NewService`. The service
has no store, live view reader or optional validation fallback. Its
`Create`/`Update`/`Delete` methods require an explicit valid draft `ReadScope`.
Handlers retain responsibility for current root edit authorization; the source
checks current access to selected foreign projects.

The operation retains its project, context and transaction privately. Validation
and persistence methods cannot select a different project or forget the bound
context. A shared fragment projection resolves model composition, nested
collections, concept membership and constraints. Foreign references resolve
through the consumer's selected source membership and release pins. Inheriting a
project alone does not authorize every entity in it as a reference candidate.

Existing parent metadata is read through an owned draft content reader without
traversing its old value graph. Update/delete cannot target another project's
example. Delete can remove an owned example whose model is no longer resolvable.
Linked targets use owned draft metadata or the selected foreign fragment; there
is no global hot-example lookup.

## Atomic save and conflicts

`txscope.Run` owns one repeatable-read transaction, or a savepoint when an outer
operation already owns one. Every read session and store joins that transaction.
Parent values and all typed-label stub examples commit or roll back together,
including failure during validation, persistence or outer capture. Submitted
values are copied before normalization and stub-ID assignment.

Form loading and saving are separate requests. A save validates against the
selection at the start of its transaction; it is not an optimistic concurrency
token for the earlier form response. Existing database project fences reject
conflicting writes during the operation. Serialization/deadlock failures use the
standard `conflict` response (HTTP 409), asking the user to reload and retry.
There is no automatic retry or source repin within a save.

Ordinary saves do not enroll projects in capture. An enrolled project still
requires an enclosing `draftcapture.Mutate`; the example boundary joins it and
cannot commit it independently. See [atomic draft capture](atomic-draft-capture.md).

## Active links and retained evidence

Migration 044 removes the linked-example foreign key to the hot table. A valid
target can exist only in an immutable source release; deleting its hot row must
not erase a foreign consumer's pinned reference.

The mutation adapter checks every normalized active `linked_example_id` before
writing, including values on removed or moved slots that skip field validation.
Missing or unselected targets reject the save. Ordinary validation issues still
produce the existing `has_issues` status; this does not turn the editor into a
valid-values-only store.

Deleting an owned draft example explicitly clears active links only in its own
project. Raw value payloads remain available for the existing missing-link
validation/display. Foreign consumer links remain attached to their selected
source releases. Low-level import/restore writers must continue to validate their
complete selected graph; the removed hot foreign key is not a substitute for that
validation.

Downgrade attempts to restore the old foreign key and fails if archived-only
links cannot satisfy it. It never discards or rewrites those links to succeed.

## Verification

Conformance tests prevent live validation readers returning to the service and
require its adapter to use the shared transaction runner. PostgreSQL tests cover
pinned definitions, nested concept membership, archived-only links, stale slots,
ownership/source access, parent/stub rollback, concurrent changes, capture joins
and migration downgrade refusal. The shared-workspace Chromium test also saves a
draft through the original form endpoint.
