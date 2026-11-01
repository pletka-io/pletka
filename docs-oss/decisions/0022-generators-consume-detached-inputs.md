# ADR-0022: Generators consume detached versioned inputs

Status: Accepted (2026-11-01)

## Context

Generators previously combined six independently loaded readers with optional
capabilities for concept lists, collection discovery and ontology packaging.
Some readers selected draft explicitly, some errors removed constraints, and
bulk output could combine different database snapshots. Putting a scope argument
on each function would still leave every renderer responsible for read discipline.

## Decision

`generators.Service` receives one `Source` and a renderer registry. The production
source is `service/generatorread`: it binds an explicit `auth.ReadScope` to a
`readsession.Factory`, invokes the bounded `projectread` assembler and closes the
session before returning a validated `projectsnapshot.EntityFragment` or
`CompleteProject`. Request policy authorizes the root; the configured factory
authorizes source projects. A selection does not grant access.

`generators.BuildSnapshot` is pure. It composes a fragment in its consumer context
and produces renderer input with explicit consumer/source identity, retained
metadata, exact semantic terms, source provenance, empty placements, fixed values,
and closed-list constraints. An empty closed list remains closed. Semantic aliases
must expand to the retained URI, including when source and consumer prefixes differ.
Unresolved required legacy semantics fail; current ontology policy cannot repair them.

Optional collection composition uses retained placements or explicit collection
references. It cannot discover an additional dependency through a global anchor
index. Arches packs follow retained model/collection references and project exact
ontology bodies and dependency relations. X3ML uses the same retained ontology
bundle as its model. Conflicting archive filenames are errors.

Bulk operations call `BindProject` once and resolve identifiers, enumerate entities
and render from that complete value. Entity requests use bounded assembly directly.
Rendering receives a defensive copy with no database or loading capability, after
input errors have been checked. An import conformance test prevents the core
projection and renderers from importing services, database clients or peer slices.
HTTP entity CSV and Arches packs finish rendering before publishing output.

`EncodeInput` defines schema version 1 of the JSON renderer input. It is the natural
data boundary for a future Lua adapter: tables containing values, identities,
paths, options and constraints, without callbacks into Go readers. This step adds
no interpreter, script deployment, permissions, execution quotas or sandbox. Those
remain an explicit future design. Go renderers remain registered through the
existing host injection seam.

## What we considered and rejected

- Keep six readers and require each caller to pass the scope correctly: leaves
  fallback and independent transaction choices scattered across every export.
- Let renderers request missing content: makes reproducibility depend on hidden
  calls, requires a much larger script API, and keeps connections alive during output.
- Preserve global anchor discovery: the discovered dependency is not pinned by
  the selected release. Store an explicit reference and release it instead.
- Treat renderer JSON as the restore format: it is a derived view. Lossless
  schema3 project transport remains a separate complete-value contract.

## Consequences and rollout limits

Existing Go renderers use the same `Snapshot` shape, extended with schema/selection,
retained bundles and closed-enum state. Low-level tree builders remain available
for renderer unit fixtures; production assembly enters through `BuildSnapshot`.
Defensive copying and complete bulk capture cost memory; large-project latency,
projection reuse and output budgets still need measurement before broad rollout.

The service, entity CSV, visualization/X3ML, integration artifacts, generator CLI
and Arches packs use the boundary. The whole-project CSV/ZIP service now shares
the complete-project source (amendment below). Standalone current ontology export
and browser inventory still need migration.
Project-qualified generator routes now support released roots absent from live
tables. Legacy owner-relative aliases still require live owner discovery. Browser
details use a separate [raw bounded projection](../reference/browser-detail-reads.md);
[project summaries](../reference/project-summary-reads.md) also use bounded
detached inputs. Paginated inventories and adoption closure remain separate work.
This decision does not enable capture-aware mutations on normal HTTP/MCP traffic.

The [HTTP route contract](../reference/generator-routes.md) describes explicit
consumer routing, current authorization and compatibility aliases.

See [whole-project assembly](0012-whole-project-reads-belong-to-an-assembler.md),
[source release selection](0011-cross-project-reads-resolve-at-the-source-release.md)
and [canonical node identity](0002-canonical-node-identity.md).

## Amendment: whole-project CSV (2026-11-01)

The separate `project/csvexport` verification pipeline now receives the same
complete-project source. All eight files, ZIP entries and selected names derive
from one value, while current membership gates access. Serialization never calls
slice stores or current ontology readers, and responses publish only prepared
files. The format preserves owned raw rows and authored paths, not generator
composition. Historical active ontology policy remains unknown when it was not
captured. See the [CSV contract](../reference/project-csv-export.md).
