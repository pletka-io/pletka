# ADR-0013: Ontology editing uses drafts; dependencies use immutable releases

Date: 2026-10-26
Status: Accepted; publication, project consumers, portable restore, separate active policy and draft backend and schema-driven source editor/API implemented; structured term editing pending

## Context

Ontology management will become an editor with releases. Project version browsing
showed why making every function remember a version is insufficient: a reader
that retains live access can silently mix released and draft content. The ontology
editor must start with the owner/selection/capability boundary of ADR-0010 and the
dependency rule of ADR-0011, rather than requiring that migration afterward.

The current materializer already treats ontology version IDs as stable artifact
identities. `vendorOntologyVersion` loads version content by ID, while restore's
`ImportVendoredVersion` returns early when that ID exists. Before migrations 023–024, the write side did not
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

Migration 023 now supplies an explicit local-body seal and storage guards, as
described below. Migration 024 adds complete publication on ordinary import.
Existing unsealed versions retain their legacy mutation behavior and are not
automatically certified.

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

## Implemented foundation (2026-10-26)

Migration 023 gives relations enforceable version/source ownership. Existing
orphan rows remain evidence with no invented owner; new writes must satisfy the
constraints, and term-ID reuse cannot reconnect those orphans to new content.
Unsealed re-import now really removes the old owned relations by cascade.

`pkg/database/ontologybody.Seal` captures the local body and digest in a caller's
write transaction. Storage constructs the evidence itself and rejects subsequent
metadata, term, relation, companion and seal mutations, including cascade and
truncate paths. Body writers and sealing update the same parent revision, so a
stale repeatable-read operation fails serialization instead of missing a commit.
The seal retains its version and ontology; this initial implementation has no
unseal or purge operation.

The captured value includes catalog descriptors and the global namespace binding
context, preserving aliases and weights. Its format-1 digest is SHA256 of stored
PostgreSQL JSONB text with sorted arrays. This is a local evidence format, not the
final portable vendor/restore format. The ontology-specific bound reader receives
only generated SELECTs over seals, verifies the digest and fails closed for
unsealed/missing/wrong-owner bodies. Its callback owns a read-only snapshot.

At the body-sealing checkpoint, active selection remained a legacy mutable
`is_active` column excluded from the body. Migration 027 below moves it to a
separate policy record.

`pletka weave ontology-seal-body <ontologyID> <versionID>` explicitly seals current
bytes. It does not establish what an older project saw. Body-only sealing remains
a separate operation from the publication boundary below.

## Implemented publication boundary (2026-10-27)

Ordinary `Store.ImportVersion` now imports, seals and publishes in one
repeatable-read transaction. This includes counts, companions, aliases and active
selection; a failed cut rolls them all back and emits no import event. A committed
publication cannot be force-overwritten. The only legacy staging entry point is
`StageLegacyVersion`, restricted by conformance tests to the old Git vendor
restore path. Old snapshots carry no dependency evidence and must not acquire
current pins while being restored. Raw create/edit APIs can still stage legacy
rows, but released readers never expose them.

`ontologyrelease.Capture` owns the resolution policy:

- Resolve the declared base to an explicit owner/version pin or its active
  **published** version; compatibility labels constrain that choice and do not
  select another version heuristically.
- Resolve each `owl:imports` IRI to an exact version IRI, otherwise a unique
  ontology namespace/ontology URI and its selected published version. Ambiguity
  or absence fails. Exact explicitly external imports remain references; no
  network fetch or automatic external fallback occurs.
- Capture the `defaults` ontology once, using an explicit pin or its active
  published version. If no defaults catalog entry exists, record absence; the
  defaults ontology records itself. An existing but unpublished selection fails.
- Select every direct choice before merging source closures. Keep one version
  and digest per ontology; conflicting versions and cycles back to the consuming
  owner fail. A defaults upgrade can therefore require an explicit older pin
  when another source still depends on that older defaults artifact.
- Check relation and inverse-property targets against the local body and captured
  source bodies. Additional sources require explicit pins; missing terms require
  correction or an exact external-reference declaration. Namespace binding owner
  metadata is provenance, not proof of term ownership. Effective prefixes follow
  ADR-0004 through `namespace.Effective`, shared with project generators.

Migration 024 stores `weave_ontology_releases` and derives retained dependency
rows from each artifact. Storage requires repeatable read (or serializable), checks local body identity,
requirement inventory, source identities and closure containment, then computes
the artifact digest.
Foreign keys retain every source; update, delete, truncate and unsafe downgrade
are rejected. Exact selection/compatibility policy stays in the single publisher,
with conformance gates against raw publication query calls elsewhere.

Artifact format 1 contains owner/version, the exact sealed body JSON **as a
string**, its SHA256 digest, sorted closure pins and sorted direct requirements.
The artifact digest is SHA256 over the exact stored UTF-8 envelope bytes. Transport
must preserve these bytes; another database need not reproduce PostgreSQL's JSONB
formatting. Publication time and mutable active policy are outside the envelope.
`Artifact.Decode` verifies format, body ownership and both content hashes.

