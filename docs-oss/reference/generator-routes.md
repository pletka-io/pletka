# Generator HTTP routes

Use the consumer project in the URL:

```text
GET /projects/{projectID}/gen/models/{modelID}/{format}?version=1.0.0
GET /projects/{projectID}/gen/collections/{collectionID}/{format}?version=1.0.0
GET /projects/{projectID}/gen/fields/{fieldID}/{format}?version=1.0.0
```

The project identifies the composition context. An inherited field can belong to
a different project; the consumer's selected release supplies its exact source
pin and copied overrides. Do not replace the consumer with the field's owner or
apply the consumer's version number to the source project.

All three kinds support `diagram`, `turtle`, `jsonld`, `shacl`, `exportgraph`,
`cytoscape`, `sparql`, `x3ml-a` and `x3ml-b`. Models additionally support
`researchspace`, `arches`, `snapshot` and `ascii-tree`. Existing capability checks
and renderer registration still determine availability. `?bundle=zip` on X3ML
includes the snapshot's retained ontology artifacts. Other generator options can
be combined with `version`.

The browser's existing derivative schema supplies these URLs. Its UI components
continue to use the same responses and renderer inputs.

## Selection and access

Every endpoint uses one shared selection path:

1. Authorize the consumer using its **current** project visibility and ownership.
2. Resolve the explicit version, or apply the existing default policy: public
   non-editors see the latest release; editors and authorized private-project
   readers see the working state. A project with no releases can show its draft.
   Release lookup errors stop the request.
3. Load a bounded entity fragment through the configured generator source. It
   checks selected membership and authorizes captured source projects using
   current access metadata. No live root lookup is required.
4. Project and render the detached input.

An entity retained in a release can be rendered after its live row is deleted.
A live entity absent from that release cannot substitute for it. Missing release
content returns HTTP 404 with the canonical `not_in_release` code, including X3ML
bundle requests. A consumer inaccessible to the caller returns an existence-hiding
404 before content assembly; a denied source returns the canonical forbidden
error. Historical public visibility never grants current access.

`auth.ProjectFromContext` contains current authorization metadata. It is not the
selected project's historical display row. Content readers must receive explicit
scope and identity separately.

## Compatibility and remaining migration

Old `/gen/{kind}/{id}/{format}` URLs remain owner-relative aliases. They discover
the owner from a live entity, so they cannot identify deleted roots or a foreign
consumer context. New clients should use the project-qualified routes.

These routes migrate generator selection and browser derivative links. Browser
detail content now uses its [own bounded raw projection](browser-detail-reads.md).
Project metadata, credits and ontology summaries use a
[bounded summary input](project-summary-reads.md). Paginated inventories and
adoption closure remain separate migrations; list pages must use bounded scoped
queries rather than loading an entire project.
No Lua runtime or draft-capture enrollment is enabled by this change.

See [the generator boundary decision](../decisions/0022-generators-consume-detached-inputs.md)
and [current authorization](../decisions/0005-pletka-auth-boundary.md).
