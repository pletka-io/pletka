# Atomic draft capture

`service/draftcapture` is the central mutation boundary for enrolled projects.
It does not yet enable the schema3 draft Git writer or enroll normal API traffic.
The immutable release writer remains independent.

## Operation contract

1. Construct `draftcapture.Service` with the pool, assembly limits and an explicit
   source authorization policy. Authorize root reads/edits at the request boundary.
2. `Enroll(ctx, project)` validates a complete draft and stores revision zero.
   Existing enrollment rejects; it never resets history. Coordinate this operation
   with Git initialization before exposing it in a normal request flow.
3. `Mutate(ctx, request, callback)` takes explicit project, actor and message.
   Propagate its context to every store call. Existing stores use `txscope.New`
   for queries and child transactions; the real change-log runner joins too.
4. A successful return exposes a committed `Receipt`. `Get(project, revision)`
   retrieves exactly the same immutable before/after values later. `Patch()`
   derives the changed rows and context, validates them and proves byte parity
   with the captured after snapshot. It never assembles current hot tables.
5. An external actor/default/source-policy change blocks an ordinary edit.
   Use `RefreshContext` under its own explicit message to advance that context;
   this operation accepts no write callback.

The owner acquires the existing restore-compatible project lock, global input
lock and capture project lock before opening a repeatable-read transaction.
This prevents waiting transactions from assembling a stale initial snapshot.
Storage fences additionally reject raw transactions started before enrollment.
All project changes, audit entries, raw effects, snapshot objects and revision
advance commit together. Failed capture or patch validation rolls everything back.

A scoped read session sees the owner's own writes and closes only its borrowed
reader lifetime. A nested store transaction is a savepoint; it cannot commit the
outer transaction. Nested transaction options are rejected. Override locks reuse
the owner connection and last through its commit. A context used after callback
return, or with a different pool, rejects. Do not share a pgx transaction across
concurrent callback goroutines. Failed operations are not retried automatically.

## Storage and coverage

Migration 042 defines immutable blobs/manifests, project heads, attributed mutation
receipts, raw row effects, parent tombstones and retained coordination fences.
Content-addressed storage deduplicates unchanged row and ontology artifact files.
Each snapshot still stores a complete path-to-blob manifest. The implementation
assembles complete values before and after an edit, rather than reconstructing
normalized context from raw row effects. This is a correctness-first cost that
requires measurement before enabling large projects.

The capture table/owner registry is ratcheted against the release capture ledger.
Runtime triggers cover INSERT/UPDATE/DELETE and reject TRUNCATE once any project
is enrolled. Raw effects include hot-only columns as well as all captured columns.
JSON columns become nullable text, retaining SQL NULL versus authored JSON null;
numerics and dates/timestamps retain exact text. No floating-point conversion is
used. Changes to generated/derived tables outside the inventory are not independent
project content and are not represented as extra authored rows.

Receipts must seal and advance their matching project head before commit.
Sealed history cannot be rewritten or deleted; downgrade refuses enrolled state.
Project moves, cross-project mutation callbacks, global writes inside a mutation,
and enrolled project deletion fail. Restore/import/maintenance callers receive no
special bypass. Retained history has no garbage collector yet.

## Delivery boundary

The legacy audit poll excludes enrolled projects. This prevents two writer
protocols from competing, but means enrollment must wait for the coordinated new
baseline and delivery workflow. Pending older audits, a Git commit acknowledged
late, staged/unstaged user changes, and recovery after a failed ref update are
requirements of that workflow, not solved by capture alone.

A full current-state rebuild must never be committed under an individual user's
mutation message. An explicit baseline operation and a per-project revision
checkpoint will identify the state represented in Git. See
[ADR-0020](../decisions/0020-draft-mutations-own-their-capture.md).
