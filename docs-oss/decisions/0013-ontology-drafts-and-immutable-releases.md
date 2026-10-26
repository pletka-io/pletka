# ADR-0013: Ontology editing uses drafts; dependencies use immutable releases

Date: 2026-10-26
Status: Accepted architectural direction; editor and enforcement not yet implemented

## Context

Ontology management will become an editor with releases. Project version browsing
showed why making every function remember a version is insufficient: a reader
that retains live access can silently mix released and draft content. The ontology
editor must start with the owner/selection/capability boundary of ADR-0010 and the
dependency rule of ADR-0011, rather than requiring that migration afterward.

The current materializer already treats ontology version IDs as stable artifact
identities. `vendorOntologyVersion` loads version content by ID, while restore's
`ImportVendoredVersion` returns early when that ID exists. The write side does not
enforce that assumption:

- `ontology.postgresStore.ImportVersion` deletes and recreates the version and
  its dependent rows under the same ID.
- `UpdateVersionMetadata` changes serialized metadata and the version label.
- Companion sources can be upserted, and term/relation rows remain writable.
- Relations have a polymorphic source ID but no version ownership column or
  source foreign key. Import comments claim cascading relation cleanup, but the
  schema and import path do not implement it; re-import can retain old edges.
- The vendor manifest reads ontology catalog metadata and global namespace aliases
  live; implicit defaults resolve through the active version at materialization.

A version ID alone therefore does not yet prove immutable content. The project
dependency inventory marker deliberately certifies project dependencies only.
It must not be reused as proof that ontology artifacts or complete output are frozen.

## Decision

An ontology has an editable draft and zero or more immutable releases. The
release identity includes its ontology owner and a stable release ID; its version
label is a human-facing identifier scoped to that ontology. Project and ontology
identities remain distinct types and foreign keys. Never pass an ontology ID to
an API that expects a project, or use a global textual version join.

The existing version-keyed ontology structures are the starting point for the
released representation. Editing gets a separate draft write capability.
Importing into an editor follows the same draft-to-release pipeline as manual
editing; it does not become a privileged overwrite path. The working default for
external published imports is immediate publication after a successful atomic
import. Whether the UI also offers import-as-draft is a workflow policy, not a
reason to make released content mutable.

The boundary exposes separate draft and release readers, bound to the ontology,
selection and one database snapshot. Release readers receive only immutable
artifact queries, with no draft store, pool or unrestricted query object. Methods
cannot retarget an owner or version. Missing selection, missing release and
missing released content fail closed. Authorization remains independent.

Publication validates and freezes the complete release in one transaction:

- Version metadata, source bytes and companion files.
- Classes, properties, relations, labels, constraints and module provenance.
- Catalog fields used in serialization, including prefix, namespace and name.
- Effective namespace bindings and the separate alias set needed to interpret
  source files. The collapsed effective map cannot replace that alias set.
- Concrete dependency identities for imported/base/default ontologies and their
  transitive closure. A mutable active/latest pointer is not a release pin.

Term identity and storage-row identity are distinct. A term may retain its logical
identity across draft and releases without sharing a mutable row between them.
The physical draft schema must be chosen with that distinction explicit.
Relations must carry enforceable artifact ownership too; polymorphic IDs and a
service comment are insufficient to protect them during sealing or deletion.

The active/default selection is mutable discovery policy, outside immutable
content. Resolve it once when publishing a consumer, then store its concrete pin.
Changing the active version cannot alter an already released project or ontology.

Enforcement belongs in construction and storage, not a convention on service
methods. Publication must seal all artifact tables together. Re-importing or
editing a sealed release must either prove byte/content identity and be a no-op,
or fail and require a new release. A reference to a sealed release prevents its
physical removal. Retirement is separate from deletion and preserves existing pins.

The materializer and restore use the same immutable artifact reader. They do not
reconstruct release metadata from live catalog rows, and an existing ID is not
enough to accept a restore: compare the immutable artifact identity/digest. The
future project assembler consumes these values through typed dependency readers.

## Migration and acceptance

Do not copy mutable ontology content into every project's private release snapshot
as the long-term solution. Establish the ontology's immutable artifact once and
let consumers retain it by identity. Existing imports cannot automatically certify
what an older project release originally saw. Keep historical evidence and current
state separate; any sealing of current data starts a new guarantee, not a claim
about past bytes.

Before routing released consumers through this boundary, add actual database tests
that attempt metadata edits, forced re-import, term/relation/companion mutations
and deletion. They must reject changes to a sealed release atomically. Verify that
draft editing and active-version changes still work. Materialize a project, edit
the ontology draft, publish a newer ontology, change defaults/aliases/catalog
metadata, and require the old project tree and restore round trip to remain
identical. Exercise missing pins, failed publication and concurrent editing.

Structural checks must cover importer, admin, editor, CLI and restore paths, not
only the ontology service file. The same enforcement applies to all writers.

## Alternatives rejected

- **Treat the existing version string/ID as proof of immutability.** Current
  import and metadata APIs disprove it.
- **Add optional scope arguments to the editor's existing store.** That repeats
  the project read leak: callers still retain a route to draft tables.
- **Freeze only RDF bytes.** Derived terms, companions, metadata and namespaces
  also affect reads and serialized output.
- **Let the materializer choose current defaults and aliases.** That changes an
  old artifact without any new release of its owner.
- **Give force-import an exception to sealing.** It would bypass the exact
  invariant on which project pins and restore depend.

Related: [ADR-0010](0010-versioned-reads-through-a-scope-bound-reader.md),
[ADR-0011](0011-cross-project-reads-resolve-at-the-source-release.md),
[ADR-0012](0012-whole-project-reads-belong-to-an-assembler.md).
