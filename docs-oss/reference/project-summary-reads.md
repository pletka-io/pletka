# Project summary reads

The project page and overview keep their existing schema builders. Their required
`Source.Summary` returns one detached `browserinput.ProjectSummary` after closing
a repeatable-read session. The HTML shell uses the same source's smaller
`Header` operation for its title and breadcrumb. Missing sources and missing
capture evidence fail; neither path falls back to current display metadata.

## Content and authorization

The selected header, owned entity counts, explicit adoption count, credits and
linked ontology summaries come from one session. The handler authorizes the
current root before loading and verifies the returned project and version.
Inherited source sessions apply current source authorization and the centrally
selected release pins.

`ProjectHeader.Project()` produces display data without owner or visibility.
Those captured fields must never become authorization inputs. Current permissions
control navigation and return-to-draft actions. Release history and the working
publication-change badge remain current operational information. Settings retain
their current operational view.

Counts are owned models, collections, fields, categories, examples and concept
lists. The adoption badge counts only explicit project-context model, collection
and field receipts, matching that tab's selection. It does not count automatic
context receipts as additional project adoptions.

Released credits join archived rows to retained actor descriptors, preserving
names, types, notes and order. Draft credits use actors from the same database
snapshot. Missing required actors fail rather than disappearing.

## Ontology summaries

Metadata is projected in SQL from immutable body evidence; full ontology bodies
and entity inventories are not transferred to the browser source:

- Own draft links use sealed bodies, including staged legacy versions without a
  publication. Catalog edits and active-version changes do not rewrite a seal.
- Verified release links use the exact captured publication and digest.
- Legacy release links use their retained local body inventory. They are never
  promoted to published semantics or filled from the current ontology catalog.
- Inherited links traverse captured parent relationships through authorized source
  sessions. Duplicate version IDs require the same body digest; conflicting
  bodies fail. The schema supplies the exact source project/release URL.

Usage is computed for the selected root project's references. Verified releases
use captured term resolutions. Legacy releases with unresolved assessments mark
usage unavailable; the existing widget shows totals and an explanatory label
instead of a fabricated zero fraction. Drafts match normalized authored references
against each selected source's exact bodies, including explicit version choices,
additional types and subfield paths. These draft counts describe matching authored
references, not semantic publication certification.

Release usage is aggregated in one query. Draft references are read once, then
matched in batches per selected source. A foreign body is never replaced by a
global seal with the same version ID.

## Bounds and remaining work

The summary reads aggregates and small metadata inventories. It does not assemble
a complete project or fetch every entity. Limits are 1,000 credit rows, 1,000
distinct linked versions, 32 selected projects/parents and 10,000 distinct draft
semantic references. Exceeding a limit fails the operation without a partial
successful value. These are safety bounds, not performance qualification.

Pure overview projection is checked by the browser import conformance test and
can run after the pool closes. Regression tests cover live deletion, parent
repinning, descriptor/catalog edits, current access changes, concurrent commits,
legacy ambiguity and credit overflow.

Model/collection/field inventories and the adoption tab/closure now use the
[scoped inventory boundary](project-inventory-reads.md). Concept-list/example
inventory tabs retain their existing release restrictions until their inventory
paths migrate.
Standalone ontology and SKOS exports, representative performance qualification
and capture-aware mutation rollout are also separate work.

See [browser details](browser-detail-reads.md),
[whole-project assembly](../decisions/0012-whole-project-reads-belong-to-an-assembler.md)
and [current authorization](../decisions/0005-pletka-auth-boundary.md).
