# ADR-0021: Captured draft Git delivery uses ordered checkpoints

Status: Accepted (2026-10-31)

## Context

Atomic capture retains the exact state produced by each mutation, but PostgreSQL
and Git cannot share a transaction. A process can stop before publishing a ref or
after publication but before acknowledgement. Rebuilding current state on retry
would assign unrelated later edits to the pending author's commit. The legacy
initializer and reconciler also stage and overwrite the working directory.

## Decision

Enrolled projects publish pure schema3 content to `refs/heads/pletka-draft`.
This automation ref is separate from the user's HEAD, index and working files.
It must be a direct ref and must not be checked out in any linked worktree.
Unenrolled projects retain the existing legacy workflow. Enrollment is explicit
through the coordinated `InitProjectWithOptions` path (`--capture-draft` in CLI).
The migration enables no projects or request handlers automatically.

Migration 043 retains immutable revision-zero baselines, audit coverage mappings
and a per-project delivery journal. Existing pre-delivery baselines come from
retained snapshot digests, never current project reads. Preparation records the
revision, captured digest, parent, tree and commit identity. Storage permits only
the next revision after its predecessor is acknowledged, with one pending delivery.
Acknowledgement and matching audit/log completion happen in one SQL statement.

The writer checks the Git parent against the receipt's complete before value,
applies the pure row/context patch, and validates exact resulting bytes. Git
objects use an isolated index and explicit blobs without clean filters or line
conversion. Recorded timestamps and attribution make the prepared commit
reproducible. The ref advances by compare-and-swap. An already-published prepared
commit is acknowledged without another commit; missing prepared objects can be
recreated from retained inputs. Empty content changes still carry their revision.

Initialization, reconciliation and incremental delivery use the same protocol.
An explicit initialization/reconciliation can restore a missing acknowledged ref
from its checkpoint. An unexpected ref is an error, not permission to overwrite it.
Legacy writers recheck enrollment under the shared project filesystem lock.
Closed pending legacy audits are recorded as baseline-covered; old unassociated
capture-era audits are covered only after all their existing captured revisions
have been delivered. Their messages never become full-rebuild commit messages.
Release and archive-release outbox jobs remain independent and eligible.

## Consequences

The dedicated ref preserves existing staging intent as well as file bytes. Users
inspect it with `git show`/`git log`; the writer does not check it out or update
working files. A checked-out or symbolic automation ref blocks publication.

The draft ref contains complete frozen project content and selected source bodies.
Mutable release-catalog metadata is a separate overlay, published with immutable
release/tag bundles. A draft ref alone is not the complete history-import package:
that operation also requires an explicit catalog with original source headers.
No delayed user mutation reads today's catalog opportunistically.

Capture-aware request writers and performance qualification remain rollout gates.
There is no automatic conversion of existing traffic, repair of divergent refs,
or destructive rebuild of a missing repository with unavailable legacy ancestry.
The initial implementation still encodes full complete values; Git deduplicates
unchanged blobs. Retained delivery history blocks downgrade.

See [draft delivery operations](../reference/captured-draft-delivery.md) and
[atomic capture](0020-draft-mutations-own-their-capture.md).
