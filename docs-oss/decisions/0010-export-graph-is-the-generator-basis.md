# ADR-0010: The export graph is the basis for every generator

Date: 2026-11-16  
Status: Accepted

---

## Why this decision was needed

Each generator that needs the model's structure (mermaid, cytoscape, a SHACL
renderer maintained outside this repository, SPARQL, and the rest) built its
own tree from the raw `FieldNode.Path`s. The copies diverged: one kept date
datatypes as classes, others disagreed on how step counts, required
propagation and branch identity work. Every fix had to be repeated per
renderer, and the outputs for one model could contradict each other.

## What we decided

Every generator reads the merged path graph returned by `exportgraph.Build`
(and `exportgraph.BuildMembers` for referenced collections), never the raw
paths. The graph is renderer-neutral and JSON-serialisable; its JSON (the
`export-graph` format) is the contract for script generators that live
outside core. Nothing renderer-specific (shape names, `sh:` lines, query
text) goes into the package.

Node identity is the path-builder node id (`PathNodeID`), falling back to
parent + property + class when a path carries no ids.

The graph carries:

- **Classes and extra types:** the node class and `AdditionalTypes`.
- **Datatype literals:** a terminal datatype becomes a `LiteralBinding` with
  the datatype as `End`, not a class node. A field whose path ends on a
  property still gets a binding, with a zero `End`.
- **Step counts and endings:** `ResourceEdge.Min/Max`, the `Endings` (each
  with its own counts and, in memory, its field), `Entry` for the step into
  a placed collection, `LiteralBinding.Min/Max`, `EffectiveMin`, and
  required propagation to the steps above a required field.
- **Collection anchors and addresses:** `ResourceNode.Collection` with the
  collection's semantic id, label and address.
- **Member graphs:** `BuildMembers` builds the graph of a referenced
  collection as it sits under a stub.
- **Node identity:** `NodeIdentity` (class, own set value, the fixed values
  of required one-step set-value fields), `Distinguishable`, and
  `Identity.WithLineage`, which let a renderer tell sibling branches apart.
  The shared RDF term rules live in `pkg/weave/generators/rdfterm`.

Migration status:

| Generator | Status |
| --- | --- |
| exportgraph, mermaid, cytoscape | on the graph |
| SHACL (renderer outside core) | this phase |
| SPARQL | this phase |
| modelcheck, rdf, x3ml, researchspace, csv, arches (outside core) | later, one PR each, each gated by its existing goldens |

## What we considered and rejected

- **A per-renderer tree:** rejected because the three copies already
  diverged (one kept dates as classes). It would be less coupling per
  renderer, but the same rule is then fixed several times, and wrongly in
  some.
- **Putting the graph on `Snapshot` itself:** rejected because `exportgraph`
  imports `generators`, so the field would make an import cycle. `Build(snap)`
  is cheap, so a renderer calls it where it needs the graph.

## Consequences

- **Easier:** a structural rule (counts, identity, datatype handling) is
  fixed once and every renderer follows. Script generators outside this
  repository read one documented JSON shape.
- **Harder:** the graph's shape is now a contract. A change to its JSON keys
  affects renderers outside core, so new fields carry `json` tags and
  removals need care.
- **Constrained:** renderer-specific logic stays out of `exportgraph`.
  Each remaining generator moves in its own PR and must keep its existing
  golden output unchanged.

## References

- Related ADRs: ADR-0004 (namespace resolution boundary)
