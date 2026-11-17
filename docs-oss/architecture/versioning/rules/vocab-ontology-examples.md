# Concept lists, vocabularies, ontologies, examples: details

Rules V1–V6, O1–O6, X1–X5 from [the rules page](../../versioning-and-inheritance.md).

## Concept lists and vocabularies

**Ownership (V1).** A vocabulary is a project's cache of an external or
local source. A concept list is a selection of that project's vocabulary
entries. A list without a source vocabulary still only holds entries that
belong to its project; adding an entry by id from elsewhere is refused.

**What a release holds (V2, V4).** The list, its members, their labels and
scope notes as cached at the cut, the broader edges of the project's scheme,
and the vocabulary with its cached entries. A release view reads all of it
from the release. It never calls the vocabulary service and never joins the
live entries: a term deleted or relabelled later does not change the
release.

**The cache is the pin (V4).** External vocabularies have no version of
their own that we control. What the project cached when it released is what
the release means. In a draft, refreshing an entry is an edit like any
other.

**Other projects' lists (V3).** Used through a receipt (A7), read at the
closure's selector. A value is a member if it is a member in that release.
The membership check on save, the sidebar picker, generator enums and
example validation (X5) all ask the same reader.

**Broader edges (V5).** Every edge belongs to one project's scheme. Edges
without a scheme are migrated to the scheme of the project whose list uses
them, or dropped when no list does; the audit lists them first.

**Term pickers (V6).** Searching for a term to add is a draft edit against
the vocabulary as it is now. This is the one place a live external read is
correct.

**Not archived today:** vocabulary connector configuration. Proposed: it is
configuration, not content, and stays out of releases; a release needs only
the cached entries.

## Ontologies

**The set (O1).** Own links plus those of every closure member at its
selector, taken from the closure ([`links-and-closure.md`](links-and-closure.md)).
One function; autocomplete, validation, exports and verify-git use it.

**One version per ontology (O2).**

| Situation | Write (link change, ontology link, cut) | Read |
|---|---|---|
| one version in the closure | fine | that version |
| two versions, root links one itself | fine | the root's |
| two versions, root links none | refused, `ontology-version-conflict`, naming both paths | nearest link in closure order, with a warning (D7) |
| root links two versions itself | refused, same error | both, only for a project grandfathered as `ontology-mixed` |

The primary flag on an ontology link orders display; it does not pick a
version, and an ancestor's flag never beats the root's own link.

**Grandfathered: `ontology-mixed` (D13).** Some projects link two versions
of one ontology themselves since the import, and use terms that exist in only
one of them, from both. Removing either link breaks fields. Such a project
keeps both links as a named exception: it cannot add a third version or
re-add a removed one, its draft shows the warning, a cut carries the flag
into the release, and the exception ends when the paths are rewritten to one
version. A second link that is unused is not grandfathered; it is removed. A
project that reaches a grandfathered one and links no version itself is in
the ordinary conflict of the table above.

**Terms missing in the deciding version (O6).** Root links ontology version
B itself; a parent's release was written against version A. The parent's
fields are then read against B. The reference set lists every class or
property those fields use that B lacks, as warnings on the root's draft and
in the cut's report. It never refuses: the root chose B deliberately.

**Revisions (O3).** An ontology version is identified by version string and
content hash. Importing different content under an existing version string
makes a new revision; a revision that any project or release links cannot be
replaced or deleted. Restore compares hashes instead of skipping a version
that is already present.

**Defaults (O4).** The defaults ontology is linked explicitly, with
revision, by every project and locked in every release and in `pletka.mod`.
Moving to a newer defaults is an edit of that link. Server, git vendoring
and verify-git read the link; none of them picks "active" or "highest".
Releases cut before this carry `no-defaults-link` (D4).

**Namespace bindings (O5).**

1. the project's own;
2. each closure member's, in closure order, at its selector;
3. global.

A release locks the bindings it resolved, including the global ones in use,
under its own key, so a later change to a global binding or another
project's release with the same version string cannot alter it. Ontology
import that adds global bindings affects drafts only.

## Examples

**Ownership (X1).** An example belongs to the project that wrote it, also
when its model belongs to a parent.

**Validation (X2).** The example's model, that model's fields and nested
collections are read through the closure; the project's own placements and
overrides are applied on top. Creating an example for a model outside the
closure is refused. A linked example is read at the same scope as the
example that links it, and must belong to a project in the closure.

**Status (X3).** Status is a function of (example, scope). The stored value
is a cache; the cut stores the status computed at the cut. A read that finds
a different result shows the fresh one and the audit lists the mismatch.

**Link changes (X4).** After a re-pin the project's examples are
re-validated and the differences returned with the change. They never block
it (D10).

**Concept values (X5).** Checked against the list at its selector (V3).

**Not rules, but consequences:**

- An example of a pinned parent's model does not change when the parent's
  draft does.
- A release's examples validate against the same closure the release locked,
  so their archived status stays true.

## Relations

- V3 needs A7 (receipts for lists), which needs the receipt table to accept
  the kind and the format spec to keep doing so.
- O1, O2, O4, O5 are all outputs of the closure resolver.
- X2, X5 are the reading rule (I1) applied to examples.
- O3 and K9 meet in `pletka.sum`: the revision hash is the module hash.
