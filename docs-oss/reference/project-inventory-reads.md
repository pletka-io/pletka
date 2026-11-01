# Project inventory reads

Model, collection, field, concept-list and example inventories and their filter options require the shared
`browserhttp.Source.Inventory` boundary. The adoption tab and its drilldowns
require `browserread.Source.Adoptions`. Both return detached values after closing
one repeatable-read session. Neither operation uses a complete project assembler
or loads ontology bodies.

## Selection and membership

Handlers authorize the current root, require an explicit draft or release scope,
and verify the returned project/version. The session authorizes current source
projects and resolves every foreign identity through the captured or planned
source-release graph. A source version is never selected independently by a list
handler. Deleted live identities remain readable from the selected release.

Model/collection/field inventories combine owned rows, explicit project-context receipt seeds and
foreign structural references from the consumer. Merely inheriting a project
does not list its unused definitions. Reference membership includes override
containers, fields, typed model/collection targets, collection placements and
`part_of_collection_id`. Own/fork provenance wins over adoption; explicit receipts
win over reference provenance. A receipt without a consumer reference is marked
“not yet referenced.” Receipt closure members are not automatically list members.

Search, origin, scope, category and model-type filters apply before pagination to
all three membership groups. SQL returns small sorted prefixes and exact totals;
the source merges these using the same byte ordering and stable ID tie-breaker.
Only rows on the requested page are hydrated. An empty page preserves the total.
Search examines translation values, not language keys. Filter labels come from
the selected source categories. Missing required content fails without a partial
successful response.

Composition and usage counts describe the selected **consumer** context. An empty
consumer composition is not substituted with the source's composition. Counts are
computed for the page in one pass over compact consumer relation data. Global
cross-project usage is omitted because it has no historical selection contract.
Working publication-change badges remain separate current operational data and
are queried only for owned draft rows. Released rows never call that live reader.

The existing schema UI is reused. Its optional `detail_url_field` selects the
server-provided per-row URL, preserving the resolved release or explicit draft.
Source links always name the actual owning project and selected source release.

## Adoption bill of materials

Only explicit project-context model, collection and field receipts become cards.
Receipt names use selected source rows; adopter names use retained release actor
descriptors (same-session actors for draft). Counts and expanded lists derive from
the same source-relative traversal, with cycle detection and shared read caches.

The traversal follows typed `resource_model` and `collection_model` override refs,
collects attached fields, includes a model/collection seed in the transitive set,
and includes a field seed in both field sets. Collection placement is a separate
relationship, not an additional receipt traversal edge. Every transitive foreign
reference is resolved relative to its source's captured membership. Purely
transitive definitions that are not consumer members link to their pinned source
context. Missing released receipts use the canonical `not_in_release` error.

## Bounds and verification

Limits fail explicitly: 32 projects, 100,000 root relation rows, 10,000 receipt or
fork records, 1,000 explicit adoption cards, 10,000 distinct closure entities,
100,000 cached closure edges and 100,000 expanded receipt rows. Inventory pages
allow 1,000 rows, a 20,000-row prefix per source and 100,000 total merge keys;
facets allow 1,000 distinct scopes/categories per selected source. These are
bounded initial implementations; representative performance qualification and
cursor pagination beyond that merge window remain follow-up work.

Real PostgreSQL tests cover union pagination and filters, explicit/reference
precedence, cycle closure, distinct source versions, repinning, live deletion,
retained actors/category labels, current source/root authorization, concurrent
writes between indexing and hydration, facet overflow and rendering after pool
closure. Conformance tests protect state-specific SQL, pure projections and
mandatory delegation by all five list/filter handler families.

## Concept lists and examples

These two inventories list owned rows. Compact vocabulary/parent-term and target
identities resolve through the selected dependency graph before filtering and
pagination. They do not enumerate a whole project or vocabulary, load example
values, or invoke an editing service to obtain labels. Database filtering,
sorting and pagination return exact totals, including an empty page. Concept-list
entry and distinct bound-field counts use the selected consumer state; vocabulary
facets use the same owned membership. Authored concept-list/example statuses are
preserved rather than replaced by publication badges. Example target names retain
translations for the request language.

The bound catalog facets allow 10,000 distinct reference identities, 128
project/kind label batches, 1,000 vocabulary facets, and the same 1,000-row page /
20,000-row window as other inventories. Overflows fail without partial values.
Metadata/count work remains database-side; large-project performance qualification
is still outstanding.

Project release tabs are enabled. Concept-list links use the existing detached
detail viewer. Examples use **one `example-form` schema and one
`ExampleWorkspace` component** for draft editing and read-only browsing. The
schema's GET `view` action opens that same workspace. There is no release-only
viewer or alternate detail JSON format.

Example detail and form-schema reads require the same bounded source fragment.
A source-only handler passes detached values to `example.ProjectRead`; it cannot
reach the editing service. Shared form construction and validation use the
selected composition, including nested collections in their selected owner's
context, concept memberships/custom labels, linked examples and nullable legacy
scalars. Saved values that no longer fit a field remain in `unplaced_values`.
Validation reports derive from retained inputs; the stored example status is
preserved. Target links retain the example's owning context, while linked
examples point at their selected owning release.

The detail endpoint returns the existing `{example, values, validation}` contract
in both states. HTML requests redirect to the project tab with the selected
release. The form-schema endpoint accepts `create`, `edit` and `view`: `edit` on a
release returns the same form without a submit endpoint. Only an authorized editor
of the owning draft receives POST/PUT capabilities; `view` always omits them.
The workspace derives editing controls from that endpoint's presence, so the
server's capability decision governs the same component in every state. Write
routes still enforce authorization and reject release scopes independently.

The schema carries retained concept choices, including an empty list when there
are none. The workspace/picker does not resolve recorded concept labels through a
global live vocabulary endpoint. Read-only loading needs only the form schema;
authorized editing also loads the scoped example inventory for link choices.
Mutation-time validation uses the same selected projection through a mandatory
transaction boundary; parent values and stubs save atomically. See
[example mutations](example-mutations.md).

Resolved release schemas pin data, filter, form, model-option and navigation URLs,
including public requests that default to the latest release. Draft links omit
the version parameter: `draft` is not a supported release selector. Generic row
URLs are authoritative; the browser does not append an unrelated ambient version.

Real-cut tests cover pinned foreign vocabularies/targets, nested composition,
concept choices, empty consumer composition, stored statuses, stale values,
linked examples, live deletion, capability gates, default selection, current
source access and projection after pool closure. An opt-in Chromium test uses
actual routes and built assets to check the shared workspace, retained labels,
absent mutation controls/live enrichment and a real authorized draft save.
Run it after building frontend assets with
`PLETKA_EXAMPLE_BROWSER=1 bash test/integration.sh ./pkg/weave/projectpage -run TestCatalogInventoriesAndExampleSchemaRetainRelease -count=1 -v`.


SKOS/standalone ontology exports, remaining slice reader migrations and
capture-aware mutation rollout are separate work.

See [project summaries](project-summary-reads.md),
[browser details](browser-detail-reads.md) and
[version-aware reads](../decisions/0010-versioned-reads-through-a-scope-bound-reader.md).
