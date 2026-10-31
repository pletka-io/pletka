# ADR-0020: Draft mutations own their capture

Status: Accepted (2026-10-31)

## Context

Domain audit entries describe intent. They omit some project tables, prior child
rows and cascading effects. Reading current fragments after a save can attribute
a later edit to the earlier author's commit. A complete draft patch therefore
needs retained inputs from the writing transaction, not more optional log calls.

## Decision

`service/draftcapture` owns an explicitly enrolled project's mutation transaction.
`Enroll` records revision zero; `Mutate` records one attributed, ordered revision.
Store adapters route through `database/txscope`, joining the callback's transaction
or using child savepoints. Read sessions borrow that transaction through their
existing SELECT-only facets. Infrastructure owns commit and rollback; slice stores
cannot commit the parent. The ordinary change-log runner uses the same routing,
so its entity writes and audit flush are also atomic.

Storage guards all 22 captured hot tables. Enrolled project writes require an
open receipt in the same transaction. Effects retain generated fields, cascading
children and every hot column, with nullable JSON text and exact numeric/time
representations. Parent tombstones preserve ownership after cascade deletion.
Per-project locking orders revisions; MVCC fences reject transactions whose old
snapshot predates enrollment, including moves out of the project. Lost callback
contexts and different pools fail instead of writing outside the receipt.

The capture owner assembles and validates complete before/after values on that
transaction. Immutable content-addressed blobs and manifests retain their exact
schema3 bytes. `Receipt.Patch` derives explicit changes only from those retained
values, then verifies pure patch replay reproduces the captured after value.
Delayed delivery never reads current content. Effects preserve raw storage changes
that the normalized project representation may intentionally omit.

External actor/default/source-policy changes require a separately attributed
`RefreshContext`; an ordinary edit rejects context drift before calling its write
callback. Source selections become concrete immutable releases during capture.
Global input writes cannot be hidden inside one project's mutation.

## Consequences and limits

Enrollment is opt-in. Migration 042 enrolls nothing, and the legacy poller excludes
enrolled projects. Coordinated baseline initialization, reconciliation, Git ref
CAS and crash recovery must land before enabling this for normal traffic. The
capture API supplies no automatic retry for an ambiguous client acknowledgement.

The initial implementation assembles the full project per mutation and retains
full manifests. Unchanged row/artifact blobs are deduplicated, but assembly cost,
manifest growth and raw effect history still need measurement before rollout.
Project serialization trades same-project write concurrency for attributable order.
Session locks require direct PostgreSQL connections, not transaction-pooling
PgBouncer. Callbacks are sequential database work, with cancellation/deadlines
supplied by the caller; external side effects belong after commit.

Root edit authorization remains the caller's responsibility; source authorization
is required when configuring capture. SQL guards protect application invariants,
not against a database administrator disabling triggers. Enrolled project deletion,
bulk restore and maintenance have no implicit bypass. Deletion needs an explicit
terminal lifecycle; other operations must use capture or fail. Downgrade refuses
retained enrollment/history. No automatic history garbage collection is provided.

See [atomic capture](../reference/atomic-draft-capture.md) and
[pure patch rules](../reference/draft-project-patches.md).
