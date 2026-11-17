# Decisions

The decisions behind the rule set, taken in November 2026, with the options
that were weighed.

| # | Question | Recommendation | Answer |
|---|---|---|---|
| D1 | Receipt version: stored and authoritative, or derived from the closure? | Derived. | **Derived** (2026-11-14) |
| D2 | A draft-following parent changes and breaks the child's closure or references. Check at the parent's write, or report at the child's read? | Report at read, block the cut. | **Report at read** (2026-11-14) |
| D3 | A receipt whose entity is missing at the selector (restore, old data). | Keep as a "broken receipt", reported; no version stored. | **Keep and report** (2026-11-14) |
| D4 | Releases cut before a rule: which old behaviours stay, and where? | One legacy reader, per flag. | **Legacy reader with flags** (2026-11-14) |
| D5 | Closure rows keyed by version only, some already merged between projects. | Add the releasing project to the key; flag merged ones. | **Accepted** (2026-11-14) |
| D6 | Withdrawing the current release. | Refuse unless the same action names a new current release (or none exists to name). | **Accepted**; with no current release non-editors see no content (2026-11-14) |
| D7 | Which ontology version does a read use under an unresolved conflict? | Nearest link in closure order, with a warning. | **Accepted**, plus a report of terms missing in the deciding version (2026-11-14) |
| D8 | Categories copied from a parent on linking. | Forks at the selector. | **Pointer model** (option A) with five sub-decisions (2026-11-14, after review) |
| D9 | Permission to read a release: present or archived project visibility? | Present. | **Present** (2026-11-14) |
| D10 | Example values are references, but must not block. | Exempt from A5 and L6; reported. | **Exempt; a cut warns only** (2026-11-14) |
| D11 | Keep the closure lock in the database although a pinned parent's own release already holds its links? | Keep. | **Keep** (2026-11-14) |
| D13 | A draft breaks a rule and repairing it is modelling work, not cleanup. Must it be repaired before the rule is enforced? | No: it may be grandfathered, by name. | **Grandfathering allowed** (2026-11-17) |

## Detail

### D1. Receipt version is derived

**Decided 2026-11-14: derived.**

Today a receipt stores `source_version` and readers use it (old A3), while
inherited reads use the link (old I1). The two agree only while every write
keeps them equal, and three paths do not: a draft-following chain where a
middle project re-pins, restore, and old releases. Git already has no version
on receipts (K5).

Recommended: a receipt is `(adopting project, source project, entity)`; its
version is whatever the closure says. Re-pin needs no re-stamp. In a release,
the closure lock gives the version, so archived receipts need none either.
The column stays during the transition as a cache that the audit compares
with the closure; old releases with a differing value get
`receipt-off-selector` and are read by the legacy reader (D4).

Cost: every receipt reader must have the closure at hand (plan phase 3 before
phase 4). Alternative: stored version wins and every closure change
re-stamps, including through draft-following chains; that needs cascading
writes into other projects' drafts.

### D2. Draft-following closures are validated at read

**Decided 2026-11-14: report at read, block the cut; no cascading checks.**

L5, L6 and O2 check when *this* project changes a link. If B follows A's
draft and A re-pins C, B's closure can gain a diamond, a dangling reference
or an ontology conflict without B doing anything.

Recommended (L10): do not cascade. A draft-following closure is resolved, and
therefore validated, on every read; problems are shown in B's draft views and
block B's release cut. A pinned closure cannot change, so it never needs
this. Entity deletion or deprecation in a parent's draft is covered the same
way.

Alternative: A's write is refused when it would break any draft-following
dependent. That makes a parent's editing depend on projects its curators may
not be able to see.

### D3. Broken receipts

**Decided 2026-11-14: keep and report; never deleted automatically.**

K8 (keep and report) contradicted A2 and A5 (no other version, refuse). With
D1 there is no version to keep: the receipt stays, the reference check marks
it broken, the audit and the restore report list it, draft views show it, and
a cut is refused until it is removed or the link is re-pinned to a release
that has the entity.

### D4. Legacy reader

**Decided 2026-11-14: one legacy reader with flags.** After the audit, a
flag that only a few releases carry may be cleared by re-releasing those
instead of building its reader branch; that is decided per flag from the
counts.

