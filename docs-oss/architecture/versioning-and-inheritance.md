# Versioning and inheritance

This page states the rules for how projects are versioned, how they inherit
from each other, and how that maps onto git as the primary storage. Code and
tests are built against it. Each rule has an id to cite in tests and reviews.

## How to read this

| You want | Read |
|---|---|
| The model and the rules, one line each | this page |
| Where each decision is made in code (one place each) | [`design.md`](versioning/design.md) |
| The order of work, up to git as primary storage | [`plan.md`](versioning/plan.md) |
| Decisions D1–D11 and their reasons | [`decisions.md`](versioning/decisions.md) |
| Corner cases, rationale, relations between rules | [`rules/`](versioning/rules/) |

## The model in six sentences

1. A **project** owns content. Its **draft** is editable; a **release** is an
   immutable snapshot named by a version that means nothing outside the
   project.
2. A project depends on other projects through **parent links**, each
   **pinned** to a release or **draft-following**.
3. The **closure** of a project at a scope is the project plus everything
   reachable through links, each at exactly one **selector** (a release, or
   the draft). The addressed project's own selector is the request's scope.
4. **Every read is `(project, selector)` taken from the closure.** There is no
   other way to choose a version.
5. A project uses another project's entity through a **receipt** (still the
   source's entity, read at the closure's selector) or a **fork** (a copy,
   the project's own, remembering what it copied). In the product these are
   **adopt** and **adapt**, and they work the same for every kind of entity.
6. In git the closure is `pletka.mod` (links) plus `pletka.sum` (the locked
   closure); a release is a tag. The database holds the same facts, so moving
   to git as primary storage changes where they are stored, not what they
   mean.

## Principles

1. **One closure, one selector per project and per ontology.**
2. **Every read goes through the closure.** Own content at the request's
   scope, other projects' content at their selector, the closure itself walked
   at the request's scope.
3. **A release never changes.**
4. **No fallbacks.** Nothing silently reads another version than the closure
   selects. It refuses with a named error, or it reports.
5. **A version string means nothing outside its project.**
6. **One decision, one place.** Each decision below is taken by one function;
   handlers, CLI commands, importers and restore call it and never re-check
   or bypass it ([`design.md`](versioning/design.md)).
7. **Old data is reported, not broken.** Rules are enforced on new writes
   after an audit; releases cut before a rule are flagged and read by one
   legacy reader. A draft whose repair would change its meaning may be
   grandfathered by name (D13).

## Rules

Status: **holds**, **partial**, **not built**.
"Where" names the decision point in
[`design.md`](versioning/design.md).

### Closure and parent links (L) — [details](versioning/rules/links-and-closure.md)

| Id | Rule | Status | Where |
|---|---|---|---|
| L1 | A pinned link names a release of the parent that exists; a draft-following link names none. | partial | link guard |
| L2 | Adding or re-pinning a link requires read access to the parent. | partial | link guard |
| L3 | A link may be pinned only to a release that is not withdrawn; existing pins keep working. | partial | link guard |
| L4 | A link that would create a cycle is refused (409). A closure with a cycle or deeper than the limit fails with a named error; it is never truncated. | partial | closure |
| L5 | A closure holds each project at one selector. A link change that would create two is refused (409 `project-version-conflict`, naming both paths). | not built | closure |
| L6 | Re-pinning or removing a parent first checks every reference into the affected projects; any missing target refuses the change, listing all. Examples are reported, not blocking. | not built | link guard + reference set |
| L7 | The primary parent only sets order (namespace precedence, display). It never resolves a content conflict. | partial | closure |
| L8 | Every parent is a link row. The legacy single-parent column is migrated away, then removed.| not built | migration |
| L9 | Every link write (add, re-pin, remove, reorder, set primary; HTTP, CLI, import, restore) goes through the link guard. No upsert re-pins silently.| not built | link guard |
| L10 | A closure that became invalid through a draft-following link (the parent's draft changed) is reported in draft views and blocks a release cut; it is never repaired silently. | not built | closure |

### Releases (R) — [details](versioning/rules/releases-and-scope.md)

| Id | Rule | Status | Where |
|---|---|---|---|
| R1 | A version is semver (`MAJOR.MINOR.PATCH`, optional `-prerelease`, optional `+build`) and valid as git tag `v<version>`. | partial | version grammar |
| R2 | A release snapshots the project's own content: fields, models, collections, categories, overrides, value targets, placements, receipts, forks, concept lists with members and broader edges, vocabularies with cached entries, examples, credits, own namespace bindings. | holds | cut |
| R3 | A release locks its closure: for every project in it, the selector, parent links, project row, ontology links and namespace bindings, keyed by releasing project and version. This is the database form of `pletka.sum`. | partial | cut |
| R4 | A release copies no other project's content. | holds | cut |
| R5 | A cut is refused when the project has no content, the version exists, the closure is invalid (L4, L5, O2), any selector in the closure is a draft, or a pinned release is missing. | partial | cut (uses closure) |
| R6 | A cut holds the project's exclusive lock and reads one consistent snapshot; saves during a cut wait or get 409. | not built | cut |
| R7 | Only the cut writes release rows. Backfill tools are migrations: they run once per rule, are listed in the audit, and are the only exception. Enforced by conformance.| not built | cut |
| R8 | Withdrawing marks a release and lists the projects pinned to it. It stays readable and usable by existing pins; new pins refuse it. | partial | release state |
| R9 | Each project has an explicit current release. Every "latest" means it; nothing sorts versions. Withdrawing the current release requires naming the new one. | not built | release state |
| R10 | A project cannot be deleted while another project's release or link refers to it. Deleting a project removes all of its own release rows and nothing of another project's.| not built | reference set |

### Read scope (S) — [details](versioning/rules/releases-and-scope.md)

| Id | Rule | Status | Where |
|---|---|---|---|
| S1 | A request reads the draft or one release. `?version=` may name any release of the project; anything else is 404. Default: members read the draft; on a public project non-editors read the current release, or get "no published release" if there is none, never the draft. | partial | scope |
| S2 | The scope is resolved once at the request boundary and passed down explicitly. Nothing reads it from ambient context. | partial | scope |
| S3 | Mutations act on the draft; release views are read-only. | holds | scope |
| S4 | A failed scope lookup is an error, never a silent draft read.| not built | scope |
| S5 | Whether a release may be read is decided from the project's present visibility, not the archived one. | not built | scope |

### Reading (I) — [details](versioning/rules/reads-adoption-forks.md)

| Id | Rule | Status | Where |
|---|---|---|---|
| I1 | Every read of content takes `(project, selector)` from the closure: views, pickers, search, resolver, validation, generators, exports.| partial | entity reader |
| I4 | A draft selector inside a release's closure (releases cut before R5) is read from that draft at request time and the release is flagged `chain-unpinned`. | partial | legacy reader |
| I5 | Pickers, search and autocomplete offer only entities that exist and are not deprecated at their selector. | partial | entity reader |
| I6 | `deprecated` is content, seen as of the selector. References to a deprecated entity warn. | not built | entity reader |
| I7 | A pinned parent's content is visible to the child's readers. Through a draft-following link, a reader who cannot read the parent sees nothing of it. | not built | closure |
| I8 | Exporting the same release twice gives byte-equal output. | not built | entity reader |

### Adoption (A) and forks (F) — [details](versioning/rules/reads-adoption-forks.md)

| Id | Rule | Status | Where |
|---|---|---|---|
| A1 | A receipt is a row of the adopting project, read at its scope and snapshotted with its releases. | partial | entity reader |
| A2 | A receipt names a source project and entity. Its version is the closure's selector for the source; it is derived, never chosen. One receipt per entity. | not built | closure |
| A4 | A project adopts only from its closure, and only an entity that exists at the selector. | partial | reference set |
| A5 | A reference to an entity that does not exist at the closure's selector is refused (409). Example values are exempt and reported (X4); a cut with invalid examples warns and proceeds. | not built | reference set |
| A6 | A source may be any project in the closure, not only a direct parent. | partial | closure |
| A7 | Concept lists are adopted with receipts under the same rules. | not built | reference set |
| A8 | Categories follow adopt and adapt like every other entity. Linking a parent adopts all its categories: receipts only, shown under the parent's id at the selector, position stored on the receipt. A new id is minted only on an explicit adapt. Nothing is matched by name. | not built | link guard |
| F1 | A fork copies its source at the closure's selector and stores that version. | not built | entity reader |
| F2 | A fork never reads the request's release scope. | holds | |
| F3 | After a fork the copy is the project's own; its stored version is history and never moves. | holds | |

(A3 is folded into I1 and A2: with a derived version, "read at the receipt's
version" and "read at the selector" are the same statement.)

### Concept lists, vocabularies, ontologies, examples — [details](versioning/rules/vocab-ontology-examples.md)

| Id | Rule | Status | Where |
|---|---|---|---|
| V1 | Concept lists and vocabularies belong to one project; every member of a list is an entry of one of that project's vocabularies. | partial | |
| V2 | A released list is read with the members, labels and broader edges archived in that release. | partial | entity reader |
| V3 | A list of another project is used through a receipt (A7) and read at the selector; membership checks use that version. | not built | entity reader |
| V4 | A release reads its archived vocabulary entries; it never fetches or overwrites. | not built | entity reader |
| V5 | Every broader edge belongs to one project's scheme. | not built | |
| V6 | Term pickers search the vocabulary as it is now. | holds | |
| O1 | A project's ontologies are its own links plus those of its closure, each at its selector. | partial | closure |
| O2 | A closure holds one version per ontology, and a project links one version per ontology itself. The project's own link decides; otherwise a write is refused (409 `ontology-version-conflict`). A project that links two versions itself and uses both is grandfathered as `ontology-mixed` (D13). A read under an existing conflict warns and uses the nearest link in closure order. | not built | closure |
| O3 | An ontology version is a revision (version string + content hash). A linked or released revision cannot be re-imported or deleted. | not built | |
| O4 | The defaults ontology is an explicit link (version + hash) in every project, release and `pletka.mod`. | not built | closure |
| O5 | Namespace bindings resolve own, then closure order, then global; other projects' at their selector. Global bindings used by a release are locked with it (R3).| partial | closure |
| O6 | When the deciding ontology version differs from the one a closure member was written against, terms that member's content uses and that are missing in the deciding version are reported; never refused. | not built | reference set |
| X1 | Examples belong to the project that wrote them and are snapshotted with its releases. | holds | |
| X2 | An example is built and validated against its model at the closure's selector, plus the project's own placements. The model must be in the closure. | not built | entity reader |
| X3 | An example's status is the result of validating it at its scope; a stored status is a cache that must match. | not built | |
| X4 | A link change re-validates examples and reports; it never blocks. | not built | link guard |
| X5 | Concept values are checked against list membership at the list's selector. | not built | entity reader |

### Identity (G), git format (K), migration (M) — [details](versioning/rules/git-format.md)

| Id | Rule | Status | Where |
|---|---|---|---|
| G1 | An entity keeps its id across releases. | holds | |
| G2 | Entities are referenced by semantic id in git; no database id appears in a tree. | partial | |
| K0 | The format spec (`docs-oss/format/`) is changed first wherever a rule here differs from it; the list is in the details.| not built | |
| K1 | A parent link is a `pletka.mod` requirement (tag = pinned, branch = draft-following). `project.yaml` keeps order and primary only. | partial | |
| K2 | A release is an annotated tag `v<version>` that never moves; `releases/index.yaml` records withdrawn releases and the current one. | partial | |
| K3 | A release tree carries `pletka.sum` for the whole closure and `vendor/`; it equals R3's lock. | not built | |
| K4 | A release requires tags only. Commit selectors come with the format version that makes git primary. | partial | |
| K5 | Adoption receipts carry no version; fork records keep theirs. | not built | |
| K6 | verify-git resolves the closure with the same rules as the database resolver (L4, L5, O2, A4, A6). | not built | closure |
| K7 | Concept-list files carry members, broader edges and source vocabulary; examples are files keyed by semantic id. Release trees contain everything R2 lists. | not built | |
| K8 | Restore rebuilds pinned releases from vendored tag trees, checked against `pletka.sum`, and reports broken receipts; it never stops on old data. | not built | |
| K9 | `pletka.sum` hashes content files only. | partial | |
| M1 | Before a rule is enforced, a read-only audit lists every violation per instance. | not built | audit |
| M2 | Releases are never rewritten. Releases that break a rule are flagged and read by the legacy reader, which is the only place old fallbacks live. | not built | legacy reader |
| M3 | Drafts are brought into line per rule, by migration or by curators from the audit list, before the rule is enforced. A draft whose repair would change its meaning may instead be grandfathered: a named exception per project and flag, which cannot grow and ends when repaired (D13). | not built | audit |

## Order of work (summary)

[`plan.md`](versioning/plan.md) has the phases. In short: audit first; then one closure
resolver; then one link guard; then reads through the closure; then receipts
and the cut; then ontologies, vocabularies and examples; then git trees that
carry the full closure; then git as primary storage.
