# Retained SKOS exports

`GET /api/v2/projects/{projectID}/concept-lists/{listID}/skos` exports the selected
concept list as the existing SKOS Turtle document. The response remains
`text/turtle; charset=utf-8`, downloaded as `<listID>.ttl`. No alternate JSON
response or viewer is introduced.

## Selection and authorization

The route uses the same project read, explicit version and default-release policy
as other content reads. `?version=1.0.0` selects that project release; a public
request without a version resolves the default release. Authorized working views
can select draft through the normal request policy. A missing/invalid scope is an
error, never a silent draft fallback.

The caller's project is the consumer context. A foreign list resolves through
that context's retained source membership and release pin. Its hot identity is
not required. Current root and source permissions still apply; a captured public
visibility does not grant access after the current project becomes private.
Content absent from a release returns the canonical `not_in_release` response.

Release detail schemas expose the existing `skos_url` capability with the resolved
project version. The existing concept-list component renders its download link
from that capability; it does not implement separate release behavior.

## Data boundary and format

The vocabulary host requires a `SKOSExporter`, implemented by
`generators.Service.GenerateConceptListSKOS`. The export-only HTTP handler has no
editing service or store. Generator selection uses the same required `Source`
as model/collection/field generators, verifies the returned consumer and root,
then passes a detached `EntityFragment` to `skos.RenderFragment`.

The pure projection uses the selected list, memberships, custom labels, concept
labels/scope notes and scheme hierarchy edges. Concept identities resolve within
the list owner's selected context. The list owner's retained effective concept
namespace expands `pletka:` identifiers, preserving the existing format's rule;
the consumer's current namespace cannot replace it. Missing required namespace,
concept data or invalid label data fails rather than consulting live tables.

The existing renderer emits only member concepts and edges whose endpoints are
both members. Global edges and edges belonging to other schemes are excluded;
an empty list still emits its scheme. Curator custom labels replace the original
preferred-label map as before. Ordering remains deterministic.

All source/projection work finishes before Turtle reaches the response. Errors
use the standard JSON error envelope, without a partial Turtle download.
`RenderFragment` can run after its database pool has closed. It adds no new
renderer JSON schema or Lua runtime; future script support can consume these same
detached inputs without acquiring a reader.

## Verification and limits

PostgreSQL regressions cover source repins, namespace/hierarchy/label changes,
deleted hot lists, empty lists, explicit/default releases, access revocation and
rendering after pool closure. Handler tests cover absent scope and partial writer
failure; conformance protects the export-only handler and pure generators.

Assembly uses the existing bounded entity-fragment limits. Large-list memory and
latency qualification remain follow-up work. Ordinary vocabulary editing,
autocomplete and global hierarchy browsing are separate consumers.

See [generator boundary](../decisions/0022-generators-consume-detached-inputs.md),
[browser details](browser-detail-reads.md) and
[source release selection](../decisions/0011-cross-project-reads-resolve-at-the-source-release.md).
