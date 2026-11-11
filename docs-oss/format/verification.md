# Verification

`pletka project verify-git <dir>` checks a tree with no database. Every
problem is a finding `file:line: code: message`; any finding exits non-zero;
`--format json` is for CI. `load-git` refuses what verify-git would report
under *load*; the other classes are checks the loader does not make.

Inputs: the tree; dependencies from `vendor/`, else `--parent <dir>`
(repeatable) and `--ontologies <dir>`; **[not built]** else the module
cache. `--structure-only` skips the ontology class.

## Check classes

Checks run in this order; a *load* finding stops the run.

### 1. Load

The tree can be read at all: `project.yaml` and `pletka.mod` parse;
`pletka.sum` lines parse; receipt indexes and receipts parse and agree;
vendored manifests parse; `project.yaml` parents and `pletka.mod` project
requirements agree. Code: `load`.

### 2. Syntax — against [`schema/`](schema/)

| Code | Finding |
|---|---|
| `decode` | a file is not valid YAML, or a value has the wrong type |
| `unknown-key` | a key the schema does not define, at any depth |
| `schema-version` | `schema_version` missing or not one this binary knows (1, 2) |
| `schema` | any other schema rule: a required key missing, a pattern (e.g. a semantic id), an enum |

Every format file, including vendored modules (validated from their own
root), is checked against its JSON Schema before anything else; `load-git`
refuses the same files with the file and line in the error.

### 3. Structure

| Code | Finding |
|---|---|
| `unknown-file` | a file in an entity directory matches no layout pattern |
| `semantic-id` | missing, malformed, wrong project prefix, or not the file name |
| `semantic-id-type` | type code does not match the directory **[not built]** |
| `override-location` | an override's `field_id`/`entity_type`/`entity_id` disagree with its path |
| `orphan-overrides` | `overrides/` with no `model.yaml`/`collection.yaml` beside it |
| `placement-path` | a placement's `model_id`/`collection_id`/`category_id` disagree with its path |
| `placement-orphan` | a placement for a collection the model places no field of |
| `placement-range` | `min_occurs`/`max_occurs` do not form a range within 0..2³¹−1 |
| `duplicate-placement` | the same field placed twice on one owner **[not built]** |

### 4. References

| Code | Finding |
|---|---|
| `unresolved-field` | an override's field is in neither the tree, a receipt, nor a parent |
| `unresolved-category` | `category_id` does not resolve |
| `unresolved-collection` | `part_of_collection_id` does not resolve |
| `placement-category` | a placement's `category_id` does not resolve |
| `unresolved-concept-list` | a `concept_list` ref does not resolve |
| `unresolved-model-ref` | a `resource_model`/`collection_model` ref does not resolve **[not built]** |
| `adoption-receipt-missing` | an `_adopted_from` entity without a receipt |
| `adoption-unknown-parent` | a receipt's source project is not a parent |
| `adoption-missing-source` | a receipt's source entity is not in the parent |
| `parent-not-declared` | a `--parent` tree that `project.yaml` does not name |
| `parent-tree-missing` | a declared parent that is neither vendored nor given |
| `mod-mismatch` | `pletka.mod` requirements and `project.yaml` parents/ontologies differ **[not built for ontologies]** |

Notes are reported (`note:` in text output, `notes` in JSON) but do not fail
the tree:

| Code | Note |
|---|---|
| `namespace-diverges-from-parent` | `project.yaml` binds a prefix to another namespace than a parent's tree does; the project's binding wins in its own output |

### 5. Integrity

| Code | Finding |
|---|---|
| `vendor-checksum` | a vendored module's tree hash differs from `pletka.sum` |
| `sum-missing` | a closure module has no `pletka.sum` line **[not built]** |
| `sum-extra` | a `pletka.sum` line for a module outside the closure **[not built]** |
| `vendor-missing` | `vendor/` exists but lacks a closure module **[not built]** |
| `version-conflict` | two selectors for one module in the closure **[not built]** |

**[differs]** Today one `vendor-checksum` finding stops the integrity check
at the first mismatch.

### 6. Ontology (semantics)

The autocomplete index is built in memory from the closure's ontologies
(same code as the server: `MemoryStore`, `OfflineEngine`), and every own
field's path is replayed step by step as the path builder asks
(`ReplayPath`): the scope must be offered at the root, each step must be
offered after the steps before it. This covers domain, range and subclass
reasoning in one rule set shared with the server.

| Code | Finding |
|---|---|
| `ontology-none` | the tree links no ontology |
| `ontology-missing` | a linked ontology version is not available |
| `ontology-skipped` | path checks skipped because an ontology is missing |
| `ontology-load` | an ontology's RDF does not parse, or `ontology.yaml` has no primary source |
| `unbound-prefix` | a qname whose prefix no binding defines |
| `scope-missing`, `path-missing` | a field without scope or path |
| `path-incomplete` | the path ends on a property |
| `terminal-mismatch` | the terminal element disagrees with `expected_value_type` |
| `not-offered` | a step the path builder would not offer |
| `model-scope-not-offered` | a model/collection scope that is not a known class **[not built]** |
| `subfield-not-offered` | a `subfield_paths` step not offered **[not built]** |

Live counterpart: `pletka weave replay-paths --project <id>` asks the
database-backed engine the same questions; the two agree by construction.

### 7. Canonical form **[not built]**

`--canonical`: re-encode each file and report files whose bytes differ, and
`ontology_path` values that disagree with `path_elements`.

## Not checked, by design

- Values of `status`, `visibility`, `expected_value_type` beyond the known
  sets: listed in the schemas, reported only once data is normalised.
- Cardinality coherence (`min_occurs` ≤ `max_occurs`, `is_required` vs
  `min_occurs`) **[not built]**.
- Whether owner/creator actors exist: an instance concern, checked on load.

## CI

A repository holding a tree runs verify-git on every push with the core
binary at a fixed commit:

```yaml
- run: go run github.com/pletka-io/pletka@<commit> project verify-git . --format text
```

The workflow file must live under `.github/workflows/`.