"Old releases are read as before" keeps fallbacks that principle 4 forbids.
Recommended: name each flag, set it by audit, and keep the matching old
behaviour in one reader:

| Flag | Meaning | Old behaviour kept |
|---|---|---|
| `chain-unpinned` | closure contains a draft selector | that parent is read from its draft at request time |
| `closure-ambiguous` | closure rows shared with another project's release of the same version | read the shared rows as today |
| `receipt-off-selector` | archived receipt version differs from the closure | read at the archived receipt version |
| `no-defaults-link` | cut before O4 | active defaults ontology |
| `diamond` | closure holds a project at two selectors | nearest hop wins |

Each flag disappears when no instance has such a release (re-release, or
withdrawal with nothing pinned).

### D5. Closure rows

**Decided 2026-11-14:** the key column may be added to existing release rows
and backfilled (it is bookkeeping, not content); rows shared by two releases
flag both `closure-ambiguous`. Global namespace bindings likewise.

Adding the releasing project to the key is a schema change on release rows.
Recommended reading of principle 3: a release's *content* never changes; a
key column backfilled from the release that wrote the row is not content.
Rows that two releases share cannot be attributed; both releases get
`closure-ambiguous`. The audit counts them first; if the count is zero on
every instance, the flag is never needed.

The same applies to global namespace bindings archived under `(id,
version)`.

### D6. Current release and withdrawal

