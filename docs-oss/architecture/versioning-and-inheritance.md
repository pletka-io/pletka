# Versioning and inheritance

This page states the rules for how projects are versioned, how they inherit
from each other, and how one project uses another's content. Code and tests
are built against it. Each rule has an id to cite in tests and reviews, and a
status:

- **holds**: the code does this today.
- **partial**: some paths do, others do not.
- **not built**: decided, not implemented.

The git storage format (`docs-oss/format/`) is the target representation;
section 11 maps the rules onto it.

## Terms

- **Project**: a workspace that owns fields, models, collections, categories,
  concept lists, vocabularies and examples.
- **Draft**: a project's live, editable rows.
- **Release**: an immutable snapshot of a project, named by a **version**. A
  version belongs to one project; two projects may both have `1.0.0`.
- **Current release**: the release a project marks as its default (rule R9).
- **Withdrawn release**: a release marked as no longer offered for new use; it
  stays readable (rule R8).
- **Parent link**: a project's dependency on another project, either
  **pinned** to one of its releases, or **draft-following** (it sees the
  parent's draft). A project may have several parents; one is primary.
- **Closure**: a project and every project reachable through its parent links,
  each at one version (its **selector**: a release, or the draft).
- **Receipt (adoption)**: a project's record that it uses an entity of another
  project in its closure.
- **Fork**: a copy of another project's entity into the project as its own,
  with a record of where it came from.
- **Reference**: anything in a project that points at another project's
  entity: a receipt, an override on an inherited entity, a placement, a value
  target (resource model, collection model, concept list), a category, an
  example value.
- **Scope**: what a request reads: the draft, or one release of the project it
  addresses.

## Principles

1. **A project's own content is read at the request's scope; another
   project's content is read at the version the closure selects for it; the
   closure itself is walked at the request's scope.**
2. **A release never changes.**
3. **One version per project and per ontology in a closure.**
4. **No fallbacks.** A project never silently reads another version than the
   closure selects ("newest release", "the child's version number"). It
   refuses with a named error, or it reports.
5. **A version string means nothing outside its project.** Never read project
   P's releases at another project's version string.

## 1. Parent links (L)

**L1.** A pinned link names a release of the parent; a draft-following link
names none. *holds* (check constraint on the link table and its archive).

**L2.** Adding or re-pinning a link requires read access to the parent.
*not built.*

**L3.** A link may be pinned only to a release that is not withdrawn.
Existing pins to a release withdrawn later keep working (R8). *partial*:
re-pin accepts a withdrawn release.

**L4.** Adding a link that would create a cycle is refused (409). A read or a
release cut that meets a cycle, or a chain deeper than the limit, fails with a
named error and never truncates. *not built*: chain walks stop silently at
depth 10 and skip cycle edges.

**L5.** A closure holds each project at one selector. Adding or re-pinning a
link is refused (409 `project-version-conflict`) when the closure would hold a
project at two selectors (a diamond); the error names both paths. *not built*:
today the pin lookup picks one arbitrarily and a release snapshots both.

**L6.** Re-pinning a parent, or removing it, first computes every reference
the project holds into that parent and into the projects reached through it.
It is refused (409) when any referenced entity is missing from the new
selector (or, on removal, from the remaining closure), listing them all.
Examples are re-validated and reported, never blocking (X4). On success every
receipt whose closure selector changed is re-stamped to the new selector.
*not built*: re-pin changes only the link; removal checks receipts only.

**L7.** The primary parent only sets order: namespace-binding precedence
(O5), display order, and the legacy single-parent column. It never resolves a
content conflict (L5 and O2 do). *holds.*

**L8.** A project with no parent links but a legacy single-parent column has
one draft-following parent. *holds*; retired by migration M3.

## 2. Releases (R)

**R1.** A version is semver (`MAJOR.MINOR.PATCH`, optional `-prerelease` and
`+build`) using only characters valid in a git tag `v<version>`. *partial*:
`MAJOR.MINOR.PATCH` is enforced; pre-release and build parts are not
accepted, and the git spec allows other names.

**R2.** A release snapshots the project's own content under its version:
fields, models, collections, categories, overrides, value targets,
placements, receipts, forks, concept lists with their members and broader
edges, vocabularies with their cached entries, examples, and the project's own
namespace bindings. *holds.*

**R3.** A release also snapshots its closure's structure: for every project in
the closure, its parent links, its project row, its ontology links and its
namespace bindings, each taken from that project at its selector. These rows
are keyed by the releasing project and version, so they can never mix with
another project's release of the same version. *partial*: parents' namespace
bindings are not snapshotted, and the rows are keyed by version only.

**R4.** A release copies no other project's content; that content is read from
its own release (I2, I3). *holds.*

**R5.** A release cut is refused when the project is empty, the version
exists, any link anywhere in the closure follows a draft, any receipt follows
a draft, or the closure holds two versions of an ontology without the
project's own link deciding (O2). *partial*: only direct links are checked;
there is no ontology check.

**R6.** A release cut holds the project's exclusive lock and reads one
consistent snapshot; saves during a cut wait or are refused (409). *not
built.*

**R7.** No statement writes to a release's rows except the cut that creates
them; enforced by `pkg/conformance`. *partial*: true by convention, not
enforced.

**R8.** Withdrawing a release marks it withdrawn and lists the projects pinned
to it. A withdrawn release stays readable, and stays usable by projects
already pinned to it (adopting, referencing); only new pins and re-pins
refuse it (L3). *partial*: withdrawal checks nothing, and the receipt checks
treat a withdrawn release as absent.

**R9.** Each project has an explicit current release, set when a release is
cut (by default the new one; a cut may leave it unchanged, e.g. a hotfix on an
older line). Every "latest release" (public default, pickers) means the
current release; nothing sorts versions. *not built*: today one path takes the
highest number and others the most recently cut.

**R10.** A project cannot be deleted while another project's release refers to
it (closure rows, receipts, references); it can be deprecated instead. *not
built*: delete checks live links and receipts only, and deleting a project
removes rows other releases read.

## 3. Read scope (S)

**S1.** A request reads the draft or one release of the project it addresses.
An explicit `?version=` may name any release, withdrawn or not; an unknown
version is 404. Without one: members of an internal or private project read
the draft; on a public project an editor reads the draft and anyone else the
current release (R9), or the draft if there is none. *partial*: "current" is
the highest version number today.

**S2.** The scope is resolved once at the request boundary (HTTP handler or
MCP tool) and passed down. *holds* (enforced by `pkg/conformance`).

**S3.** Mutations act on the draft; release views are read-only. *holds.*

## 4. Reading inherited content (I)

**I1.** In a draft view, a project's parents are its links; each parent's
content is read at the link's release, or from its draft for a
draft-following link. *partial*: entity search does; model and collection
option pickers, concept-list options and the detail view read parents' drafts.

**I2.** A pinned parent's own parents are the links recorded in its pinned
release, and each is read at its own link's release. *partial*: entity search
and the release cut do; the shared project-chain helper drops versions.

**I3.** In a release view at version V, the closure comes from the structure
snapshot under V (R3), and each other project's content is read at its
selector, never at V (principle 5). *partial*: entity search does; the
override resolver, the detail view and examples look parents up at V.

**I4.** A draft-following link inside a release's snapshot (possible only in
releases cut before R5) is read from that parent's draft at request time, and
the release is flagged `chain-unpinned`. *partial*: read from the draft; no
flag.

