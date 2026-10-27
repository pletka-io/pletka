# ADR-0015: Resolve project semantic references at the release cut

Date: 2026-10-27
Status: Accepted; capture, storage verification and bound inventory reader implemented.

## Context

Project ontology links and explicit `ontology_version_id` values do not cover all
semantic inputs. Path editors also persist QNames and URIs without version IDs;
additional types do not even have a version-ID field. Selecting current ontology
content later can change an old release. Legacy subfield paths were also missing
from field archives altogether.

## Decision

The release transaction resolves every declared semantic input against its
selected published ontology closure. Selection comes from project links, explicit
version IDs, captured source-project closures and the defaults policy resolved at
the cut. No global term search, prefix-to-owner inference or later active-version
lookup may fill a gap. An unlinked user ontology must be linked or explicitly
pinned before the cut succeeds. Standard-vocabulary-only projects capture defaults
even when they have no explicit ontology links.

Conformance tests require explicit inventory review when path attributes or
path-bearing domain locations change. One SQL inventory drives both capture and
storage coverage:

| Location | Interpretation |
| --- | --- |
| Field/model/collection `ontology_scope` | Main term and additional types |
| Field `path_elements[]` | Class/property terms, additional types, literal term and datatype |
| Field `subfield_paths[].path_elements[]` | The same complete path-element contract |
| `additional_types[]` | Class references, independently resolved |
| `datatype` | Class/datatype term, including standard vocabulary terms |

Absent scopes and absent paths contribute nothing. A present semantic element
must resolve. Type names are case-insensitive; scopes without an explicit type
default to class. The old `type=complete` editor sentinel is not a term.
`complete` flags do not suppress actual references.

Application value types, examples, override set-values, vocabulary concept URIs,
legacy subfield `scope`/`source` hints, instance IDs, class display codes and the
qualifier `sub_property_of` code are not independent ontology term references.
`ontology_path` is a derived display string. Do not scan arbitrary string values
or URLs. A future feature that gives these metadata fields semantic meaning must
extend the inventory and tests in the same change.

Matching preserves the exact published term URI. QNames may match a term's own
stored prefix/local name or a declared namespace alias in the captured project
or publication context. The collapsed effective namespace map cannot replace
that alias set. A preferred namespace may contain no actual terms. An explicit
URI and accompanying QName must agree; unknown terms, wrong kinds, ambiguous
owners/aliases and conflicting versions reject the entire cut. There is no
automatic rewrite of legacy literal spellings or external-reference allowlist.
The canonical Go matcher lives in `pkg/namespace`; a storage constraint checks
the same contract against published bytes.

Migration 030 records one immutable resolution document per project release,
including empty inventories. Each entry records entity kind/ID, a zero-based JSON
pointer, the original reference, exact resolved URI and ontology owner/version/
digest. Database checks enforce complete, unique coverage and exactly one matching
published candidate. Capture requires an unfinished repeatable-read release cut.
Updates, direct deletion, truncate and destructive downgrade are rejected; deleting
an unreferenced owning release cleans up its inventory normally.

Foreign entities retain their source release's inventory. A consumer captures the
source closure and requires the source's semantic inventory to exist; it does not
re-resolve source references with its own aliases. This applies transitively.

`ontologyrelease.Factory.ReadProject` now requires both the older pin inventory
and this semantic evidence. Its callback-bound reader exposes detached artifacts
and `References` values through generated archive-only SELECTs. Historical absence
is `not_in_release`; neither a legacy link inventory nor a current live field is
proof of historical semantic completeness. Migration 030 also adds archived
subfield storage, with no historical backfill.

## Consequences and remaining work

New cuts can fail on previously tolerated missing links or invalid terms. Repair
the draft and cut a new release, bottom-up for source projects. Older cuts remain
unverified; reconstructing them requires separate retained evidence, not this
migration. Existing tags are not rewritten.

The materializer and field archive readers now preserve subfield paths, including
adopted fields read at their source release. The whole-project assembler must
consume these captured identities and enforce complete consumer coverage. This
decision does not certify all project browsing, generators, or raw archive storage
as fully isolated yet.

Related: [namespace boundary](0004-namespace-resolution-boundary.md),
[source release reads](0011-cross-project-reads-resolve-at-the-source-release.md),
[assembler](0012-whole-project-reads-belong-to-an-assembler.md),
[ontology publication](0013-ontology-drafts-and-immutable-releases.md).
