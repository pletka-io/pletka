# Reading, adoption and forks: details

Rules I1–I8, A1–A8, F1–F3 from [the rules page](../../versioning-and-inheritance.md).

## One way to read

Every read of content names a project and a selector, and the selector comes
from the closure:

| Reading | Project | Selector |
|---|---|---|
| the addressed project's own entity | root | the request's scope |
| an entity of a parent, at any depth | that member | its selector in the closure |
| an adopted entity | the receipt's source | its selector in the closure |
| a value target, placement or override base in another project | that member | its selector in the closure |

The earlier wording had three rules (draft view, pinned parent's parents,
release view) and a separate one for adopted entities. They are one rule
once the closure exists; the cases differ only in how the closure is
obtained ([`links-and-closure.md`](links-and-closure.md)).

A reader never joins another project's live table from its own query. The
resolver that builds a model view asks the field slice for the parent's
fields at the parent's selector.

## Corner cases

**Release view of a child.** Child at `2.0.0`, parent pinned at `1.3.0`.
The parent's fields are read at `1.3.0`. Reading them at `2.0.0` is the bug
class behind most known wrong reads: it returns nothing, or the wrong rows
when the parent happens to have a `2.0.0`.

**Draft view of a pinned child.** The parent has moved on in its draft. The
child's draft views, pickers, validation and generators all still see
`1.3.0`. To see newer content the child re-pins.

**Draft-following child.** Sees the parent's draft, including entities that
were never released, and may adopt them. It cannot cut a release in that
state (R5).

**Deprecated (I5, I6).** A field deprecated in the parent's draft but not in
`1.3.0` is offered to a child pinned at `1.3.0` and is not flagged. After
re-pinning to a release where it is deprecated, existing references warn
and pickers stop offering it.

**Entity deleted in a parent's draft.** A pinned child is unaffected. A
draft-following child gets a broken reference, reported per L10.

**Unreadable parent (I7).** Pinned: the pinned release is visible to anyone
who can read the child, like vendored code; linking required read access
(L2). Draft-following: a reader without access to the parent sees the child
without that parent's content, and the response says so.

**Exports (I8).** A generator or export run at a release reads everything
through that release's closure lock, so two runs are byte-equal. An export
of a draft-following draft is not reproducible and says which members were
drafts.

## Receipts

A receipt says "this project uses entity E of project Q". It has no version
of its own (A2, D1): the closure has exactly one selector for Q.

| Situation | Result |
|---|---|
| adopt E from a pinned member | allowed if E exists at the selector |
| adopt E from a draft-following member | allowed if E exists in the draft |
| adopt E from a project not in the closure | refused; link it first |
| adopt E that only exists in a newer release than the pin | refused; re-pin first |
| adopt E twice | one receipt |
| the selector for Q changes (re-pin) | nothing to update; L6 checked E exists at the new one |
| E is missing at the selector (old data, restore) | broken receipt: reported, blocks a cut (D3) |

**Source at any depth (A6).** Root → A → C. Root may adopt from C; C's
selector is the one A's pinned release locked.

**What can be adopted.** Fields, models, collections, categories today;
concept lists under A7. Field receipts are derived from overrides on save; that derivation
moves into the reference set so there is one place that knows which
references need a receipt.

**References without a receipt.** A value target pointing at a parent's
collection or list is a reference without a receipt today. Proposed: the
reference set treats every cross-project reference the same for checking;
whether each also needs a visible receipt is a UI question, except concept
lists, which get receipts (A7) because membership checks need a recorded
intent.

**Categories (A8).** Same as every entity.

| Situation | Result |
|---|---|
| link a parent | all its categories adopted: receipts, no copies; shown under the parent's ids |
| child's fields grouped under an adopted category | reference the parent's category id |
| child reorders categories | position is on the receipt; no adapt needed |
| parent renames a category, child re-pins | child sees the new name |
| child wants a different name | explicit adapt: new child id, copy at the selector, fork record, child's uses repointed |
| same system name in child and parent, or in two parents | both shown, labelled by project; never merged or skipped |
| parent removed | L6: adopted categories still in use refuse the removal; adapt them first |
| re-pin to a release without the category | L6 refuses, listing the uses |

Today linking copies the parent's draft categories under new child ids,
records the copies as adoptions, matches by system name, and detaches
silently on edit. Migration: M3 table in [`git-format.md`](git-format.md).

## Forks

A fork copies the entity as the closure sees it and records
`(source project, entity, version)`. For a draft-following source the
version is empty.

- The copy is read like any own entity from then on.
- The recorded version never moves (F3); it answers "what did we copy".
- A fork does not need the source to stay in the closure. Removing the
  parent leaves forks intact (L6 table).
- Forking under a release-scoped request still copies from the draft
  closure, because the fork is a draft edit (F2).

## Relations

- A2 depends on I1: a derived version only works when every reader has the
  closure. Plan phase 3 before phase 4.
- A4 and A5 are the reference set applied at save time; L6 is the same set
  applied at link-change time; R5 at cut time.
- I7 depends on L2.
- V3, X2, X5 are I1 for concept lists and examples
  ([`vocab-ontology-examples.md`](vocab-ontology-examples.md)).

## Tests to change

`TestVersion_SourceReachedThroughPinnedParent`, case "through draft
parents", expects the newest release (`3.0.0`). Under A2 the answer is the
draft selector. The pinned-parent case stays valid for A6.
