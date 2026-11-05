# The project tree

The git materializer (`pkg/service/gitmaterializer`) writes every project as
a tree of canonical YAML files, and `pletka project load-git <dir>` turns such
a tree back into a database project. The tree is therefore both an export
format and an authoring format: a project can be written by hand, kept in
git, reviewed as a diff, and loaded.

## Layout

```
project.yaml                 identity, parents, ontology links, namespaces
pletka.mod                   module path and requirements
pletka.sum                   tree hashes, only when vendor/ exists
categories/<ID>.yaml
concept-lists/<ID>.yaml      the list itself; entries stay in the database
fields/<ID>/field.yaml       + base-override.yaml
models/<ID>/model.yaml       + overrides/<FIELD>@<token>.yaml, one per placement
collections/<ID>/collection.yaml + overrides/...
adoptions/, forks/           receipts for entities taken from other projects
vendor/ontologies/, vendor/projects/   optional, for self-contained trees
```

`<ID>` is the semantic id (`XF.1`, `XM.2`, `X.CAT.3`, `X.CL.4`). The token
after `@` in a placement file name only keeps names unique; the materializer
writes the override row id there and restore ignores it.

## What a hand-written file may use

- **Paths as qname strings.** `ontology_scope: crm:E21_Person` and
  `path_elements: [crm:P1_is_identified_by, crm:E41_Appellation]` decode the
  same as the full path-element maps the materializer writes.
- **Ontology links by prefix.** A `project.yaml` `linked_versions` entry may
  give `prefix` + `version` instead of ids; restore resolves them against the
  database. The materializer writes prefix, ids and version together.
- **Semantic ids only.** Files reference each other by semantic id; ULIDs
  never need to appear.
- **Concept lists.** `concept-lists/<ID>.yaml` declares a list; a field binds
  to it through `refs: [{ref_type: concept_list, semantic_id: <ID>}]` on an
  override. A ref to a list that is neither in the tree nor in the database
  fails the load.
- Every key the materializer writes has a default and may be omitted.

## Load semantics

- Entities upsert by semantic id; nothing is deleted. Placements are cleared
  and rewritten per load, so the tree is authoritative for them.
- Phases commit separately (vendored ontologies, project shell, entities,
  overrides and provenance). After a failure, fix the file and load again.
- A loaded project is a draft. Releases are cut from the database.
