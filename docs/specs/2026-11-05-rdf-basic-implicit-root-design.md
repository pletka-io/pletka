# RDF basic vocabulary as the implicit root of every project

Date: 2026-11-05
Status: approved design, not yet implemented

## Problem

Every ontology Pletka loads is RDF, so every class is an `rdfs:Resource` and
every datatype is an `rdfs:Literal`. Today only the path audit acts on that:
it resolves terms against the project's linked ontology versions *and* the
implicit `defaults` ontology. The path builder, term descriptions and the MCP
autocomplete build their index from project links alone, so `rdfs:label`,
`rdfs:comment`, `rdfs:seeAlso` and `owl:sameAs` are never offered unless an
application ontology re-declares them with an explicit domain. Each bridging
ontology ends up carrying the same boilerplate, and the three readers
disagree about what a project can see.

## Decision

1. The `defaults` ontology (prefix `defaults`, the curated W3C subset) is the
   RDF basic vocabulary. Its active version is part of every project's
   autocomplete index, exactly as it is already part of every path audit.
2. `rdfs:Resource` is the implicit root class. Every class that declares no
   superclass is treated as a direct subclass of `rdfs:Resource`, so a
   property whose domain is `rdfs:Resource` is offered on every class.
3. A property in `defaults` opts in to "offered on every class" by declaring
   `rdfs:domain rdfs:Resource`. Domain-less properties stay audit-only. No
   new flag, no namespace exception.
4. A range of `rdfs:Resource` terminates a path ("an IRI"), the same way
   `rdfs:Literal` terminates it ("a literal"). It never expands to the
   subclass closure of the root.

## Defaults file 1.1

Same file shape, version bumped to 1.1.

| term | change |
|---|---|
| `rdfs:label` | add `rdfs:domain rdfs:Resource`; range stays `rdfs:Literal` |
| `rdfs:comment` | new; domain `rdfs:Resource`, range `rdfs:Literal` |
| `rdfs:seeAlso` | new; domain `rdfs:Resource`, range `rdfs:Resource` |
| `owl:sameAs` | add domain `rdfs:Resource` and range `rdfs:Resource` |
| `rdf:type`, `rdf:value`, `dc:format`, `dcterms:conformsTo`, `skos:*` | unchanged, domain-less, audit only |
| classes and datatypes | unchanged; `rdfs:Literal subClassOf rdfs:Resource` already declared |

Future additions (skos basics, `dc:title`, …) follow the same rule: a row with
domain `rdfs:Resource` is offered everywhere, a row without is audit-only.

## Index changes (`pkg/weave/ontology/autocomplete`)

1. **Resolver.** `IndexCache.resolve` appends the `defaults` active version
   ID to the version set when the project does not already link it. The
   lookup (`WeaveGetOntologyByPrefix` + `WeaveGetActiveOntologyVersion`,
   prefix constant `defaults`) moves out of `pkg/weave/pathaudit` into
   `pkg/weave/ontology` as one shared helper; `pathaudit` and
   `gitmaterializer` call it instead of keeping their own copies. The
   version ID lands in the cache key, so a defaults re-import invalidates
   every project index through the existing `InvalidateForVersion` path.
2. **Implicit root.** After pass 2 of `buildIndex`, if a non-ghost class
   node `rdfs:Resource` exists, every other non-ghost class node with no
   `Superclasses` gets `Superclasses = [Resource]` and
   `Resource.Subclasses` gets the node. The existing superclass `closure`
   in `suggestPropertiesForClass` then carries `Resource.AsDomainOf` to
   every class. No engine change for property suggestions. Because the
   root is the deepest ancestor, inherited basics sort after the class's
   own properties with no ranking code.
3. **Terminal root range.** `suggestRangeForProperty` keeps the
   `isResourceRange` rule and applies it before the subclass walk, so it
   holds for a real `rdfs:Resource` node and not only for a ghost. A range
   of `rdfs:Resource` therefore yields one terminal suggestion.
4. **Datatypes.** No change. `xsd:*` path ends stay engine-synthesised;
   the datatype rows in `defaults` keep serving the audit. Driving the
   literal list from those rows is a later change.

When no `defaults` ontology is imported, nothing changes: no root node, no
extra version, index identical to today.

## Consistency

| reader | after this change |
|---|---|
| path builder (indexed engine) | links ∪ defaults active, implicit root |
| term description / hover | same index, resolves the basics for free |
| path audit | unchanged, already links ∪ defaults |
| MCP autocomplete | same engine |
| MCP term listing, project ontology settings, release link snapshot | unchanged: defaults stays implicit, never shown as a project link. Suggestions carry `prefix: defaults` and its version as the ontology badge |
| release lock | live-only today; implicit version resolved at build like any other |
| git vendor snapshot | already vendors the defaults active version |

The direct engine (super-admin debug only) is out of scope.

## Tests

Table-driven in `pkg/weave/ontology/autocomplete`:

1. Defaults version plus one class with no superclass: `rdfs:label`,
   `rdfs:comment`, `rdfs:seeAlso`, `owl:sameAs` offered after the class's
   own properties.
2. Class with an explicit superclass chain still gets them, once.
3. A domain-less defaults property (`rdf:type`) is not offered.
4. `owl:sameAs` suggests a single terminal `rdfs:Resource`, not every class.
5. No defaults imported: index identical to today.
6. Resolver: a project that links defaults explicitly gets no duplicate
   version ID.

Deployment side (out of this repo): vendored import of defaults 1.1, probe
shows 12 classes / 10 properties, then re-run the field audit.

## Consequence for extension ontologies

A bridging ontology no longer needs to re-declare `rdfs:label`,
`rdfs:comment`, `rdfs:seeAlso` or `owl:sameAs` with a domain. Drafts that do
so should drop those rows before publication.

## Considered and rejected: link defaults to every project

Treating defaults as an ordinary per-project link was weighed. It still needs
the implicit root (no ontology declares `subClassOf rdfs:Resource`), and it
adds a project-create hook, a backfill of every existing project, release
link snapshots, a dedupe across parent and child links, and a relink of every
project on each defaults upgrade because links pin a version ID. Opting out
would produce a project whose audit accepts `rdfs:label` while its builder
cannot reach it. Implicit resolution keeps one upgrade path (the active
version) and one answer across all readers.

## Follow-up (not in this change)

Show the defaults ontology as a read-only "built-in" row in the project
ontology settings list so a curator can see where the basics come from. UI
only; it creates no link.