**Decided 2026-11-14:** withdrawing the current release requires naming the
new one in the same action; if it is the only release the pointer clears. A
public project with no current release shows non-editors no content ("no
published release"), never the draft. This changes today's behaviour for
public projects that never released; the audit counts them first.

R9 sets the pointer at cut time only; R8 says nothing about the current
release. Recommended: `release.SetCurrent` exists as its own action;
withdrawing the current release requires naming the new one. If it is the
only release, the pointer clears and the public default falls back as S1
says. 

### D7. Reads under an ontology conflict

**Decided 2026-11-14:** nearest link in closure order, with a warning. The
related gap below becomes rule O6: a report, not a refusal.

O2 said reads warn, not what they read. Existing drafts have conflicts
(e.g. two versions of one ontology through two parents). Recommended: the
closure resolver picks the nearest link in closure order, deterministically,
and returns the conflict as a problem. Releases cannot be cut in that state
(R5).

A related gap (now O6): when the project's own link overrides a
parent's ontology version, the parent's content is read against a version it
was not written for. The reference set could report terms missing in the
deciding version; proposed as a report, not a refusal.

### D8. Categories on linking

**Decided 2026-11-14, after reviewing today's behaviour: pointer model.**

- Linking a parent adopts its categories: a receipt only. The child shows
  the parent's category under the **parent's id**, read at the selector. No
  copy, no new id.
- A new id is minted only on adapt.
- 1 Order: the position of an adopted category is stored on the receipt; the
  child reorders without adapting.
- 2 Name clash: categories are never matched, merged or skipped by system
  name. Two with the same name are both shown, each labelled with its
  project.
- 3 Linking adopts all of the parent's categories automatically.
- 4 Adapt is explicit. An adopted category is read-only; "Adapt" mints a
  child id, copies the category at the selector, records the fork and
  repoints the child's own uses. Editing never detaches silently.
- 5 Existing copies: a copy equal to its source at the selector becomes a
  pointer (uses repointed to the parent's id, copy dropped); a changed copy
  stays the child's own and is recorded as a fork. The audit lists both
  groups before anything moves. Existing releases are not touched.

To check before migrating (5): whether anything outside the application
(exported trees, downstream packs) depends on the re-minted child category
ids; if so, the migration keeps them resolvable.

Context (2026-11-14): the product logic is adopt
(follow the parent's version) then adapt (own copy). The designers wanted
adopted things kept as pointers under the parent's ids, and new ids minted
only on adapt.

What the code does today (`pkg/weave/settings/handler.go` `copyParentCategories`,
`pkg/weave/category/service.go` `detachProjectCategoryAdoption`), read
2026-11-14 at e8c9de8:

- Linking a parent creates, for every parent category whose system name the
  child does not have yet, a **new child category with a new child id**
  (`<CHILD>.CAT.<n>`), copying names, description and status from the
  parent's draft, appended after the child's own categories.
- It also writes an adoption receipt pointing at the parent's category. Copy
  and source are matched afterwards by system name, not by id.
- The copy never updates when the parent changes or the link is re-pinned.
- Editing the copy's system name, display name or description silently
  removes the receipt (an implicit adapt). Reordering does not.
- Removing the parent deletes copies that nothing uses and drops the
  receipts; copies in use stay as the child's own.

So ids are re-minted at link time, which is what the designers wanted to
avoid, and the receipt records a "follow" that does not happen.

Earlier text, kept for the review:

Adding a parent copies its draft categories into the child and writes an
adoption receipt, although the result is a copy the child owns. Recommended:
record it as a fork (with the selector's version, F1) and copy from the
selector, not the draft. Needs a check with the people who use categories
across projects before it is fixed.

### D9. Visibility of releases

**Decided 2026-11-14: present visibility.**

Recommended: a release's readability
follows the project as it is now; content already vendored into pinned
children stays visible there (I7).

### D10. Example values

**Decided 2026-11-14:** exempt and reported. A cut with invalid examples is
allowed and shows how many are invalid; it does not refuse or ask for
confirmation.

Terms list example values as references; L6 and X4 say examples never block;
A5 said any reference is refused. Recommended: examples are illustrations.
Saving one with a value that fails validation is allowed and gives it an
invalid status (X3); link changes report (X4); a cut is not refused for
invalid examples. 

### D11. Closure lock in the database

**Decided 2026-11-14: keep.**

Once R5 holds, a pinned parent's links are already in its own release, so
copying them under the child's release (R3) is redundant. Recommended: keep
it. It is the database form of `pletka.sum`, it makes a release view one
lookup instead of a walk, and the round trip database ↔ git (plan phase 7)
compares exactly these two.


### D13. Grandfathering a draft

**Decided 2026-11-17: a draft that breaks a rule may be grandfathered instead
of repaired.**

M3 said drafts are brought into line before a rule is enforced. The first
audit showed a case where that is not a cleanup. Eight projects link two
versions of one ontology themselves; all sixteen links date from one import.
For four of them one link is unused and can be removed. The other four use
terms that exist in one version only, from both versions: three with CIDOC-CRM
6.2 and 7.1.3 (the temporal relations P114–P120 exist in 6.2 only), one with
two versions of another ontology. Removing a link there breaks fields; keeping
both breaks O2. Rewriting those paths is a modelling decision for whoever
maintains the project and should not hold up the rule for everyone else.

What grandfathering means:

1. **It is by name.** An exception is one project, one flag, one subject (for
   `ontology-mixed`: the ontology and the two versions), set from an audit
   run, with the date. There is no "grandfather everything as it is": what
   the audit can repair without changing meaning is repaired.
2. **The rule is enforced for everyone else, and for the same project on new
   writes.** A grandfathered project cannot make its exception larger: no
   third version, and a link it removed cannot come back.
3. **It is visible.** The audit lists it as grandfathered, not as a new
   violation, and the project's draft shows it as a warning.
4. **A cut is allowed** and the release carries the flag, so the legacy
   reader (D4) reads it the way the draft was read. Otherwise such a project
   could never release again.
5. **It ends once.** When the draft is brought into line the exception is
   removed and cannot be set again.
6. **It does not spread.** A project that reaches a grandfathered project and
   has no own link for that ontology is in the ordinary O2 conflict and
   resolves it with its own link.

Flags a draft can carry:

| Flag | Meaning | Behaviour kept | Ends when |
|---|---|---|---|
| `ontology-mixed` | the project links two versions of one ontology itself and uses terms that only one of them has, from both | both versions stay linked and are read as today | the paths are rewritten to one version and the other link is removed |

A new flag is added here when an audit shows the same situation for another
rule: repair changes meaning, and the choice is to keep the state.

Point 4 follows from the others rather than from the decision itself and is
open to revision. The alternative is that a grandfathered project cannot
release until repaired.