**I5.** Pickers, search and autocomplete offer only entities that exist, and
are not deprecated, at the selector they would be read at. *partial* (I1).

**I6.** `deprecated` is content: a project sees another project's entity as
deprecated or not as of its selector. References to an entity deprecated at
the selector show a warning. *not built.*

**I7.** Once linked, a parent's content at the pinned release is visible to
the child's readers, like vendored code. Through a draft-following link, a
reader who cannot read the parent sees nothing of it. *not built*: only the
child's read permission is checked.

**I8.** Generators and exports at a scope resolve other projects' content per
I1–I3, and exporting the same release twice gives byte-equal output. *not
built.*

## 5. Adoption (A)

**A1.** A receipt is a row of the adopting project, read at its scope and
snapshotted with its releases. *holds.*

**A2.** A receipt's version is the closure's selector for its source project:
the pinned release, or empty when the source is reached only through
draft-following links. It has no other value. *partial*: when the selector
lacks the entity, or the path is draft-only, the writer falls back to the
newest release that holds it.

**A3.** An adopted entity is read at its receipt's version, whatever the
request's scope. *partial*: adopted-entity lists do; the detail view and
adopting a collection into a model read the source's draft.

**A4.** A project adopts a source entity only at the closure's selector: from
the pinned release, or from the draft through a draft-following link. The
entity must exist there. *partial*: the adopt panel offers the source's newest
release, the adoption endpoint accepts any live release and refuses drafts,
and the detail-view adopt action checks nothing.

