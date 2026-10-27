# Ontology draft operations

Ontology drafts are separate from immutable releases. This backend supports one
editable source document per ontology, revision-checked saves, and atomic
publication. The operator CLI uses the normal application database configuration
and `weave --instance` selection. The same lifecycle now backs the schema-driven
source editor and HTTP API.

## Browser editor

Super-admins can choose **Edit draft** from an ontology page. Create a blank draft,
or open a published version and choose **Start draft from this release**. Existing
drafts are offered as **Open draft** and are never replaced by that operation.
The editor page is `/admin/ontologies/{ontologyID}/draft/page`.

The existing schema form renderer edits name, prefix, namespace, description,
homepage/source URL and primary/companion RDF/XML files. Companions can be added,
removed or edited in the same save. Other descriptor/dependency settings remain
in the draft unchanged; the full-document API can edit them. This is a source
editor, not yet a structured class/property editor or graph preview.

**Save draft** preserves incomplete source text. **Publish saved draft** is a
separate confirmed action with a version label and optional active selection.
Source and dependency validation failures are shown visibly. After publication,
the page displays the receipt's version and offers further editing; publication
is offered again only after a new save. Existing version views and version lists
omit direct edit/delete controls when the editor is wired. Legacy storage/API
compatibility remains subject to the immutable-release guards.

A stale save preserves the form's unsaved values. **Compare with latest** prepares
a three-way comparison: independent changes combine, overlapping changes require
an explicit choice. **Apply review to form** updates the revision but does not
save; the user submits again. A later concurrent change produces another conflict.
If source ordering/filenames, permissions, endpoint or form fields changed, automatic
reconciliation is refused and the original values remain available for manual
review. There is no automatic retry that overwrites another edit.

## HTTP API

All editor and schema routes require the existing super-admin permission and
return canonical API errors. Responses are private and non-cacheable. Mutations
require `Content-Type: application/json`; request bodies are limited to 64 MiB.
The shared `/api/` middleware exempts JSON APIs from CSRF tokens, so accepting
browser-simple form/text content types here would be unsafe.

Relative to `/api/v2/ontologies/{ontologyID}/draft`:

| Method/path | Request or result |
| --- | --- |
| `GET /` | Current `DraftSnapshot`; absent drafts return 404 |
| `POST /` | Exactly one of `{"from_release":"VERSION_ID"}` or `{"document":{...}}` |
| `GET /{draftID}` | Snapshot bound to that owner and draft |
| `PUT /{draftID}` | `{"revision":N,"document":{...}}` |
| `POST /{draftID}/publish` | `{"revision":N,"version":"1.1","set_active":false}` |

Create/publication returns 201; reads/saves and identical publication replay return
200. Stale revisions and identity collisions return 409; invalid publication inputs
return 422. Snapshot `publication`, when present, is the immutable receipt for that
exact current revision; it disappears on the next edit. It is distinct from the
publication options inside `document`.

The same slice provides `page-schema`, `start-form-schema`, `/{draftID}/form-schema`
and `/{draftID}/publish-form-schema`. Form submissions use the emitted endpoints;
clients must not construct them. The form adapter derives its field inventory and
decoder from the same bindings, preserves unexposed settings, rejects unknown or
missing fields, and saves only through a bound `DraftEditor`. App wiring supplies
`EditorHost`; it owns no store or pool and cannot construct database capabilities.

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
Editor handlers receive the lifecycle service through application wiring and use
canonical API error adaptation; schema capabilities do not replace authorization. Migration 028 creates no draft or
publication from historical active/version rows. It refuses downgrade while drafts
exist; deleting published evidence is not part of this workflow.

## Verification

`npm --prefix frontend run test:forms` runs the generic reconciliation tests. The
required database tests cover the mounted HTTP API, authorization, source-owner
checks, revision conflicts, source/companion edits, receipt status and publication.
For a real browser regression, build assets and opt into Chromium explicitly:

```bash
npm --prefix frontend run build
PLETKA_EDITOR_BROWSER=1 make test-integration TEST_PACKAGES='./pkg/weave/ontology -run TestOntologyEditorBrowser -v'
```

This requires `node` and `chromium` on PATH. Missing prerequisites fail an opted-in
run. The test uses an isolated database, serves actual editor schemas and built
assets over loopback, and checks conflict review, validation and publication.
`PLETKA_EDITOR_SCREENSHOT=/tmp/editor.png` optionally saves the final view.
