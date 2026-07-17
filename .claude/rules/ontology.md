# Ontology — Design Contract

Apply CIDOC-CRM / RDFS / OWL / SHACL best practices for all ontology work.
Read `docs-oss/architecture/ontology-autocomplete.md` for how CRM paths are
constrained and audited, and `docs-oss/reference/cidoc-crm-reference.md` when
working with CIDOC-CRM classes, properties, or paths.

## Core Principle: Semantic Correctness Over Schema Conformance

Ontological correctness is determined by CRM scope notes, not just schema validation. A triple that passes RDFS/OWL/SHACL checks but contradicts the scope note of a class or property is incorrect. Always verify against scope notes when assigning CRM classes to Models or CRM properties to Fields.

## Pletka Mapping

| Ontology Concept | Pletka Entity |
|---|---|
| Ontology property/edge | Field (has ontology path, expected value type) |
| Scope class (CRM class) | Model (entity definition = scope class + fields) |
| Grouped properties sharing root context | Collection (e.g., "Birth Event" = E67_Birth) |
| Application Profile | Project (selects, constrains, documents vocabulary usage) |

## Constraints

- Never mint a new IRI when a suitable term exists in CIDOC-CRM, SKOS, Dublin Core, or Schema.org
- Never use `owl:sameAs` for vocabulary alignment (use `skos:exactMatch`)
- Never model controlled vocabulary terms as OWL classes (use `skos:Concept`)
- Always use the event-centric pattern when modelling change over time
- There is no save-time alternation validator on `Field.PathElements`. Correctness
  comes from two other points instead: the autocomplete engine
  (`pkg/weave/ontology/autocomplete/`) only ever offers properties whose domain
  matches the current class lineage and classes that are the property's range (or
  a subclass of it), so a path built through the `RichPathBuilder`/
  `SimplePathBuilder` Svelte widgets cannot go wrong at build time; `pletka weave
  verify-paths` is the after-the-fact audit that walks every stored path and
  flags `malformed`/`type-mismatch`/`missing`/`ambiguous` rows against the
  project's linked ontology versions. "PathBuilder" names the Svelte widget
  family, not a validator.
- When reviewing CRM paths, check domain/range constraints against the class hierarchy