`ontologyrelease.Factory.Read` binds an owner/version to a read-only snapshot. Its
reader holds only generated SELECTs from `queries/read_ontology_release/`.
`Dependency(owner)` accepts only owners captured in that artifact, verifies their
pinned digest and never resolves active/latest. Zero, missing, wrong-owner and
unpublished selections fail before callback entry; escaped readers fail.

Existing stored bytes may be explicitly published with:

```
pletka weave ontology-publish <ontologyID> <versionID> \
  --pin <dependencyOntologyID>=<dependencyVersionID>
```

Publish dependencies first. `--external-import` and `--external-reference` classify
exact references and can be repeated; the same options exist on `ontology-import`.
HTTP multipart import accepts a `publication` JSON object with `pins`,
`external_imports` and `external_references`. Parse probes do not certify that
publication will succeed. This operator action establishes a guarantee from now
onward and never reconstructs historical content automatically.

## Implemented project capture and portable restore (2026-10-27)

Migration 025 adds a separate `ontology_dependencies_version` marker to project
releases and retained `(ontology_id, ontology_version_id, digest)` pins. Existing
cuts remain unverified. In the release transaction, `CaptureProject` inventories
archived ontology links, explicit `ontology_version_id` selections in archived
fields/models/collections, and every captured source project's ontology inventory.
It merges each publication's transitive closure and resolves defaults once for
direct selections. An explicit defaults link overrides active policy. Conflicting
versions, unpublished selections and unverified source inventories abort the
entire cut, including metadata, archives and outbox. Publish the intended current
ontology versions explicitly before cutting a new project release; this does not
repair an old release's missing evidence.

Storage checks links, explicit selections, source closures and inherited pins
before setting the marker. It rejects later pin changes and marker downgrades;
foreign keys retain the publications. `Factory.ReadProject` binds a concrete
project cut and exposes only captured artifacts through a dedicated SELECT
package. Missing/unverified selections fail before callback entry. Conformance
checks exclude live ontology tables and unrestricted database capabilities from
this reader and restrict raw capture queries to the capture boundary.

Released project manifests and ontology requirements now use frozen artifact
descriptors. The vendor writer receives artifact values and serializes captured
sources, companions and namespace aliases. New release commits and existing-tag
verification share the same snapshot writer, including vendored closure and
checksums. Draft materialization can still use the explicit legacy path for
unpublished rows; released materialization cannot.

Ontology vendor schema 2 includes exact `artifact.json` bytes and their digest.
`pletka.mod` records the digest alongside owner/version identity; `pletka.sum`
covers the vendor tree. Loading/restoring checks that requirements, vendor
manifests and source files agree with those artifacts. An existing published ID
is a no-op only for identical bytes and digest. An existing unverified ID is a
conflict, not permission to overwrite or certify it. Schema 1 remains an explicit
unverified legacy restore path.

Migration 026 restores the complete published bundle in one repeatable-read
transaction, dependencies first. It rebuilds indexed terms/relations/companions
from the frozen derived content, without reparsing RDF. The seal verifies indexed
content against the transported body and stores its exact text, retaining the
original descriptor and namespace context even when destination policy differs.
It does not rewrite destination global namespaces or an existing catalog entry.
Catalog family membership is restored only when the referenced family exists;
the artifact always retains the original descriptor. Original defaults decisions
survive transport; ordinary publication still checks current defaults policy.
Unsafe downgrade is refused once transported bodies exist.

This inventory does **not** yet resolve unversioned QName/URI references in project
content. Actor/module descriptors and other whole-project assembler inputs also
remain outside this checkpoint. Tests prove ontology-input stability and exact
artifact restore, not complete project-output immutability. Separate editor draft storage and active policy are now implemented as
described below; source-editor integration is also implemented below.

## Implemented active-selection policy (2026-10-27)

Migration 027 moves active selections to `weave_ontology_active_versions` and
removes `is_active` from version rows. The policy has at most one selected version
per ontology; a composite foreign key enforces owner/version identity. Migration
copies existing choices, including unverified legacy versions, and leaves absent
selections absent. Ambiguous legacy flags abort migration without choosing a
winner. No body, artifact, digest or publication evidence is rewritten. Migrated
policy timestamps record migration time, not a reconstructed selection history.
Downgrade preserves the choices in the former flags, without policy timestamps
or changes to artifacts.

`weave_set_ontology_active_version` is the single atomic selection operation.
Explicit changes replace the selection; restore initializes it only when unset.
The latter uses the same conflict handling as explicit selection, so a concurrent
admin choice cannot be overwritten by a read-then-write restore path. Invalid or
foreign-owner targets leave the current choice intact. Deleting an unsealed
selected version clears its policy by cascade without choosing a replacement;
published version retention remains enforced by the existing storage guards.

Create/import activation participates in the content transaction. Failed
publication rolls policy back with the imported content. Portable artifact
restore calls the same operation, and legacy restore uses the explicit
`SetActiveVersionIfUnset` store method. Conformance restricts generated selection
query calls to the shared store helper.

