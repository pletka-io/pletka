# Design: one decision, one place

Today the same decision is taken in several places that disagree: seven
functions walk the parent chain, three define "latest release", two paths
write links, and checks sit in handlers while writes sit in stores. The rules in [the rules page](../versioning-and-inheritance.md) only
stay true if each decision has one owner. This page names those owners.

Package names are proposals; the boundaries are the point.

## Decision points

| # | Decision | One function | Callers (never decide themselves) |
|---|---|---|---|
| 1 | Which scope does this request read? | `scope.Resolve(request) → Scope` | HTTP middleware, MCP tools, page handlers |
| 2 | What is the closure, and is it valid? | `closure.Resolve(project, scope) → Closure, []Problem` | every read, the cut, the link guard, verify-git, the audit |
| 3 | May this link change happen? | `links.Change(project, change) → error` | settings handler, CLI re-pin, importers, restore |
| 4 | What does project P reference in other projects? | `refs.Of(project, scope) → []Reference` | link guard, save validation, delete guard, cut, verify-git |
| 5 | What is entity E of project Q at this selector? | each slice's `Read(ctx, Selector)` | views, pickers, search, resolver, generators, exports, forks |
| 6 | Cut a release | `release.Cut(project, version)` | HTTP, CLI |
| 7 | Release state: current, withdrawn | `release.SetCurrent`, `release.Withdraw` | HTTP, CLI, scope (reads only) |
| 8 | Is this a valid version name? | `version.Parse` | cut, link guard, restore, format |
| 9 | How is a pre-rule release read? | `legacy.Reader` | closure (when a release carries a flag) |
| 10 | What violates the rules today? | `audit.Run(instance)` | CLI, CI |

### 1. Scope

Input: the request (project, `?version=`, caller). Output: `Draft` or
`Release(v)`, or an error. It owns: the default for each kind of caller, the
current-release pointer lookup, 404 for a version the project never released,
405 for a mutation under a release, and the rule that a lookup failure is an
error. Nothing below it reads a version from context; the scope is an
argument.

### 2. Closure

The centre of the design.

```
Closure {
    Root      ProjectID
    Scope     Scope
    Members   []Member            // closure order: root, primary parent first, then link order, breadth first
    Ontologies map[OntologyID]Revision
    Bindings  []NamespaceBinding  // resolved, in precedence order
}
Member { Project ProjectID; Selector Selector; Via []ProjectID; Readable bool }
Problem { Code string; Paths [][]ProjectID; ... }   // cycle, depth, project-version-conflict,
                                                    // ontology-version-conflict, missing-release, draft-in-closure
```

- For a **draft** scope it walks live links; a pinned member's own links come
  from that member's release lock (R3).
- For a **release** scope it reads the lock stored with the release. It does
  not walk.
- It **always returns problems instead of hiding them**: no depth truncation,
  no skipped cycle edge, no arbitrary winner in a diamond.
