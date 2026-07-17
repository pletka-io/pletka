# ADR-0004: Namespace Resolution Boundary

Date: 2026-06-28  
Status: Accepted

---

## Why this decision was needed

Ontology import, RDF parsing, path element construction, and generators all
depend on stable prefix/base-URI resolution. Earlier code mixed static prefix
lists, local parser heuristics, and namespace manager lookups. That made
namespace surprises hard to diagnose: a missing prefix could silently become an
empty path element field, or a downstream generator could fail far away from the
RDF file that introduced the namespace.

## What we decided

`pkg/namespace` is the canonical namespace resolution package. RDF and ontology
readers must consume a namespace manager, enrich it with namespace declarations
found in RDF input, and fail explicitly when a URI, base URI, or prefix cannot
be resolved. Static namespace lists are seed data only; runtime import,
autocomplete, validation, and generator code should use the manager or the
stored namespace bindings derived from it.

## Design Rules

- `pkg/namespace` owns prefix/base-URI lookup and URI splitting.
- Standard namespaces are seed data. They are not a substitute for project,
  ontology, imported RDF, or user-managed namespace bindings.
- RDF/ontology readers receive a namespace manager instead of constructing
  private prefix maps.
- RDF/ontology readers parse namespace declarations from RDF input and add
  missing bindings to the manager when the binding is valid.
- If RDF declares a prefix/base pair that conflicts with an existing binding,
  the reader must report an explicit conflict instead of guessing.
- If a path element, ontology term, or generator target needs a prefix and no
  prefix can be resolved for the base URI, fail with a validation error at the
  import/read boundary.
- Do not emit persisted `PathElement` values with empty `Prefix`,
  `LocalName`, or unresolved `URI` when the source term was intended to be an
  ontology class/property.
- Prefer explicit validation errors over runtime repair heuristics. Heuristics
  that normalize or guess namespaces hide upstream data issues and produce
  confusing downstream failures.
- Generator snapshots and renderers consume effective namespace state. They do
  not reconstruct prefixes from compact strings or private static lists.
- Backfill/import compatibility paths may handle old data, but they must record
  or report that compatibility mode was used.

## Operational Rule

The normal ontology/RDF read path is:

1. Start with a namespace manager seeded from standard namespaces and persisted
   project/ontology namespace bindings.
2. Parse RDF namespace declarations.
3. Add missing declarations to the manager, subject to conflict validation.
4. Resolve every imported term through the manager.
5. Return typed path/ontology data only when prefix, local name, and URI are
   all resolved.
6. Surface missing/conflicting namespaces as validation errors before data is
   saved or handed to generators.

## What we considered and rejected

- **Static namespace lists in each consumer:** rejected because lists drift and
  cannot account for custom RDF namespaces, ontology aliases, or project-local
  bindings.
- **Runtime best-effort prefix guessing:** rejected because downstream path
  elements and generators depend on non-empty, deterministic prefix/base
  resolution. Guessing hides the data that needs fixing.
- **Letting RDF readers ignore undeclared namespaces:** rejected because the
  error then appears later in autocomplete, path validation, RDF/SHACL/SPARQL
  generation, or X3ML export.
- **Treating standard namespaces as authoritative runtime state:** rejected
  because they are only defaults. The effective namespace state is composed
  from standard bindings, project bindings, ontology bindings, RDF declarations,
  and imported aliases.

## Consequences

- **Easier:** namespace failures are reported near the import/read boundary
  where the user can fix the RDF or add a binding.
- **Easier:** path elements, ontology terms, and generator snapshots share one
  resolution model.
- **Easier:** custom namespaces from RDF files become available to downstream
  usage instead of disappearing after parse time.
- **Harder:** imports that previously relied on implicit guesses may fail until
  the missing namespace binding is supplied.
- **Harder:** readers must accept and update a namespace manager rather than
  operating as isolated parsers.
- **Constrained:** future namespace code should extend `pkg/namespace` or the
  namespace binding stores, not add new static lookup tables in consumers.

## References

- Related ADRs: [ADR-0003](0003-explicit-errors-over-heuristics.md) (explicit
  context over runtime heuristics — the same fail-loud principle applied here
  to namespace resolution).
