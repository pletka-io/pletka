# Identity, git format, migration: details

Rules G1–G2, K0–K9, M1–M3 from [the rules page](../../versioning-and-inheritance.md). The format
spec is `docs-oss/format/`.

## The target

A project is a git repository. `main` is the draft. A release is an
annotated tag. `pletka.mod` lists what the project requires; `pletka.sum`
locks the whole closure; `vendor/` holds the closure's content so the tree
verifies offline. The database rules in this directory are the same rules
with rows instead of files ([`../design.md`](../design.md), last table).

## K0: changes to the format spec

The spec is changed before code is built against it. Each line is a place
where the rules here and the spec differ today.

| Spec today | Change to | Rule |
|---|---|---|
| version: any `[0-9A-Za-z.+-]` starting with a digit, semver recommended | semver, git-tag valid, required | R1 |
| `releases/index.yaml`: `archived_at`, no other keys | add `current`; name the state "withdrawn" (keep the key if renaming costs more than it gives) | K2, R8, R9 |
| fork receipts lose `source.version` like adoption receipts | fork receipts keep it | K5, F1 |
| `adoption-unknown-parent`: source is not a parent | source is not in the closure | K6, A6 |
| concept lists: entries file planned, no source vocabulary | members, broader edges, source vocabulary in the tree and in the hash | K7, V2 |
| no examples, no vocabularies in layout or hashed files | add both | K7, X1 |
| commit selectors allowed in a release | tags only until the git-primary format version | K4 |
| defaults ontology implicit | a `pletka.mod` requirement with hash | O4 |
| one version per module: proposed | required, with the error codes of L5 and O2 | K3 |
| `_adopted_from` copies inside the project's own files | adopted content lives under `vendor/` only | R4 |

## Files and what they carry

| File | Carries | Does not carry |
|---|---|---|
| `pletka.mod` | parent requirements with selector; ontology requirements incl. defaults | order, primary |
| `project.yaml` | parent order and primary | link mode and version (K1) |
| `pletka.sum` | one line per closure member and ontology: selector and content hash | hashes of non-content files (K9) |
| `vendor/` | closure members' content at their selectors | |
| `adoptions/` | source project and entity | version (K5) |
| `forks/` | source project, entity, version copied | |
| `releases/index.yaml` | withdrawn releases, current release | anything that changes a tag's tree |
| the tag `v<version>` | the release | |

## Corner cases

**Where the release index lives.** A tag never moves, so the tree at a tag
cannot record that the release was later withdrawn. The index is read from
the branch that holds release history, not from the tag's tree. Today tags
and index sit on a separate `releases` branch rebuilt from the database;
with git as primary storage they are made by tagging `main`.

**`pletka.sum` in a draft.** A draft-following requirement has a branch
selector; its `pletka.sum` line records the commit that was vendored, so
verification is still exact for that checkout, and the tree is marked as not
releasable (R5).

**Hash scope (K9).** The hash covers the content files of a module at its
selector. It excludes nested release directories, `vendor/`, and generated
or cache files. Today it covers everything in the vendored directory.

**Receipts on restore (K8).** A receipt in an old tree may carry a version.
Restore ignores it, resolves the closure, and reports the receipt as broken
when the entity is missing at the selector (D3). Restore never refuses old
data; it returns a report.

**Pinned parents on restore (K8).** A requirement at a tag is restored as a
release (release row, archive rows, lock) from `vendor/`, checked against
`pletka.sum`. Loading it as a draft, as today, leaves a pinned link that
names a release the database does not have.

**Ontologies on restore.** A vendored ontology whose version exists with a
different hash is a new revision (O3), not skipped.

**Semantic ids (G2).** No database id in any file: not as a fallback when a
semantic id is empty, not beside a prefix, not inside a path element. An
entity without a semantic id gets one before export; the audit lists them.

**Commit selectors (K4).** A commit is immutable like a tag but is not a
release. Allowed in a release only under the format version that makes git
primary; until then the selector kinds are release and draft
([`links-and-closure.md`](links-and-closure.md), Terms).

## The gate before git becomes primary

For every project on every instance:

1. export the tree;
2. verify-git passes, using the same closure resolver as the server (K6);
3. restore into an empty database;
4. export again: byte-equal to step 1 (I8);
5. the closure from the database and the closure from the tree are equal for
   the draft and for every release.

## Migration

**M1 audit.** One command, read-only, output grouped by rule id. It uses the
closure resolver and the reference set, so it reports exactly what
enforcement will refuse.

**M2 old releases.** Never rewritten. Flags and what each keeps:
[`../decisions.md`](../decisions.md) D4. The one accepted change to
existing release rows is the key column of D5.

**M3 drafts, per rule.**

| Data | Rule | Fix |
|---|---|---|
| legacy single-parent projects | L8 | migration: write a draft-following link row |
| cycles | L4 | curators, from the audit |
| diamonds | L5 | curators re-pin; audit names both paths |
| pins to missing or withdrawn releases | L1, L3 | curators; withdrawn pins may stay |
| receipts off the selector | A2 | automatic once the version is derived; broken ones to curators |
| lists referenced without a receipt | A7 | migration: write receipts |
| categories copied on linking | A8 | copy equal to its source at the selector → pointer (uses repointed to the parent's id, copy dropped); changed copy → the child's own, fork record. Audit lists both groups first; check outside dependents on the copied ids |
| broader edges without a scheme | V5 | migration where one list uses them; else drop after review |
| ontology conflicts | O2 | curators add an own link |
| projects without a defaults link | O4 | migration: link the defaults active on the day |
| entities without a semantic id | G2 | migration |
| public projects without a release | S1 | its maintainers cut a release or accept "no published release"; audit lists them |
| closure rows shared between releases | R3 | flag (D5) |

## Relations

- K3 is R3; K1 is L1; K2 is R8 and R9; K5 is A2 and F1; K6 is the closure
  resolver; K8 is the link guard in report mode.
- K0 is phase 0 of [`../plan.md`](../plan.md): the spec moves first.