- Everything that depends on the chain is derived here and nowhere else:
  the selector per project (so a receipt's version, A2), the ontology set
  (O1, O2, O4), namespace bindings (O5), visibility through links (I7), and
  the order the primary parent implies (L7).

Who does what with problems:

| Caller | On a problem |
|---|---|
| link guard, ontology link write | refuse the write (409) |
| release cut | refuse the cut |
| draft read | serve what resolves, attach the problems to the response |
| release read | none possible for a release cut under these rules; flagged legacy releases go to the legacy reader |

This is how L10 works without cascading checks: a pinned closure is validated
when it is written and cannot change afterwards; a closure with a
draft-following link can change under the project, so it is validated every
time it is resolved.

### 3. Link guard

The only writer of parent links. One call takes the intended change (add,
re-pin, remove, reorder, set primary), then:

1. checks access to the parent (L2) and that the target release exists and is
   not withdrawn (L1, L3);
2. resolves the closure as it would be after the change and refuses on any
   problem (L4, L5, O2);
3. computes the reference set (point 4) against the new closure and refuses
   on missing targets (L6);
4. writes the link, adopts the parent's categories when adding (A8), re-validates
   examples and returns their report (X4).

There is no second path: the legacy single-parent column is gone (L8), the
store's upsert is not callable from outside, and CLI, importers and restore
call the same function. Restore may pass a "report, do not refuse" mode
(K8); that mode is an argument of this function, not a different function.

### 4. Reference set

One function lists everything a project points at in other projects:
receipts, overrides on inherited entities, placements, value targets (model,
collection, concept list), forked-from records, example values. Each
reference is checked against the closure: is the source a member, does the
entity exist at its selector, is it deprecated there.

Used by: the link guard (L6), save validation (A4, A5), the delete guard run
in reverse (R10: who references this project, in drafts and in releases), the
cut (R5), verify-git (K6), the audit (M1). A new kind of reference is added
here once and all of them see it.

### 5. Entity reader

Each slice exposes reads that take a selector `(project, draft | release v)`
and nothing else. "Own content at the request's scope" and "another
project's content at its link's version" are the same call with a different
closure member. No reader joins another project's live table directly; the
resolver, pickers, generators and example builder ask the reader of the
owning slice.

The ambient version accessor and the read allowlist in `pkg/conformance`
shrink to zero as slices convert; conformance then forbids a query on a
versioned table that takes no selector.

### 6. Cut

One transaction under the project's lock: resolve the closure, refuse on
problems or any draft selector, write the own-content snapshot (R2) and the
closure lock (R3), set the current release unless told not to (R9). Nothing
else inserts into release tables; conformance enforces it. Backfills are
migrations with their own allowlisted entry and an audit line (R7).

### 7. Release state

Current release and withdrawal are two fields with one writer each. Scope
(point 1) and pickers read the current pointer; the link guard reads the
withdrawn mark. No query orders releases by version or creation time.

### 8. Version grammar

One parser for the semver subset that is also a valid git tag. The cut, the
link guard, restore and the format schemas use it.

### 9. Legacy reader

Releases cut before a rule cannot be made to satisfy it (principle 3). Each
such release carries flags set by the audit (`chain-unpinned`,
`closure-ambiguous`, `receipt-off-selector`, `no-defaults-link`). The closure
resolver hands a flagged release to the legacy reader, which contains the old
behaviour for exactly those flags. New code paths contain no fallback. When
no instance has a release with a given flag, that branch is deleted.

### 10. Audit

A read-only command that runs the rules over an instance and lists
violations per rule id. It uses points 2 and 4, so the audit and the
enforcement cannot disagree. It runs before a rule is switched on, and in CI
against fixtures afterwards.

## What disappears

| Today | Replaced by |
|---|---|
| chain walks in the release CTE, two `listParentTargetsDraft`, the adoption pin lookup, `ProjectChain`, `ResolvedOntologyVersions`, the bindings query, path audit | `closure.Resolve` |
| receipt `source_version` chosen by writers, with fallbacks | derived from the closure |
| legacy parent column and its sync path | link rows only |
| "latest" by highest number and by creation time | current-release pointer |
| ambient project version in context | scope argument |
| per-handler adoption and removal checks | reference set |

## Database and git say the same thing

| Concept | Database | Git |
|---|---|---|
| parent link | link row (mode, version) | `pletka.mod` requirement (tag or branch) |
| closure lock of a release | closure rows keyed by (releasing project, version) | `pletka.sum` + `vendor/` at the tag |
| release | release row + archive rows | annotated tag `v<version>` |
| current, withdrawn | fields on the release | `releases/index.yaml` |
| receipt | row without version | `adoptions/` file without version |
| fork | row with copied version | `forks/` file with version |
| ontology revision | version + hash | module at tag, hash in `pletka.sum` |
| closure resolution | `closure.Resolve` over rows | the same function over `pletka.mod`/`pletka.sum` |

The last row is what "prepare for git as primary storage" means here: the
resolver has one implementation of the rules and two sources of links. When
verify-git and the server give the same closure for every project, the
database becomes a cache of the tree and the switch is a storage change.
