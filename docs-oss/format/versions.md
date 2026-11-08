# Versions: draft, releases, branches

A project's versions are git refs in its repository. There is no other
version store.

| Ref | Is | Mutable |
|---|---|---|
| `refs/heads/main` | the draft | yes, every save is a commit |
| `refs/tags/v<version>` | a release | no |
| `refs/heads/release/<name>` | work on an old release, cut from its tag | yes |

**[differs]** Today the draft lives in the database and is mirrored to the
repository's default branch (created by a plain `git init`, so its name
depends on the host's git config); releases are rebuilt from archive tables
onto a separate history `refs/heads/releases`, and tags point there. Under
this spec a release tag points at a commit on `main` (or on a release
branch), so `git diff v0.1.0 main` is the change since the release.

## Releases

A release is an annotated tag `v<version>` on the commit whose tree is the
released content.

- `<version>` is any string of `[0-9A-Za-z.+-]` starting with a digit:
  `0.1.0`, `2.0`, `1.0.0-rc.1`. Semver is recommended, not required;
  versions are not ordered by tools.
- Tag message: the release title, a blank line, the description.
- Tagger: the person who cut the release; tag date: when.
- A tag is never moved or re-cut. A wrong release is followed by a new one
  and archived.
- Before tagging, the tree MUST pass `verify-git` with its closure resolved,
  and no requirement may use a branch selector.

### `releases/index.yaml` — [schema](schema/releases-index.schema.json)

The catalog of releases with what a tag cannot hold: archiving.

```yaml
schema_version: 1
releases:
  - version: 0.1.0
    tag: v0.1.0
    title: First release
    description: ""
    created_at: "2026-07-23T10:00:00Z"
    created_by: example-user
    archived_at: ""
    archived_message: ""
```

- Lives on `main` and is updated when a release is cut or archived.
  **[differs]** Today it lives only on the `releases` history.
- It is not part of the module tree hash ([`modules.md`](modules.md)), so
  archiving a release changes no content hash.
- Archiving is a registry-style yank: an archived release stays resolvable
  (existing `pletka.sum` lines keep working) but `pletka mod get` refuses
  it as a new requirement. **[not built]**

## Release branches

Editing a released version without taking the draft: branch
`release/<name>` from its tag, commit, tag the next release on that branch.
**[not built]** — no tooling or UI; plain git works.

## What a dependency sees

A child requiring `P 0.1.0` reads P's tree at tag `v0.1.0`. A child
requiring `P main` reads P's current draft. Moving a child to a new parent
release is one `pletka.mod` line plus a re-verify; the review is
`git diff v0.1.0 v0.2.0` in P's repository.

## Relation to the database

The database holds operational state (users, organisations, memberships,
sessions, keys, usage) and, until git is the store, a copy of the draft and
of release archives. Where they disagree, the repository is the reference
this specification describes, and project content moves out of the
database ([`DECISIONS.md`](DECISIONS.md) D1). **[not built]**
