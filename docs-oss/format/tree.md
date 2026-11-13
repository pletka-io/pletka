# The project tree

The tree is the set of files of one project at one git ref. The materializer
writes it (`pletka project init-git`, and on every save), `pletka project
load-git` reads it, and people may write it by hand. This file specifies the
layout, identity, every file kind, the forms an author may use, and the
canonical encoding.

Key-level detail is normative in [`schema/`](schema/); this file explains it.

## Layout

```
project.yaml                                      required  identity, parents, ontology links, namespaces
pletka.mod                                        required  module path and requirements
pletka.sum                                        when any requirement is resolved (see modules.md)
categories/<CAT>.yaml                                       one per category
concept-lists/<CL>.yaml                                     one per concept list
fields/<F>/field.yaml                                       one per field
fields/<F>/base-override.yaml                               optional: the field's own placement defaults
models/<M>/model.yaml
models/<M>/overrides/<F>@<token>.yaml                       one per field placed on the model
models/<M>/placements/<C>@<CAT|_>.yaml                      one per collection placed on the model
models/<M>/grafts.yaml                                      optional: overrides of grafted copies in the model
collections/<C>/collection.yaml
collections/<C>/overrides/<F>@<token>.yaml                  one per field placed on the collection
adoptions/index.yaml                                        list of adoption receipts
adoptions/<entity_type>-<entity_id>.yaml                    one receipt per adopted entity
forks/index.yaml, forks/<entity_type>-<entity_id>.yaml      the same for forks
releases/index.yaml                                         release catalog (see versions.md)
generators/**                                               optional: generator sidecars, opaque to core
vendor/projects/<module>@<selector>/...                     optional, see modules.md
vendor/ontologies/<module>@<version>/...                    optional, see modules.md
```

`<CAT>`, `<CL>`, `<F>`, `<M>`, `<C>` are semantic ids. Nothing else is part of
the tree. A file in an entity directory that matches no pattern above is an
error (`unknown-file`). Files outside the entity directories (`README.md`,
`.github/`, `LICENSE`) are ignored by the loader and MAY exist.

**[differs]** Today `vendor/` uses `vendor/projects/<module>[/<version>]` and
`vendor/ontologies/<module>/<version>`; see [`STATUS.md`](STATUS.md).
`releases/index.yaml` lives only on release commits today, not on `main`.

## Identity

**Semantic ids are the only identity in a tree.** File names and every
cross-reference use them; database ULIDs MUST NOT be required anywhere.

Grammar (`pkg/domain/semantic_id.go`):

```
semantic-id   = entity-id / grouping-id
entity-id     = project-id ( "F" / "M" / "C" ) "." number [ "_" system-name ]
grouping-id   = project-id "." ( "CAT" / "CL" ) "." number [ "_" system-name ]
project-id    = 1*( ALPHA / DIGIT )          ; upper-case by convention, e.g. X, LA
number        = "0" / %x31-39 *DIGIT         ; no leading zeros
```

Examples: `XF.1`, `XM.2`, `XC.3`, `X.CAT.4`, `X.CL.5`.

Rules:

- An own entity's semantic id MUST start with the tree's `project.id`.
- The file or directory name MUST equal the entity's `semantic_id` key.
- The type code MUST match the directory: `F` under `fields/`, `M` under
  `models/`, `C` under `collections/`, `CAT` under `categories/`, `CL` under
  `concept-lists/`. **[not built: not checked]**
- An entity copied from a parent carries `_adopted_from: <project-id>` and
  keeps the parent's semantic id; a receipt in `adoptions/` MUST record it.
- The `<token>` after `@` in an override file name only keeps names unique;
  the loader ignores it. The materializer derives it from the placement, so a
  name survives load-git: `<category>` (`_` for none), then `@<collection>`
  when the field sits in a collection, then `~<n>` (n ≥ 2) for the n-th exact
  duplicate of that placement, counted by `position` and then by file bytes:
  `XF.1@X.CAT.4.yaml`, `XF.2@X.CAT.4@XC.3.yaml`, `XF.2@X.CAT.4@XC.3~2.yaml`,
  `XF.5@_.yaml`. An author MAY use any string without `/`.

