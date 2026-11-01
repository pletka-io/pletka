# Browser detail reads

Model, collection, field and concept-list detail pages retain the existing
entity-view schema. `detailview.Host` requires one bounded `Source`; it has no
optional legacy content fallback.

`service/browserread.Source` opens the configured read session and assembles an
`EntityFragment`. A field also receives its project-scoped usage counts in the
same database snapshot. It returns `browserinput.Detail` after closing the
session. A fragment is not a complete inverse-usage inventory, so missing field
counts are an error rather than inferred zeroes.

The pure projection and its helpers receive values only. Conformance checks
restrict their imports and prevent access to handler capabilities. They produce
headers, contextual rows, concept-list entries, provenance, links and statistics.
Rendering can continue after the database pool closes. Current permissions and
working-copy actions are decorated separately.

## Selected content

- The page title and project breadcrumb use retained metadata. Authorization
  still uses current project visibility and ownership.
- Composition always uses the selected consumer's overrides. A foreign body is
  loaded from its captured source release; an empty consumer context stays empty.
  The provenance card offers **View source definition**, with the exact source
  project and release in its URL. Source links never infer ownership from ID text.
- Field bodies may have no base override. Raw paths are preserved, including
  authored terms that semantic generators cannot resolve. The browser does not
  reuse the stricter generator DTO or consult current ontology defaults.
- Base fixed values apply to contextual placements through the shared pure
  composition. Hidden fields and hidden collection placements do not appear in
  rows or composition statistics. Visible empty collection placements remain
  visible and count as collections. Breakdown ties have deterministic ordering.
- Concept lists retain their vocabulary, parent term, ordered membership, custom
  labels, entry metadata and sealed state, including an empty sealed list.
- Adoption and fork provenance comes from retained records. Adoption actor labels
  come from retained descriptors. Copied local categories are not assigned foreign
  provenance by guessing from matching system names.

All these paths support entities deleted from live tables. Missing selected
release content produces the canonical `not_in_release` response; missing working
roots produce `not_found`. Source authorization failures propagate. No content
fallback to a live foreign entity or to a differently selected source is allowed.

`EntityFragment.Resolve(context, kind, id)` resolves only an already selected
entity through retained membership. It cannot load another entity or choose a
new source version.

## Current operations and remaining consumers

Publication badges, integration configuration and lifecycle checks remain current
operational data. Lifecycle usage checks run only for editable working views;
their failures withhold delete actions. Release views remain read-only.

Global cross-project reuse has no historical selection contract. Release pages
omit its URL, and explicit release requests to the browser reuse endpoint are
rejected. Working-view reuse first validates selected entity membership. The
shared legacy MCP reuse builder is outside this browser migration.

The existing SKOS exporter combines selected list entries with live hierarchy
edges. Release detail pages therefore omit its link until that exporter consumes
retained input. Its working-view link remains available.

Project-page metadata, credits and ontology summaries now use their own
[bounded summary input](project-summary-reads.md). Paginated inventories and
adoption closure remain separate migrations. List pages must use bounded scoped queries, not a
complete-project capture. The temporary session-bound field-usage facet should
move into the field reader when that slice's remaining ordinary operations are
converted. Large-project performance qualification and capture-aware mutation
rollout are still separate gates.

See [whole-project assembly](../decisions/0012-whole-project-reads-belong-to-an-assembler.md),
[current authorization](../decisions/0005-pletka-auth-boundary.md) and
[generator routes](generator-routes.md).
