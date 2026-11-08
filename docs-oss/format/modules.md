# Modules: `pletka.mod`, `pletka.sum`, the cache and `vendor/`

A project builds on parent projects and on ontologies. This file specifies
how a tree names them, how a name becomes a repository and a commit, how the
result is pinned and verified, and where resolved trees are kept.

The model is Go modules: `pletka.mod` is `go.mod`, `pletka.sum` is `go.sum`,
the module cache is `$GOMODCACHE`, `vendor/` is `vendor/`. Where Pletka
differs, this file says so.

## Modules and module paths

A **module** is a project or an ontology. Its **module path** is the
location of its repository, without scheme:

```
github.com/example/weave-x                 a project repository (one project per repository)
github.com/pletka-io/ontologies/crm        an ontology inside the ontology repository
```

Mapping a module path to a git remote and a directory inside it:

1. For `github.com`, `gitlab.com` and `codeberg.org`, the repository is the
   first three segments (`host/owner/repo`); fetch from
   `https://host/owner/repo.git`. Remaining segments are a subdirectory of
   the repository (the module root).
2. For any other host, the path MUST contain the repository with a `.git`
   suffix: `git.example.org/team/weaves.git/x`. **[not built]** HTML
   `<meta>` discovery as in Go is not planned.
3. A `replace` directive (below) overrides both.

A module root that is a subdirectory tags its versions with the
subdirectory as prefix: crm `7.1.3` in `github.com/pletka-io/ontologies/crm`
is the tag `crm/v7.1.3`. A module at the repository root uses `v7.1.3`.

**[differs]** Today module paths are labels (`pletka.io/orgs/<owner>/projects/<ID>`,
`ontology.pletka.io/<slug>`) that resolve to nothing; dependencies come from
the database. See [`STATUS.md`](STATUS.md).

## `pletka.mod` — [schema](schema/pletka-mod.schema.json)

States what the project needs. Hand-editable; contains no hashes and no
database ids.

```yaml
schema_version: 2
module:
  path: github.com/example/weave-x
  project_id: X
require:
  projects:
    - module: github.com/example/weave-p
      project_id: P
      version: 0.1.0
  ontologies:
    - module: github.com/pletka-io/ontologies/crm
      prefix: crm
      version: 7.1.3
replace:
  - module: github.com/example/weave-p
    path: ../weave-p
```

- `require.projects` lists exactly the parents in `project.yaml`
  `inheritance.parents` (same `project_id`s). Every project requirement is a
  parent; there is no `mode`. **[differs]** v1 writes `mode: parent`.
- `require.ontologies` lists exactly the ontologies in `project.yaml`
  `ontologies.linked_versions` (same `prefix` and `version`).
  **[differs]** v1 also writes `ontology_id`, `ontology_version_id`, and
  (on a refactor branch) `artifact_digest`. Ids go; the digest moves to
  `pletka.sum`. Restore ignores `require.ontologies` today and reads
  `project.yaml` only.
- `version` is a **selector**, below.
- `replace` maps a module to a local directory (`path`, relative to the tree
  root) or to another module (`with` + `version`, e.g. a fork). A replaced module is
  read from there instead of fetched; its `pletka.sum` line is not checked.
  Used for working on a parent and child together. **[not built]** v1
  writes `replace: []` and nothing reads it.
- Only the root tree's `replace` applies; dependencies' `replace` entries
  are ignored (as in Go).

### Selectors

| Selector | Example | Resolves to |
|---|---|---|
| release version | `0.1.0`, `7.1.3`, `2.0` | the commit of tag `v<version>` (or `<subdir>/v<version>`) |
| commit id | `3f2c…` (40 hex) | that commit |
| branch | `main`, `release/0.1.x` | the branch's current head, at resolution time |
| `draft` | `draft` | alias of `main` |

Resolution order for a string that could be several: 40-hex → commit; a
tag `v<s>` exists → release; otherwise a branch named `<s>`; otherwise an
error. Version strings are opaque: no ranges, no ordering, no minimal version
selection. A release is a name, not a constraint.

