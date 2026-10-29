# ADR-0018: Transported ontology evidence keeps its origin

Status: Accepted (2026-10-29)

## Context

Legacy project releases can preserve multiple ontology versions and unresolved
references. Git must transport their exact evidence without reparsing source RDF
or replacing the captured interpretation context with destination policy. The
current project hydrator restores a partial editable representation; it cannot
recreate a complete structural release certificate.

## Decision

Project and module manifest schema 2 both bind a fixed `ontology-snapshot.json`
file by release version and SHA256 digest. The file contains the exact captured
ontology snapshot, including original selections, references, assessments, body
bytes and source contexts. Concrete ontology version IDs identify bodies inside
the document. Human version labels do not select bodies or filesystem paths.

Restore validates the complete evidence bundle before writing, restores sealed
body bytes without RDF parsing, and retains the original documents atomically in
`weave_imported_legacy_ontology_snapshots`. This is imported evidence, separate
from `weave_legacy_ontology_snapshots`, whose contents storage derives from local
release archives. Imported evidence creates no local release, completion receipt,
ontology publication or active ontology selection.

## Consequences

- `Factory.ReadImported` requires the original project, release and exact digest.
  Its reader has only imported-evidence SELECTs. The existing captured reader
  never falls back to imported evidence, and neither has a mutable ontology store.
- Existing concrete body IDs require identical sealed bytes. Unsealed collisions
  fail; restore cannot overwrite or certify them. Documents, source identities
  and digests remain unchanged. Imported evidence is immutable and retained.
- Storage validates the source graph and assessment coverage before retaining an
  import. An immutable context ledger binds each original project/version across
  separate imports, so a later bundle cannot replace a previously retained source
  interpretation. Concurrent conflicts fail; incomplete documents cannot occupy
  an immutable import key. Readers independently validate the detached artifact.
- Legacy `body_digest` requirements are distinct from published
  `artifact_digest` requirements. Manifest selections and the complete body
  inventory must agree with the snapshot. Missing evidence, changed digests,
  unknown formats and identity conflicts fail before hydration.
- Schema 2 omits the project's flattened `namespaces.effective_bindings` field.
  The complete weighted registry is retained in the exact snapshot and its source
  contexts, including tied aliases. Export does not promise a unique effective
  map or invent a winner. Import retains that evidence without rewriting current
  project namespace policy. Consumers requiring an effective map must still use
  the strict [namespace boundary](0004-namespace-resolution-boundary.md).
- Independent published roots may select different versions of the same ontology
  in one bundle. Each publication still validates its own dependency closure;
  differing bytes for one concrete identity remain a conflict.
- Retargeting a legacy snapshot to a different project is rejected until an
  explicit provenance mapping exists. Editing restored drafts does not change
  the retained original evidence or certify newly authored semantics.
- The release rebuild command explicitly selects the legacy contract. Its Git
  preparation verifies contract 2 and uses the same ref checks and recovery as
  ordinary rebuilds. Ordinary publication keeps its existing validation.
- This format establishes exact ontology transport. Complete project portable
  serialization and restore remain part of the assembler migration, and the
  existing multi-phase project hydrator is not an atomic complete-release restore.

See [ADR-0016](0016-legacy-ontology-snapshots-preserve-evidence.md) and
[ADR-0017](0017-legacy-release-completeness-is-a-separate-contract.md).