## File kinds

All YAML files are mappings. Keys not listed in the schema are errors.
**[differs]** The loader drops unknown keys silently; `verify-git` reports
them only at the top level of entity files and `project.yaml`.

### `project.yaml` — [schema](schema/project.schema.json)

```yaml
schema_version: 2
project:
  id: X                          # the project id, prefix of every semantic id
  system_name: example_project
  title: {en: Example project}
  description: {en: ...}
  readme: {en: ...}
  namespace: https://example.org/x/
  visibility: public
  owner: {type: organization, slug: example, display_name: Example}
  license: CC-BY-4.0
  base_url: https://example.org/x/
  topics: [archaeology]
inheritance:
  parents:
    - project_id: P
      is_primary: true
      canonical_order: 0
ontologies:
  linked_versions:
    - {prefix: crm, version: 7.1.3, is_primary: true}
namespaces:
  effective_bindings:
    - {prefix: crm, namespace: http://www.cidoc-crm.org/cidoc-crm/}
manifests:
  adoptions: adoptions/index.yaml
  forks: forks/index.yaml
```

- `inheritance.parents` names parent projects by id. **Which ref of a parent**
  is not stated here but in `pletka.mod` (see [`modules.md`](modules.md)).
  **[differs]** Today each parent also carries `source_mode`
  (`draft`/`release`) and `source_version`, and `pletka.mod` must agree with
  them. Under this spec those two keys are removed: `pletka.mod` is the one
  place that states a dependency's version.
- `ontologies.linked_versions` names linked ontology versions by `prefix` +
  `version`. The materializer also writes `ontology_id` and
  `ontology_version_id` (database ids); an author omits them.
  **[proposed]** The ids are dropped from the written form once ontologies
  resolve from the ontology repository.
- `namespaces.effective_bindings` is the prefix → namespace table the
  project's qnames are read with, and it decides them: on load, every prefix
  the instance would resolve to another namespace gets a binding the project
  owns. A prefix resolves with the project's own binding first, then its
  parents' (primary first, then canonical order), then the instance's global
  bindings, so a child inherits a prefix only its parent binds. The first
  binding of a prefix counts. Bindings are unique per project: two projects,
  or a project and the instance, may bind the same pair or the same prefix
  differently. Where the tree binds a prefix unlike a parent's tree does,
  verify-git reports a `namespace-diverges-from-parent` note (not a finding):
  the project's binding wins in its own output, also for fields adopted from
  that parent. `source` is deprecated and no longer written (it named where
  an instance got the row); the loader ignores it.
- `owner` / `created_by` are actor objects (`actor_id`, `type`, `slug`,
  `display_name`). They are informational; loading needs the owner to exist.

### `pletka.mod` and `pletka.sum`

Specified in [`modules.md`](modules.md).

### `categories/<CAT>.yaml` — [schema](schema/category.schema.json)

```yaml
semantic_id: X.CAT.4
system_name: production
ui_name: {en: Production}
description: {}
status: draft
deprecated: false
canonical_order: 4
```

### `concept-lists/<CL>.yaml` — [schema](schema/concept-list.schema.json)

```yaml
semantic_id: X.CL.5
system_name: object_types
ui_name: {en: Object types}
description: {}
status: draft
is_closed: false
```

The list's entries (concepts, broader edges) are **not** in the tree.
**[not built]** They move into the tree as `concept-lists/<CL>/entries.yaml`
when the database stops being their home; the layout then becomes a
directory per list.

### `fields/<F>/field.yaml` — [schema](schema/field.schema.json)

```yaml
semantic_id: XF.1
system_name: title
ui_name: {en: Title}
description: {en: The title of the object}
ontology_scope: crm:E22_Human-Made_Object
path_elements:
  - crm:P102_has_title
  - crm:E35_Title
  - crm:P190_has_symbolic_content
  - rdfs:Literal
expected_value_type: String
status: draft
deprecated: false
```

- `ontology_scope` is the class the path starts from; `path_elements`
  alternates property, class, property, …, ending on a class or a literal.