**A5.** Any reference to an entity that does not exist at the closure's
selector is refused (409), never recorded at another version. *not built.*

**A6.** A source may be any project in the closure, not only a direct parent.
*holds* for the version lookup; verify-git accepts direct parents only (K6).

**A7.** Concept lists are adopted with receipts under the same rules. *not
built*: receipts cannot name a concept list.

## 6. Forks (F)

**F1.** A fork copies its source at the version its receipt recorded and
stores that version on the fork; a receipt that follows a draft copies the
draft. *not built*: fork copies the source's draft and stores no version.

**F2.** A fork never reads the request's release scope. *holds.*

**F3.** After a fork the copy is the project's own entity. Its recorded source
version is history: a later re-pin does not move it. *holds.*

## 7. Concept lists and vocabularies (V)

**V1.** Concept lists and vocabularies belong to one project; a list's source
is one of that project's vocabularies. *holds.*

**V2.** A released concept list is read with the members, labels and broader
edges archived in that release. *partial*: member labels are read from the
live vocabulary entries.

**V3.** A list from another project is used through a receipt (A7) and read at
the closure's selector; membership checks on values use that version. *not
built*: lists of other projects are referenced without a receipt and read
from their draft.

**V4.** An external vocabulary is pinned by the entries cached in the
project: a release reads its archived entries and never fetches from the
service or overwrites them. *partial*: resolving a term may re-fetch and
overwrite placeholder labels.

**V5.** Every broader edge belongs to one project's scheme; edges without a
scheme are not allowed. *not built*: unscoped edges exist and are never
archived.

**V6.** Term pickers search the vocabulary as it is now, since choosing a term
is a draft edit. *holds.*

## 8. Ontology versions (O)

**O1.** A project's ontologies are its own ontology links plus those of every
project in its closure, each at its selector. *holds.*

**O2.** A closure holds one version per ontology. The project's own link
decides; without one, a conflict is refused (409
`ontology-version-conflict`) when adding an ontology link, adding or
re-pinning a parent, and cutting a release. Reads never fail on a conflict;
they warn. *not built*: both versions are used, and on a clash the ancestor's
class wins in autocomplete.

**O3.** An ontology version is a revision identified by version string and
content hash. A revision that a project or release links cannot be
re-imported or deleted; changed content is a new revision. *not built*:
re-import replaces a version under the same id, and delete counts only draft
links.

**O4.** The defaults ontology is an explicit link (version and hash) in every
project and release; moving to a newer defaults is a deliberate change. *not
built*: every project uses the active defaults, also in release views.

**O5.** Namespace bindings resolve from the project's own, then the closure's
(primary parent first, then link order), then global ones. Other projects'
bindings are read at their selector at every depth, and snapshotted with the
closure (R3). *partial*: only direct parents' draft bindings are read, and
releases keep only own and global bindings.

## 9. Examples (X)

**X1.** Examples belong to the project that wrote them and are snapshotted
with its releases. *holds.*

