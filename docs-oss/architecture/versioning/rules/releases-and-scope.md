# Releases and read scope: details

Rules R1–R10 and S1–S5 from [the rules page](../../versioning-and-inheritance.md).

## What a release consists of

| Part | Content | Keyed by |
|---|---|---|
| own content (R2) | fields, models, collections, categories, overrides, value targets, placements, receipts, forks, concept lists + members + broader edges, vocabularies + cached entries, examples + values, credits, own namespace bindings | entity id + version (entity ids are unique across projects) |
| closure lock (R3) | per member: selector, parent links, project row, ontology links with revision hash, namespace bindings; global bindings in use | releasing project + version + member |
| state (R8, R9) | withdrawn mark; whether it is the current release | project + version |

The closure lock is what `pletka.sum` is in git. Own content is the tag's
tree. Other projects' content is in neither (R4); it is in their releases.

To decide when implementing R2: whether live tables that have no archive
today hold content.

## The cut, step by step

1. Take the project's lock (R6).
2. Parse the version (R1); refuse if it exists.
3. Resolve the draft closure; refuse on any problem (L4, L5, O2,
   `missing-release`) or any draft selector (R5). The refusal lists every
   cause.
4. Compute the reference set; refuse on broken receipts or missing targets
   (A5, D3). Invalid examples do not refuse: the cut reports how many are
   invalid and proceeds.
5. Refuse if the project has no own content. "Content" means the R2 list,
   not ontology links or bindings.
6. In one consistent snapshot: write own content, write the lock.
7. Set the current release unless the caller said not to (R9).

## Corner cases

**Version grammar (R1).** `1.2.3`, `1.2.3-rc.1`, `1.2.3+build.5`,
`1.2.3-rc.1+build.5`. Not `1.0`, not `v1.2.3` (the `v` belongs to the tag),
nothing git refuses in a ref (`..`, trailing `.lock`, trailing dot). Stored
exactly as parsed.

**Versions are not ordered.** `2.0.0` cut before `1.5.1` is normal (a
hotfix on an older line). Nothing may assume the highest or the newest is
the one to show; that is what the current pointer is for (R9).

**Hotfix cut.** Cut `1.5.1` with "do not change current": current stays
`2.0.0`.

**Withdrawing (R8).** Marks the release, returns the list of projects
pinned to it, changes nothing for them: they keep reading it, adopting from
it and cutting against it. New pins and re-pins to it are refused (L3).
`?version=` still reads it. Withdrawing the current release requires naming
the new current release in the same action; when it is the only release the
pointer clears and the project has no published release.

**Un-withdrawing** does not exist today. Proposed: allowed; it only
removes the mark.

**Deleting a release** does not exist and is not proposed. Withdraw
instead.

**Deleting a project (R10).** Refused while any other project refers to it:
a live link, a receipt or other reference in a draft, or a closure lock or
archived reference in a release. When allowed, it removes all of the
project's own rows including every archive table, and no row that belongs to
another project's release.

**Backfills (R7).** A tool that adds rows to existing releases (as the
reconstruct backfill did when new kinds of content became part of a release)
breaks "a release never changes" in the letter. It is accepted only as a
migration: one run per instance per rule, listed by the audit, allowlisted
by name in conformance. After git becomes primary a release is a tag and
this cannot happen.

**Concurrent saves (R6).** A save that arrives during a cut waits for the
lock or gets 409; it never lands half inside the snapshot.

## Read scope

**Defaults (S1).**

| Project | Caller | No `?version=` |
|---|---|---|
| internal or private | member | draft |
| public | editor | draft |
| public | anyone else | current release; "no published release" if there is none, never the draft |

**Which versions exist (S1).** Only the project's own releases. Today a
parent appears to "have" every version one of its children released because
the lookup uses a closure-lock table; with R3's key that confusion cannot
occur, and the lookup should use the release table anyway.

**Failure (S4).** If the scope cannot be resolved, the request fails. Serving
the draft instead shows unreleased content to readers who should see a
release.

**Visibility (S5).** A release view is allowed when the caller may read
the project as it is now.

**Ambient scope (S2).** The scope is an argument from the boundary down. The
conformance baseline for the ambient accessor may only go down and ends at
zero (plan phase 3).

## Relations

- R5 depends on the closure resolver: "any selector is a draft" replaces two
  separate checks (draft links, draft receipts) and covers every depth.
- R3 and K3 are the same fact in two stores (D11).
- R9 feeds S1 and every picker that used to say "latest".
- R10 is the reference set run in reverse.
- M2: a release cut before these rules is flagged, not fixed
  ([`../decisions.md`](../decisions.md) D4).
