# Format decisions

Why the format is what it is. Newest first within each topic. Each entry:
the decision, why, what was rejected.

## Storage

**D1. The repository is the format; git refs are the versions.** (2026-11-08,
decided: git is the source of truth for project content.)
One repository per project, draft = `main`, release = tag, old release edited
on a branch. Why: a tag is immutable by git, a reader at a tag cannot see the
draft, and a dependency at a release is the dependency's tree at its tag —
the guarantees a database with live and archive tables must enforce with
triggers and parallel rule sets in two languages.
Rejected: a separate complete release archive format (per-row JSON plus a
hashed inventory, "format 3"): a second format to keep equal to the first,
not writable by hand.

**D2. One format for authoring and storage.** (2026-11-05, extended
2026-11-08.) The tree the materializer writes is the tree people write. Every
written key has a default; paths may be qname strings; ontologies are linked
by prefix and version. Why: a hand-editable source and a reviewable diff.

## Identity

**D3. Semantic ids only.** (2026-11-05.) Files are named by, and reference,
semantic ids; database ids are optional output and never required input.
Why: trees move between instances; ids do not.

## Modules

**D4. Go modules as the model.** (2026-05-07, revived 2026-11-08.)
`pletka.mod` = intent, `pletka.sum` = resolved fact, a read-only module
cache, optional flat `vendor/`. Why: a known, proven design for exactly this
problem — reproducible builds from named versions of remote repositories.

**D5. Module path = repository location.** (2026-11-08.) Rejected:
`pletka.io/orgs/<owner>/projects/<id>` labels plus a resolver service (needs
infrastructure, and the label resolved to nothing for six months); an
explicit mapping file (one more file to keep in sync).

**D6. Selectors: release version, branch, or commit; the sum pins a
commit.** (2026-11-08.) Why: following a parent's unreleased work is a real
need (today's `draft`), and a commit is as immutable as a tag. `draft`
stays as an alias of `main`. Rejected: version ranges and minimal version
selection — versions here are names chosen by modelling teams, not semver
promises (05-11 decision kept).

**D7. All hashes live in `pletka.sum`; `pletka.mod` has none.** (2026-11-08.)
Rejected: an `artifact_digest` per ontology requirement in `pletka.mod`
(written by an unmerged branch): it duplicates the sum and makes the
hand-edited file carry machine values.

**D8. The tree hash covers content files only.** (2026-11-08.) Excludes
`pletka.sum`, `vendor/`, `releases/` and non-format files, so the same
module hashes the same in its repository, in the cache and in `vendor/`, and
archiving a release changes no hash. Algorithm unchanged from v1.

**D9. One version per module in the closure, root decides.** (2026-11-08,
decided.) Two versions of the same parent in one closure means two
definitions of the same semantic ids, so it is forbidden. Existing data has
one such case (a child reaching one parent both as draft and as a release,
via two parents); it is repaired once resolution through git works, not
before. Rejected: allowing it with a precedence rule.

**D10. Ontologies: one repository, one module per prefix, tag
`<prefix>/v<version>`.** (2026-11-08.) Go's multi-module repository layout.
Rejected: keeping family folders with a manifest mapping prefixes to files
(custom resolution rule, not addressable by subdirectory).

**D11. Ontology versions require their bases.** (2026-11-08, proposed.) An
extension's `ontology.yaml` requires the base versions it was aligned to, so
"one version per ontology" is checked over a closure, not by convention.

## Validation

**D12. JSON Schema is normative for keys and types; Go is normative for
meaning.** (2026-11-08.) Schemas are readable by editors (YAML language
server), other languages and agents. Semantic rules (paths, references,
closure) stay in one Go validator shared with the server. Rejected: CUE
(another toolchain), schema-in-Go-only (unreadable outside Go).

**D13. Strict decoding.** (2026-11-08.) Unknown keys are errors, in the
loader as in the validator. Why: the loader dropping an unknown key
silently turned typos into lost data.

**D14. Enumerations open until data is clean.** (2026-11-08.) `status`,
`visibility`, `expected_value_type` list their known values but do not
reject others yet; existing data holds variants that a normalisation must
fix first.