**[differs]** Today only `draft` (follow the parent's database draft) and a
release version (rebuild the parent's release from archive tables) exist;
the database forbids anything else.

A branch or commit selector makes the project's **draft** follow unreleased
work. A project MUST NOT be released while any of its requirements uses a
branch selector (today's rule for `draft`, kept). A commit selector is
allowed in a release: it is as immutable as a tag. **[proposed]**

### The closure and version conflicts

The **closure** is the root plus every module reachable through `require`,
each at its resolved selector.

- **Ontologies:** one version per ontology in the closure. If two modules
  require different versions of the same ontology, the root's own
  requirement decides; if the root has none, it is an error
  (`ontology-version-conflict`). **[not built]**
- **Projects:** one selector per project in the closure, by the same rule
  (`project-version-conflict`). **[not built]** Existing data has one child
  whose closure contains a parent both as draft and as `0.1.0`; it is
  repaired when resolution through git lands ([`DECISIONS.md`](DECISIONS.md) D9).

## `pletka.sum`

Records, for every module in the closure, which commit its selector
resolved to and the hash of that module's content. Written by tools, never
by hand. Present whenever the tree has a requirement. **[differs]** Today
written only by `init-git --self-contained`, without commits.

### Grammar

```
sum-file   = *( line )
line       = module SP selector SP "git:" commit SP "tree:sha256:" hash LF
module     = 1*( %x21-7E )                    ; module path, no spaces
selector   = 1*( %x21-7E )
commit     = 40HEXLC                           ; full git commit id
hash       = 64HEXLC                           ; sha256 of the module tree, below
HEXLC      = DIGIT / %x61-66
```

Lines are sorted by `module`, then `selector`, by byte order; no duplicates;
no blank lines; one trailing LF.

```
github.com/example/weave-p 0.1.0 git:9b1e…c4 tree:sha256:5d0f…a1
github.com/pletka-io/ontologies/crm 7.1.3 git:41aa…07 tree:sha256:b242…6e
```

**[differs]** v1 lines are `<module> <version> [git:<sha>] [tree:sha256:<hex>]`
with `git:` never written. A v1 line without `git:` is accepted for a
vendored dependency; a v2 writer always writes both.

### The module tree hash

`tree:sha256` is computed over the module's **content files**:

- a project module: `project.yaml`, `pletka.mod`, and every file under
  `categories/`, `concept-lists/`, `fields/`, `models/`, `collections/`,
  `adoptions/`, `forks/`;
- an ontology module: `ontology.yaml` and every file under `src/`.

Not included: `pletka.sum`, `vendor/`, `releases/`, and any other file.

Algorithm: list content files by slash-separated path relative to the module
root; sort by byte order; feed SHA-256 with, for each file,
`path NUL bytes NUL`; the hash is the lower-case hex digest.

The same bytes hash the same wherever they are: in the dependency's
repository at the commit, in the module cache, and in `vendor/`. A tool
MUST therefore copy dependency files byte for byte, never re-render them.
**[differs]** v1 hashes every file under the vendored directory, and
vendored copies are rendered from the database, so a v1 hash says nothing
about any commit.

### Verification

For each line, with the module available (cache or `vendor/`):

1. The module tree hash recomputed over the files MUST equal `tree:sha256`.
2. If the repository is reachable, `git:` MUST be an ancestor of, or equal to,
   what the selector names (for a tag: equal). **[not built]**
3. Every module in the closure MUST have exactly one line; a line for a
   module not in the closure is an error (`sum-extra`). **[not built]**

## The module cache **[not built]**

```
$PLETKA_MODCACHE                 default ${XDG_CACHE_HOME:-~/.cache}/pletka/mod
  vcs/<sha256 of remote URL>/    bare clone, fetched as needed
  <module>@<commit>/             extracted module tree, read-only
```

- Entries are keyed by commit, not selector: a commit's tree never changes,
  so an entry is valid forever and shared by every project that uses it.
- Resolving a selector fetches the remote; resolving with a `pletka.sum`
  line present reads the commit from the line and needs no network if the
  entry exists (offline builds and CI).
- Extraction writes only the module's content files and checks the tree
  hash before making the entry visible.

## `vendor/`

Copies of the closure inside the tree, so the tree verifies and loads with
no network and no cache.

```
vendor/projects/<module>@<selector>/       content files of a project module
vendor/ontologies/<module>@<version>/      ontology.yaml + src/
```

- Flat: the whole closure sits under the root's `vendor/`; a vendored module
  has no `vendor/` and no `pletka.sum` of its own.
- Every vendored directory has a `pletka.sum` line and its hash matches.
- `vendor/` is optional. A tree without it resolves through the cache.
  A tree with it MUST be complete: a closure module missing from `vendor/`
  is an error (`vendor-missing`).
- **[differs]** v1 layout is `vendor/projects/<module>[/<version>]` (a draft
  has no version directory, so a release of the same module nests inside the
  draft's directory and its files enter the draft's hash) and
  `vendor/ontologies/<module>/<version>`.

## Ontology modules

Ontologies live in one repository, one module per prefix:

```
github.com/pletka-io/ontologies
  crm/ontology.yaml  crm/src/CIDOC_CRM_v7.1.3.rdf
  crmsci/ontology.yaml  crmsci/src/...
  namespaces.yaml                       global prefix → namespace table
tags: crm/v7.1.3  crm/v6.2  crmsci/v2.0  namespaces/v1
```

Each tag is one immutable ontology version. **[differs]** The existing
ontology repository groups files by family (`cidoc-crm/`); it is
restructured to one directory per prefix, with tags unchanged.

### `ontology.yaml` — [schema](schema/ontology.schema.json)

```yaml
schema_version: 2
ontology:
  prefix: crmsci
  title: CRMsci
  kind: extension
  namespace: http://www.cidoc-crm.org/extensions/crmsci/
  namespace_aliases: [http://www.ics.forth.gr/isl/CRMsci/]
  prefixes: [crmsci]
  source_url: https://cidoc-crm.org/crmsci
sources:
  primary: CRMsci_v2.rdf
  files: [CRMsci_v2.rdf]
  companions: []
require:
  - {module: github.com/pletka-io/ontologies/crm, version: 7.1.3}
```

- The version is the tag, not a key in the file.
- `require` lists the ontologies this one extends, at exact versions. They
  join the closure like any requirement. **[differs]** v1 has `imports:
  [string]` and writes `module`, `ontology_id`, `version_id`, `version`,
  `slug` in `ontology:`.

## Commands **[not built]**

| Command | Does |
|---|---|
| `pletka mod download` | resolve the closure, fill the cache, write `pletka.sum` |
| `pletka mod get <module>@<selector>` | add or change a requirement (and the matching `project.yaml` entry) |
| `pletka mod vendor` | copy the closure from the cache into `vendor/` |
| `pletka mod verify` | check `pletka.sum` against cache and `vendor/` |
| `pletka mod graph` | print the closure with resolved commits |

`pletka project verify-git` uses `vendor/` if present, else the cache, else
`--parent` / `--ontologies` directories (today's flags, kept as overrides).