- `expected_value_type` names what a value of the field is. The terminal
  element MUST agree: a literal for literal kinds, a class for `Model`,
  `Collection` and `Concept`.
  Known values: `String`, `Date`, `Integer`, `URI`, `GeoJson`, `Model`,
  `Collection`, `Concept`. **[not built]** The schema lists them but does
  not enforce the set: existing data holds other spellings, which a cleanup
  normalises before the set becomes closed.
- `ontology_path` (`->crm:P102_has_title->crm:E35_Title->…`) is written by
  the materializer as a readable summary and ignored on load. Authors omit it.
- `subfield_paths` (optional) lists extra paths
  (`path_elements`, `expected_value_type`, `scope`, `source`) for compound
  values.

### `models/<M>/model.yaml`, `collections/<C>/collection.yaml` — [model](schema/model.schema.json), [collection](schema/collection.schema.json)

```yaml
semantic_id: XM.2
system_name: object
ui_name: {en: Object}
description: {}
ontology_scope: crm:E22_Human-Made_Object
status: draft
deprecated: false
```

A collection adds `collection_number` and `canonical_collection_order`.

### Overrides — [schema](schema/override.schema.json)

`fields/<F>/base-override.yaml` (a field's own defaults, `entity_type: ""`)
and `models/<M>/overrides/<F>@<token>.yaml`,
`collections/<C>/overrides/<F>@<token>.yaml` (a placement).

```yaml
field_id: XF.1
entity_type: model               # "" for a base override, model, collection
entity_id: XM.2                  # "" for a base override
position: 1
category_id: X.CAT.4
is_required: true
min_occurs: 1
max_occurs: null                 # null = unbounded
display_name: {en: Title}
description: {}
collection_name: {}
collection_order: 0
part_of_collection_id: ""        # the collection this placement sits in, if any
set_value: ""
is_hidden: false
visibility: ""
refs:
  - {ref_type: concept_list, semantic_id: X.CL.5, position: 1}
```

- `field_id`, `entity_type` and `entity_id` MUST match the file's location.
- `refs` names what a `Model`, `Collection` or `Concept` value points at:
  `ref_type` is `resource_model`, `collection_model` or `concept_list`.
  `refs` is sorted by (`ref_type`, `position`).
- Placements are authoritative: loading a tree replaces every placement of
  the project with the tree's.
- `visibility` known values: `""`, `Yes`, `No`. **[not built]** Existing data
  mixes case (`no`/`No`); normalisation precedes a closed set.

### Collection placements — [schema](schema/placement.schema.json)

`models/<M>/placements/<C>@<CAT>.yaml`, or `<C>@_.yaml` when the placement
has no category: how a collection placed on a model behaves as one card.

```yaml
model_id: XM.2
collection_id: XC.3
category_id: X.CAT.4             # "" when the file name says _
is_required: false
min_occurs: 0
max_occurs: null                 # null = unbounded
is_hidden: false
```

- `model_id`, `collection_id` and `category_id` MUST match the file's
  location.
- The model MUST place at least one field of the collection (an override
  with `part_of_collection_id: <C>`).
- `min_occurs` ≥ 0 and `max_occurs` ≥ `min_occurs`, both within a 32-bit
  integer.
- Placements are authoritative, like overrides.


### Graft overrides — [schema](schema/grafts.schema.json)

`models/<M>/grafts.yaml`: per-graft overrides of the model. Keys are
addresses, the field semantic ids from the model down through the stubs,
joined by `/` (`LAF.177/LAF.100/LAF.12`); an address has at least two
segments. Each value may set `is_hidden` (drop the grafted copy and
everything under it), `max_occurs` (`null` = unbounded) and `display_name`
(a translations map). An override changes one grafted copy only, never the
collection the copy comes from.

```yaml
LAF.177/LAF.100:
  max_occurs: 1
  display_name: {en: Name for Time Span}
LAF.177/LAF.100/LAF.31:
  is_hidden: true
```

- The model directory MUST have a `model.yaml`.
- Every segment MUST name a known field (verify-git
  `unresolved-graft-address`). An address the composed model never reaches
  is a compose warning (`compose.unresolved_graft`).
- Loading a tree replaces every graft override of the project.

### Generator files

`generators/**`: sidecar files a generator reads (for example
`generators/arches/<model>/aliases.yaml`). Core stores each file as bytes
with its sha256, writes it back unchanged and does not parse it. Loading a
tree replaces every generator file of the project.
### Receipts — [index](schema/receipts-index.schema.json), [adoption](schema/adoption-receipt.schema.json), [fork](schema/fork-receipt.schema.json)

`adoptions/index.yaml` lists every adoption receipt; each receipt records which
entity was taken from which source and where it is used:

```yaml
schema_version: 1
adoption:
  entity_type: field
  entity_id: PF.7
  source: {project_id: P, entity_id: PF.7, version: 0.1.0}
  contexts:
    - {context_entity_type: model, context_entity_id: XM.2}
```

Forks are the same shape under `fork:` with `forked_at`/`created_by` instead
of `contexts`. **[differs]** `source.version` pins a release independently
of `pletka.mod`; under this spec the source's version is whatever
`pletka.mod` resolves for that project, and `source.version` is dropped.
Moving a child to a newer parent release is then one `pletka.mod` change.

## Authoring forms

A hand-written tree MAY use these forms; the loader accepts them and the
materializer writes the long form back.

1. **Omitted keys.** Every key except `semantic_id` (entities) and the
   location keys (overrides) has a default: empty string, `{}`, `[]`,
   `false`, `0`, or `null` for `max_occurs`.
2. **Qname strings for paths.** `ontology_scope` and each `path_elements`
   item MAY be a qname string instead of a path-element map. Types follow
   from position: in `path_elements` even positions are properties, odd
   positions classes; `xsd:*`, `rdf:langString`, `rdfs:Literal` (and the
   alias `rdf:literal`) are literals. `A/B` makes `B` an additional type of
   `A`. Prefixes expand through `namespaces.effective_bindings`, then the
   linked ontologies' prefixes, then the standard set.
3. **Ontology links by prefix and version** (no ids).

### Path-element map — [schema](schema/common.schema.json#/$defs/pathElement)

The materializer writes each element as a map so a round trip is lossless:

| Key | Meaning |
|---|---|
| `type` | `class`, `property` or `literal` |
| `uri` | the element as a qname, e.g. `crm:P102_has_title` **[proposed: full IRI]** |
| `prefix`, `local_name` | the two halves of the qname |
| `position` | 0-based index in the path |
| `datatype` | literals only, e.g. `xsd:date` |
| `class_code` | short class code, e.g. `E22` |
| `additional_types` | secondary classes of a multi-typed node |
| `sub_property_of` | for CRM `.1` properties, the parent property code |
| `path_node_id`, `instance_id` | generated / legacy node ids |
| `complete` | editor flag; ignored on load |
| `ontology_version_id` | database id; ignored on load **[proposed: drop]** |

## Canonical encoding

The materializer writes every YAML file in one byte form; the same bytes are
hashed for change detection. A conforming writer MUST produce:

1. One YAML document, no `---`, UTF-8.
2. Map keys sorted by byte order at every level.
3. Sequence order preserved as written (path order is meaningful). Slices
   with no inherent order are pre-sorted by the writer: `refs` by
   (`ref_type`, `position`); receipt indexes by (`entity_type`, `entity_id`).
4. Indent 2; block style for collections; scalars in the yaml.v3 default
   style (strings that would read as another type are double-quoted:
   `"2.0"`, `""`).
5. LF line endings, no trailing whitespace, exactly one trailing newline.

Readers MUST accept any valid YAML; canonical form is a property of written
files, not a requirement on authored ones. `verify-git --canonical`
**[not built]** reports files that are not in canonical form.

## Schema version

Every manifest (`project.yaml`, `pletka.mod`, receipts, indexes,
`ontology.yaml`, `releases/index.yaml`) carries `schema_version`.

- `1` — written by the materializer through 2026-11.
- `2` — this specification: strict keys, mod-only dependency versions,
  sum-only hashes, `@`-style vendor layout.

A reader MUST reject a `schema_version` it does not know.
Today nothing reads it.
