# ADR-0019: Imported project history keeps its origin

Status: Accepted (2026-10-31)

## Context

A complete project bundle carries original archived rows, source-release pins,
actor/module descriptors and ontology evidence. Restoring that history through
ordinary release capture would substitute current working copies, actor profiles
and ontology policy. The partial schema1/2 hydrator cannot preserve all captured
columns and spans several transactions.

## Decision

Schema3 restoration accepts a validated `projectbundle.ImportedProject` and its
separate catalog, including original author/date metadata for every selected
release. `database/projectrestore.Restore` owns root-first project locks and one
repeatable-read transaction. It restores exact ontology evidence, complete source
releases, the root release and editable working copies. Import never selects a
different source version or recaptures historical semantics.

Storage retains an immutable `weave_release_imports` ledger separate from the
original contract1/2 receipt. Imported finalization compares typed archive rows
and PostgreSQL receipt hashes with supplied values. It retains the original
descriptors and exact published/legacy artifacts. A deferred constraint prevents
committing an unfinished imported release. Local release capture keeps its existing
hot-table checks; finalized archive guards apply to both origins.

## Consequences

- Bound readers expose imported releases through the same archive path.
  `readsession.Session.Origin` reports the local import event separately. Its
  timestamp is installation metadata and must not enter frozen export bytes.
  Neither portable hashes nor a storage-validated receipt authenticate a sender;
  the request boundary still authorizes restoration and subsequent reads.
- Original project/entity/integer IDs are retained. Targets must be absent or an
  exact prior import. Retargeting and conflicting existing projects fail. Exact
  retries preserve subsequent working-copy edits and perform no new writes.
  This API does not merge additional snapshots into an existing working copy.
- Working copies use draft status and captured values. Hot-only columns retain
  schema defaults; missing staging attachments are cleared and counted. Archives
  retain their original staging IDs. Absent actors are seeded from descriptors,
  preferring the root context; historical source descriptions remain independent.
- Historical namespace inventories remain in archives. Only project-owned
  bindings are installed into working copies. Global bindings, active ontology
  choices and default policy remain destination policy. Restored drafts therefore
  do not claim the historical release's immutable interpretation context.
- Sequence-backed tables are locked while explicit IDs and sequence frontiers
  are installed. Project/ontology row changes roll back together; PostgreSQL
  sequence advances can leave gaps after rollback.
- Legacy evidence remains contract2 and is not promoted to publication. Lower
  ontology-only restore helpers retain their original meaning from
  [ADR-0018](0018-transported-ontology-evidence-keeps-its-origin.md).
- CLI and admin restoration dispatch by stored format. Schema3 bypasses the
  legacy DTO, ID remapping and multi-phase runner. The importer owns its locks;
  callers must not nest the legacy root lock around it. Schema1/2 behavior remains
  available for existing trees.
- Catalog entries lacking complete bodies cannot create empty releases. The
  default Git writer, incremental draft patcher and stored-format tag verifier
  migrate separately; implementing import does not switch their output format.

See the [bundle contract](../reference/project-bundle-format.md) and
[complete project values](../reference/project-snapshot-values.md).
