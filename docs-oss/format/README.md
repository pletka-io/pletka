# The Pletka project format

A Pletka project is a git repository of canonical YAML files. This folder is
the specification of that repository: what the files are, what they may
contain, how projects depend on each other and on ontologies, how versions
are named and pinned, and how a tree is checked.

It is written to be read on its own — by people and by agents reasoning about
a tree — without the Go code at hand.

## The model in five sentences

1. **One project, one repository.** The working tree holds the project's
   categories, concept lists, fields, models, collections and their
   placements as one file per entity ([`tree.md`](tree.md)).
2. **Git refs are the versions.** The draft is the branch `main`; a release
   is an annotated tag `v<version>`; an old release is edited on a branch cut
   from its tag ([`versions.md`](versions.md)).
3. **`pletka.mod` states intent, `pletka.sum` records fact.** The mod file
   names the parent projects and ontologies a project builds on and which
   ref of each it wants; the sum file records the commit and content hash
   each ref resolved to ([`modules.md`](modules.md)).
4. **Dependencies are fetched by module path.** A module path is a repository
   location (`github.com/<org>/<repo>`); resolved trees are kept in a local,
   read-only module cache and may be copied into `vendor/` to make a tree
   self-contained ([`modules.md`](modules.md)).
5. **One validator.** `pletka project verify-git` checks a tree offline —
   syntax against the JSON Schemas in [`schema/`](schema/), structure,
   references, checksums, and every ontology path replayed through the path
   builder's engine ([`verification.md`](verification.md)).

## Reading order

| File | What it specifies |
|---|---|
| [`tree.md`](tree.md) | Layout, file naming, identity, every file kind and its keys, compact authoring forms, canonical YAML encoding. |
| [`modules.md`](modules.md) | `pletka.mod`, `pletka.sum` (grammar), module paths, resolution, the module cache, `vendor/`, the ontology repository. |
| [`versions.md`](versions.md) | Draft, releases, release branches, version selectors, `releases/index.yaml`. |
| [`verification.md`](verification.md) | Check classes, finding codes, what is not checked yet. |
| [`schema/`](schema/) | JSON Schema (2020-12) for every YAML file kind. Normative for keys and types. |
| [`DECISIONS.md`](DECISIONS.md) | Why the format is the way it is; alternatives rejected. |
| [`STATUS.md`](STATUS.md) | Where the implementation stands against this spec, rule by rule. |

## Conventions in this spec

- **MUST / MUST NOT / SHOULD / MAY** are used in the RFC 2119 sense.
- Every normative rule that the code does not implement yet carries a
  marker: **[not built]** — specified, no code; **[differs]** — the code
  does something else today, described in [`STATUS.md`](STATUS.md).
  A rule without a marker is what the code does.
- Examples use the project `X` (`XF.1`, `XM.2`, `X.CAT.3`) and the
  organisation `example`.

## Glossary

| Term | Meaning |
|---|---|
| **tree** | The files of one project at one git ref, excluding `vendor/` and `.git`. |
| **entity** | A category, concept list, field, model or collection. Each is one file (or directory) named by its semantic id. |
| **semantic id** | The stable, human-readable identity of an entity: `XF.1`, `XM.2`, `XC.3`, `X.CAT.4`, `X.CL.5`. The only identity a tree uses. |
| **override** | A file that places a field on a model or collection (position, cardinality, category, display name) or, as the base override, sets the field's own defaults. |
| **adoption** | Reuse of a parent project's entity in a child, recorded as a receipt. |
| **fork** | A copy of another project's entity that the copier then owns, recorded as a receipt. |
| **module** | Anything a `pletka.mod` can require: a project or an ontology. Identified by its module path. |
| **selector** | What a requirement asks for: a release version, a branch name, or a commit id. |
| **resolution** | Turning a selector into a commit id and a content hash; recorded in `pletka.sum`. |
| **module cache** | The local, read-only store of resolved module trees, keyed by module path and commit. |
| **vendor** | A copy of resolved dependency trees inside the tree, for self-contained verification and loading. |
| **canonical encoding** | The single byte representation the materializer writes: sorted keys, indent 2, LF, one trailing newline. |
