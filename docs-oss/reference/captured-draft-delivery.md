# Captured draft delivery

Explicitly initialize an enrolled draft through:

```sh
pletkactl project init-git PROJECT --capture-draft
```

This requires capture-aware writers: enrolled content writes must enter
`draftcapture.Mutate` and propagate its callback context. The flag does not wire
normal API/MCP requests automatically. Unenrolled projects retain legacy
materialization. Migration 043 enrolls nothing. The new mode always includes
complete selected source bodies and ontology evidence; `--self-contained` is only
needed by the legacy format.

## Baseline and normal delivery

Initialization retains revision zero and publishes it under an explicit system
baseline message. Closed pending draft audits are mapped to that baseline and
marked `baselined` only after its publication. Open draft change sets block new
enrollment. Pre-existing capture-era audits without an exact association are
marked `covered` only after the last already-captured revision is delivered.
New audit rows are linked to their mutation transaction's revision.

`ProcessPending` drains revisions in order and still processes release and
release-archival jobs. Each revision gets its own commit, including operations
whose normalized content is unchanged. The message, author, date and exact content
come from retained records. A full project rebuild is never assigned to a user's
pending message.

Published content lives at `refs/heads/pletka-draft`:

```sh
git log pletka-draft
git show pletka-draft:project.yaml
```

HEAD, staged index and working files stay untouched. Do not check out this ref
while the materializer owns it. A symbolic or checked-out ref blocks publication.
Git attributes, clean filters and line conversion cannot alter captured bytes.

## Failure and recovery

The database journal distinguishes preparation from acknowledgement. Preparation
records the exact parent/tree/commit and captured snapshot digest before the
public ref moves. The writer then compare-and-swaps the ref and acknowledges it
with audit/log completion in the database.

- Before ref publication: retry reproduces the same prepared commit, even if its
  unreferenced Git objects have been pruned. Recorded ancestry must still exist.
- After ref publication but before acknowledgement: retry recognizes the same
  prepared commit and acknowledges it without adding another commit.
- A ref at an unexpected object: stop and report the conflict. No automatic
  overwrite, reset or current-state rebuild occurs.
- A missing acknowledged ref: explicit `project reconcile PROJECT` or repeated
  initialization can restore the exact recorded commit. The poller does not infer
  that deleting a published ref authorized a repair.
- Database capture failure: no receipt or content change survives to deliver.

`Reconcile` drains retained revisions up to its observed head and validates the
checkpoint. It does not absorb external context drift; that needs a separately
attributed `RefreshContext` operation. Delivery failures remain pending and are
logged; the explicit reconcile command returns the error directly.

## Content and release catalogs

The captured ref contains the pure `projectbundle.Encode` representation of a
complete draft. It includes all 22 sections, exact interpretation context and
selected immutable source bodies. Release catalogs are a separate metadata
overlay. Immutable release/tag publication retains that overlay for history
restoration. To import a draft together with source release history, supply an
explicit complete catalog as required by the history importer; the content ref
alone does not manufacture original release headers.

See [ADR-0021](../decisions/0021-captured-draft-git-delivery.md). Capture still
assembles full values and delivery encodes them; measure these costs before
rolling out to large projects. No production enrollment is implied by this code.
