# Ontology draft operations

Ontology drafts are separate from immutable releases. This backend supports one
editable source document per ontology, revision-checked saves, and atomic
publication. The operator CLI uses the normal application database configuration
and `weave --instance` selection. The HTTP editor and UI are not wired yet.

## Start and edit

An ontology catalog entry must already exist. Start from a published version ID
owned by that ontology:

```bash
pletka weave ontology-draft start ONTOLOGY_ID --from-release VERSION_ID
pletka weave ontology-draft show ONTOLOGY_ID
```

The response contains `ontology_id`, `draft_id`, `base_version_id`, `revision`
and `document`. Starting again conflicts instead of replacing existing work.
A legacy unverified version cannot serve as the source; publication of current
legacy bytes is a separate explicit operation (ADR-0013).

Edit the `document` object and save it as JSON. For example, `show` piped through
`jq '.document'` extracts it for a file editor. Pass that object, not the complete
snapshot response, to `save`:

```bash
pletka weave ontology-draft save ONTOLOGY_ID DRAFT_ID --revision 1 --document draft.json
```

Use the revision returned by the last successful read/save. A concurrent change
returns a conflict; reload and reconcile the documents. The returned revision
increments on each save. Unknown JSON fields and extra trailing JSON are rejected.
`--document -` reads standard input. Saving does not change catalog descriptors,
namespace policy, active selection or any version rows.

To start without a source release, use `start ONTOLOGY_ID --document draft.json`.
A minimal format-1 document is:

```json
{
  "format_version": 1,
  "ontology": {
    "prefix": "example",
    "namespace": "https://example.org/ontology/",
    "name": "Example ontology",
    "ontology_type": "base"
  },
  "primary": {
    "filename": "example.rdf",
    "content": "<rdf:RDF xmlns:rdf=\"http://www.w3.org/1999/02/22-rdf-syntax-ns#\" xmlns:owl=\"http://www.w3.org/2002/07/owl#\"><owl:Class rdf:about=\"https://example.org/ontology/A\"/></rdf:RDF>"
  }
}
```

Format 1 uses RDF/XML primary and companion source files as the authoritative
editable representation. Unfinished RDF may be saved; publication requires valid
RDF/XML and valid dependency closure. Optional `companions` contain `filename`,
`content` and `description`. Filenames must be distinct bare names. Editable
input has no version IDs, term row IDs or active flags. Terms keep their logical
URI across publications but receive new version-owned row IDs.

`namespace_bindings` records interpretation inputs (`prefix`, `namespace`,
`weight`, `source`, optional `ontology_id` provenance). A missing provenance owner
means global. Copying a release retains these values, its exact sources, compatible
base labels and explicit dependency pins/external classifications. The new release
is derived by the current parser; copying sources is not an exact artifact restore.

## Publish

```bash
pletka weave ontology-draft publish ONTOLOGY_ID DRAFT_ID --revision 2 --version 1.1 --set-active
```

Publication locks the expected revision and runs the shared import body writer,
seal and dependency publisher in one repeatable-read transaction. Catalog changes,
namespaces, new terms/relations/companions, optional active selection and an
immutable receipt commit together. Any failure rolls them all back and leaves the
draft available for repair. An existing version identity is never overwritten.
Omit `--set-active` to preserve the current selection.

The receipt identifies the draft revision, published version and artifact digest.
Repeating the same request for the same **current** revision returns that receipt
with `created: false`, without changing current active policy or emitting another
event. A different request for an already-published revision conflicts. Save a new
revision to publish new work; change active selection through its separate policy
operation. A retry after subsequent edits conflicts because the expected draft
revision is stale. Stored receipts retain the exact published draft document.

Optional `publication` settings use owner-to-version-ID `pins`, `external_imports`
and `external_references`, as defined in
[ADR-0013](../decisions/0013-ontology-drafts-and-immutable-releases.md). Missing pins
use the existing publication policy at cut time. References never silently fall
back to a draft. Ordinary external imports still publish immediately and leave
editor drafts alone.

## Application integration

Use `ontology.DraftService` as the lifecycle boundary. `Read` exposes only a
`DraftReader`; `Edit` exposes a separate `DraftEditor`. Both bind an explicit
owner/draft reference and a transaction to a callback. Their methods cannot
retarget another owner or publish. Capabilities reject use after the callback;
values may escape. Save results are tentative until `Edit` returns successfully.
The generated reader package has only draft SELECTs; the generated writer package
can only lock/save draft rows. Architecture checks forbid wider query capabilities.

Authorization belongs at the request boundary and is independent of selection.
Future editor handlers must receive the lifecycle service through application
wiring and use canonical API error adaptation. Migration 028 creates no draft or
publication from historical active/version rows. It refuses downgrade while drafts
exist; deleting published evidence is not part of this workflow.
