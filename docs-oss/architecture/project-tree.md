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
concept-lists/<ID>.yaml      the list itself (no deprecated flag); entries stay in the database
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

## Verifying a tree without a database

`pletka project verify-git <dir>` reads the tree and reports, as
`file:line: code: message`, what `load-git` would reject and what the path
builder could not rebuild. It needs no database: the autocomplete index is
built in memory from the tree's `vendor/ontologies/` (checked against
`pletka.sum`) or from `--ontologies <dir>` in the same layout, and every
field path is replayed step by step through that index exactly as the
builder asks its questions. Parents come from `--parent <dir>` (repeatable)
or `vendor/projects/`. A self-contained tree (`init-git --self-contained`)
verifies on its own; `--structure-only` skips the ontology part; `--format
json` is for CI. Any finding makes the command exit non-zero.

Finding codes: `load`, `unknown-file`, `decode`, `unknown-key`,
`semantic-id`, `unresolved-field`, `unresolved-category`,
`unresolved-collection`, `unresolved-concept-list`, `orphan-overrides`,
`adoption-unknown-parent`, `adoption-missing-source`, `parent-tree-missing`,
`parent-not-declared`, `vendor-checksum`, `ontology-missing`,
`ontology-skipped`, `ontology-load`, `unbound-prefix`, `path-incomplete`,
`terminal-mismatch`, `not-offered`. An unknown top-level key is a finding
because the loader drops it silently. The ontology part uses the same index
build and the same stepping as the server (`pkg/weave/ontology/autocomplete`
`MemoryStore`, `OfflineEngine`, `ReplayPath`); `pletka weave replay-paths`
asks the live database the same questions, so the two agree.
