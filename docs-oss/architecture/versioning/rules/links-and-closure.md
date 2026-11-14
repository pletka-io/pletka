# Closure and parent links: details

Rules L1–L10 from [the rules page](../../versioning-and-inheritance.md). Corner cases, reasons, and
how the rules lean on each other.

## Terms

- **Selector**: the version a closure holds a project at. Today: a release,
  or the draft. With git as primary storage: also a commit (K4).
- **Closure order**: root, then parents breadth first, primary parent first,
  then link order. Used wherever order matters (bindings, display, D7) and
  nowhere to pick between two versions of content.
- **Pinned closure**: every selector is a release. It cannot change.
- **Draft-following closure**: at least one selector is a draft. It can
  change without the root project being edited.

## How a closure is resolved

| Scope | Root's links from | A pinned member's links from | A draft member's links from |
|---|---|---|---|
| draft | live link rows | that member's release lock (R3) | its live link rows |
| release V | the lock stored with (root, V) | the same lock | the same lock (legacy only, I4) |

A pinned member's own parents are read under **that member's** pinned
version, never the root's and never its child's (principle 5). The search
walker reads them under the parent's version today; that is the kind of
difference the single resolver removes.

## Corner cases

**Same project twice at the same selector** (A → C@1 and B → C@1): allowed.
One member, two paths.

**Same project at two selectors** (A → C@1, B → C@2): refused at the write
that would create it (L5). The error names both paths so the curator knows
which parent to re-pin. There is no "nearest wins".

**Draft and release of the same project** (A → C draft, B → C@2): the same
conflict; a draft is a selector.

**Diamond that appears later.** Root follows A's draft; A's curators re-pin
C. Root did nothing. Per L10 the resolver reports
`project-version-conflict` on root's draft views, and root cannot cut until
it or A changes. A's write is not refused (D2).

**Cycle.** Refused on write. A cycle that already exists (old data, raw
SQL) makes the resolver return a `cycle` problem with the path; reads serve
the part before the cycle and show the problem, a cut is refused.

**Depth.** The limit is on levels, not on the number of projects, and
exceeding it is a problem, not a cut-off. Proposed limit: 10 levels.

**Pinned to a release that does not exist** (restore, raw writes): problem
`missing-release`; reads of that member fail with the named error; the audit
lists it (L1).

**Pinned to a release that is later withdrawn**: keeps working (L3, R8).
Re-pinning *to* it is refused. Re-pinning away from it is always allowed.

**Changing only order or primary.** Goes through the link guard because
closure order changes namespace precedence (O5) and the D7 choice. No
reference check is needed; the closure and its problems are recomputed.

**Removing a parent** removes every member reached only through it. L6
checks references into all of those, not just the direct parent.

**Re-pin of a middle project.** Root → A@1 → C@1. A cuts 2 with C@2. Root
re-pins A to 2: C's selector for root moves from 1 to 2 in the same change,
so L6 checks root's references into C as well.

## L6: what is checked

The reference set ([`../design.md`](../design.md) point 4) against the
closure as it would be after the change:

| Reference | Missing target | Deprecated at the new selector |
|---|---|---|
| receipt | refuse | warn |
| override on an inherited entity | refuse | warn |
| placement of another project's field or collection | refuse | warn |
| value target (model, collection, concept list) | refuse | warn |
| fork record | nothing (history, F3) | nothing |
| example value | report (X4, D10) | report |

All failures are listed in one response, not the first one.

## Relations

- L5, O2 and L4 are the three ways a closure can be invalid. They share one
  detection (the resolver) and three reactions (design point 2 table).
- L8 comes first in the plan because the legacy column is a second source of
  links that half the walkers ignore.
- L2 and I7 are a pair: read access is needed to link, and linking is what
  makes a pinned release visible to the child's readers.
- L3 and R8 are a pair: withdrawal blocks new pins only.
- L9 is what makes L1–L6 true: a rule checked in one handler and bypassed by
  an importer does not hold.

## Not covered yet

- Whether the depth limit should be configurable.