**X2.** An example of a model from another project is built and validated
against that model, its fields and nested collections at the closure's
selector, plus the project's own placements. *not built*: examples resolve
other projects' content at the request's scope.

**X3.** An example's status is always the result of validating it at its
scope; a stored status is a cache that must match. *not built*: the stored
status and a fresh validation can differ.

**X4.** Re-pinning a parent re-validates the project's examples and reports
stale or invalid values; it does not block the re-pin. *not built.*

**X5.** Concept values in examples are checked against list membership at the
list's selector (V3). *not built.*

## 10. Identity (G)

**G1.** An entity keeps its id across releases. A release row is keyed by the
live key plus the version, and, for closure-structure rows, the releasing
project (R3). *partial* (R3).

**G2.** Entities are referenced by semantic id in the git format; categories
are keyed by their semantic id. *holds.*

## 11. Git format (K)

The same rules, with the closure explicit in `pletka.mod` and `pletka.sum` and
other projects' content vendored.

**K1.** A parent link is a `pletka.mod` requirement; its selector is a release
tag (pinned) or a branch (draft-following). `project.yaml` does not repeat it.
*partial*: `project.yaml` still carries the link mode and version, and
restore reads them from there.

**K2.** A release is an annotated tag `v<version>` that never moves;
`releases/index.yaml` records withdrawn releases and the current release (R8,
R9). *partial*.

**K3.** A release's closure is each module's `pletka.mod` at its tag plus
`pletka.sum` lines for the whole closure; one selector per module (L5).
*not built*: release trees carry neither `pletka.sum` nor `vendor/`.

**K4.** A release requires tags only (R5). Commit selectors (a parent's
unreleased but immutable commit) are allowed once the format version that
makes git the primary storage is in use. *not built*.

**K5.** Adoption receipts carry no version (it comes from `pletka.mod`); fork
records keep the version they copied (F3). *partial*.

**K6.** verify-git accepts a receipt whose source is any module in the closure
and checks that the entity exists in that module's vendored tree at its
selector. *not built*: only direct parents are accepted.

**K7.** Concept-list files carry their members, broader edges and source
vocabulary; examples are files keyed by semantic ids and placement. *not
built*.

**K8.** Restore rebuilds a pinned parent's release from its vendored tag tree,
checked against `pletka.sum`. It re-stamps a receipt to the closure's
selector when the entity exists there, and otherwise keeps it and reports it;
it never stops on old data. *not built*: restore loads drafts only, so a
restored pinned link can name a release that does not exist.

**K9.** `pletka.sum` hashes the content files. *partial*.

## 12. Migrating existing data (M)

**M1.** Before a rule is enforced, a read-only audit lists every existing
violation of it per instance. *not built.*

**M2.** Existing releases are never rewritten (principle 2). Releases that
break a rule are flagged (e.g. `chain-unpinned`) and read as before.

**M3.** Drafts are brought into line per rule, by migration or by curators
working from the audit list, before the rule is enforced. Legacy
single-parent projects get a link row (L8).

## Tests

Every rule is a test name in waiting. Existing tests:

| Rule | Test |
|---|---|
| I2, I3, I4 | `pkg/weave/search` `TestResolveProjectTargets_ParentPinsComeFromArchivedLink` |
| I5 (search) | `pkg/weave/search` `TestInheritedSearch_UsesPinnedParentReleaseState` |
| A3 | `pkg/weave/model`, `pkg/weave/field` `TestListAdoptedExplicit_ReadsSourceAtReceiptVersion`; `pkg/weave/collection` scoped reader tests |
| A6 | `pkg/database/adoptionsource` `TestVersion_SourceReachedThroughPinnedParent` |
| F2 | `pkg/weave/model` `TestClone_ReadsDraftEvenUnderAReleaseScopedRequest` |
| S1 | `pkg/weave/search` `TestSearch_ReleaseDefault_ServesReleasedSnapshot`; `pkg/weave/router` release visibility tests |
| G2 | `pkg/weave/category` `TestCreate_KeysCategoryBySemanticID`, `TestMigrationRekeysCategoriesBySemanticID` |