Live API/UI responses retain their `is_active` field through the read projection
`weave_ontology_version_status`. This is derived policy, not a second mutable flag
or release evidence. Current discovery/default resolution can use policy; bound
released readers cannot. Selecting an unverified version does not publish it.
Tests cover migration/down/up preservation, ambiguous history, foreign/missing
targets, raw write attempts, a synchronized initialization race, import rollback,
destination restore policy and byte-identical released output/artifacts.

Next, build separate draft storage and draft write capabilities for the editor.
Publishing those drafts must use the existing immutable publication boundary.

## Implemented draft backend (2026-10-27)

Migration 028 adds one `weave_ontology_drafts` document per ontology, with a
separate draft identity and storage-managed revision. Format 1 uses editable
RDF/XML primary/companion sources plus descriptor, namespace interpretation and
publication inputs. Source files remain authoritative; publication derives fresh
version-owned term rows using the shared RDF parser and import builder. Logical
URI identity is stable without sharing mutable storage rows with releases.
Unfinished sources may be saved, but cannot be published until valid.

`ontology.DraftService.Read` and `Edit` bind an explicit owner/draft reference to a
bounded repeatable-read callback. `DraftReader` has only draft SELECT capability;
`DraftEditor` adds only draft lock/save queries and cannot publish or retarget.
Expected revisions prevent lost updates. Callback errors roll back every save;
cancellation cleans up the transaction independently of the canceled context.
Architecture checks constrain imports, retained capabilities and query tables.

`Publish` locks the expected revision and uses the same body writer, seal and
closure publisher as ordinary imports. It never deletes an existing version ID.
Descriptor/namespaces, new content, active selection and immutable publication
receipt commit together. A receipt records the exact draft document, revision,
request and artifact identity. Repeating an identical request at the same current
revision returns that receipt without selecting active again or emitting another
event. A changed request or stale revision conflicts. Edits remain available after
publication. Imported events are emitted only after a successful new commit.

`StartFromRelease` copies frozen sources, descriptor, namespace provenance,
compatible-base labels and captured pins/external classifications into a new
draft; it retains the base artifact. It reparses at publication, unlike exact
artifact restore. Namespace merging preserves source, weight and ownership;
global bindings remain global. Ordinary external imports still publish directly
and do not overwrite a draft. There is no migration backfill from active rows,
and downgrade refuses existing drafts. Request authorization remains separate.

The public operator surface is `weave ontology-draft start/show/save/publish`;
see [operations and JSON format](../reference/ontology-drafts.md). That backend checkpoint
was followed by the source-editor integration below. Integration tests cover save/publication
rollback, stale edit/publication races, same-revision retries, namespace provenance,
raw guards and old released Git-tree stability after actual draft edits and newer
publication. Unversioned project references and the whole-project assembler remain
separate unfinished work.

## Implemented editor API and schema UI (2026-10-27)

`EditorHost` receives the transaction-owning `DraftLifecycle`, a catalog lookup
for new blank drafts, and immutable source-release sessions through app wiring.
The `/api/v2/ontologies/{ontologyID}/draft` routes authorize every operation and
schema read with the existing super-admin policy. They require explicit JSON
mutation bodies and use canonical errors. Adapters cannot retain or construct
store/query capabilities; conformance checks enforce that restriction.

The existing ontology page and generic form renderer provide source/metadata
editing, companion addition/removal and separate confirmed publication. One field
binding inventory produces both form schemas and submission decoding; other draft
settings are preserved. Catalog edit links lead to draft forms, and version views,
cards and lists omit direct content edit/delete controls. Active selection and
external-import workflows remain separate. No legacy version is silently converted
or certified. Existing browse read models have not all been converted to artifact
readers by this UI checkpoint.

A narrow draft SELECT exposes the immutable publication receipt for the current
revision. The UI can therefore distinguish unpublished edits from an already
published revision without choosing current active/latest policy. The draft reader
still has no version-content queries or writes.

Generic form reconciliation is opt-in schema capability: a hidden revision,
form/source-layout identity and message. A 409 preserves local values; comparison
against the original and current values merges independent changes and requires
explicit choices for overlaps. Applying a comparison advances the revision only
in the form; saving remains explicit. Changed layouts/targets/permissions refuse
automatic reconciliation. Unmapped validation errors are shown as a summary.
All editor forms share language/cancel defaults and render as full-page form
transitions. The renderer has no ontology-specific branches or mutation URLs.

HTTP/database tests and an opted-in real Chromium regression exercise these
boundaries, including source validation during publication and old-release
immutability. See [operations/API](../reference/ontology-drafts.md). Structured
term editing, richer preview and dependency-option editing UI remain separate
work. [ADR-0015](0015-project-semantic-references-at-release-cut.md) now covers
unversioned project reference capture. The whole-project assembler and consumer
coverage remain unfinished; this does not establish full project-output immutability.

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
