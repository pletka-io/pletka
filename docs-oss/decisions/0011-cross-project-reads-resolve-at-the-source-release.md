# ADR-0011: Cross-project reads resolve at the source project's release

Date: 2026-10-23  
Status: Accepted; amended 2026-10-26, implementation incomplete

---

## Why this decision was needed

[ADR-0010](0010-versioned-reads-through-a-scope-bound-reader.md) decides *which*
scope a request reads at. It says nothing about *whose* data that scope may be
pointed at, and cross-project reuse — one project adopting another's fields,
models or collections — is a design goal here, so that silence was load-bearing.
It produced wrong answers in shipped code twice, in the same direction.

Originally `auth.Release("")` returned `Draft()`. That policy belongs at a request
boundary, where a successfully resolved empty version may mean the working
state. Applied to a *foreign* project it is wrong, and two converted call sites
applied it there: an adoption with no recorded source version resolved to
`auth.Release("")` and therefore read the source project's hot tables, and an
export resolved a picked collection at `auth.Draft()` outright. One project read
another's unpublished working state — the wrong side of a visibility boundary,
not merely stale data. Two allowlist exemptions had also been written claiming
the question was structurally unanswerable.

## Amendment: pin the dependency closure (2026-10-26)

This amendment supersedes the deferred project binding and "no schema change"
claims below. Every content reader binds its project, as amended in ADR-0010.
Caller-owned reads use that caller's selected draft/release; only foreign reads
resolve a source release. A foreign reader is obtained through a dependency
resolver, not by retargeting an ordinary scope.

A draft session may resolve an unpinned source to its latest release once. A
released project must use the concrete source identities recorded during its cut,
never latest-at-read-time. Resolve the transitive closure in the cut transaction,
including inheritance, adoptions, refs without adoption rows, export picks,
ontology versions, defaults and aliases. Reject missing/unpublished sources,
unresolved pins and follow-source-draft dependencies when cutting a release.
Explicit draft-following may be supported for draft workflows; it cannot become
an immutable release dependency.

Archive ownership must be explicit. Migration 021 replaces the earlier copying
of parent metadata under a child's version and shared bindings under globally
colliding keys. New namespace captures belong to `(release_project_id, version)`;
new project/inheritance/ontology captures contain only the releasing project's
rows. Source metadata is read at the recorded source version, with no live
fallback. Unverified history remains unavailable and has a read-only audit.
Overwriting collisions would mutate history and is forbidden. Historical
membership may be unreconstructable; current drafts cannot fabricate it.

These are required write-path and schema changes, not merely a new latest-release
query. An assembler consumes the recorded dependency graph; it does not infer or
repair it at read time. Ownership and the collection/override bound-session pilot
are implemented; complete dependency resolution, closure and historical
verification remain pending. `auth.Release("")` and whitespace-only versions are
now invalid. Only `auth.ScopeForResolvedVersion` deliberately translates a
successful empty request-policy result to draft. This removes the implicit
foreign-draft substitution; it does not supply missing source pins.

## Original decision (2026-10-23; qualified by the amendment)

> A read of another project's content resolves at **that project's** release.
> When the adoption records a version, that version. When it records none, the
> source project's **latest release**. Draft is never a cross-project answer.

Draft is the odd one out, and naming why is the justification. A draft is one
authenticated user's working state inside their own project. It is not a
version; it is the absence of one — the thing a release is cut *from*. Release
content is what a project has published and therefore what it has offered to
others. So `release(v)` and `latest release` are cross-project answers and
`draft` is not, by construction rather than by policy.

A source project that has published nothing is an **error**, not a fallback: the
adoption renders as unavailable. This is why the resolution returns
`(ReadScope, error)` and not a bare scope — `LatestReleaseReader` returns `""`
for a project with no release (`pkg/auth/project_version.go:17`), and
`Release("")` originally meant `Draft()`, so the naive one-line composition
reproduced exactly the defect this rule removes. It now fails closed instead.

Reading a source's draft deliberately is a **third kind of scope**
(follow-source-draft), reached only by explicit configuration. It is a
legitimate, explicitly-chosen state, not an accident the type should make
unreachable.

## Original alternatives (2026-10-23; qualified by the amendment)

- **Resolve the foreign entity at the asking project's version** (project A
  reads B's rows at A's release tag): rejected because it is incoherent against
  the schema. Archive primary keys are `(id, version_number)` and are *global*
  across projects, while version strings are *per-project* and releases archive
  `WHERE project_id = $1` — own rows only. So resolving a foreign entity at the
  asking project's version either misses silently or hits an unrelated
  same-named release in another project. This was the premise behind both
  mistaken exemptions.
- **Fall back to the source's draft when no version is recorded** — what the
  code did. Rejected because it crosses a visibility boundary: the source's
  draft is not content the source has offered anyone. It would have been
  *better* at one thing — an adoption always resolves to something, so nothing
  renders as unavailable — and that convenience is exactly what hid the defect.
- **Record the exemption as permanent** ("a per-project archive cannot answer a
  cross-project question", "this needs a write-path and schema change"):
  rejected because it is false. Each foreign project answers at its own release,
  and the resolution is an existing latest-release CTE without its
  single-project predicate — the semantic-version ordering it needs is already
  expressed in SQL (`pkg/weave/publication/publication.go:108`). No schema
  change is required.
- **Bind every scope to the project it was minted for** and refuse a read whose
  target project differs: attractive, because it makes the rule unrepresentable
  rather than remembered, and partially adopted. Deferred in the original decision (superseded by the amendment) because `Reader` methods take `projectID` as a per-call argument rather
  than fixing it at `At(scope)`, so total enforcement is a signature change
  across every converted slice. The binding applies to scopes minted by the
  cross-project resolution; the general case is revisitable when the last slice
  converts.

## Original consequences (2026-10-23; qualified by the amendment)

- **Easier:** a cross-project read has one answer, derived once, instead of each
  slice re-deriving it. Five slices still to convert all have adoption reads
  that would otherwise re-litigate it.
- **Easier:** two exemptions written on the rejected premise become convertible
  rather than permanent.
- **Harder:** "latest release" is a *moving* target. A release of the asking
  project that resolves a source at read time is not immutable — it answers
  differently after the source publishes again. Pinning the resolved version
  when the asking project's release is cut is what makes a released artifact
  reproducible, and it is the companion change to this rule, not an optional
  extra.
- **Harder:** an adoption whose source has never published now renders as
  unavailable where it previously rendered stale content. That is correct and it
  is a visible behaviour change.
- **Constrained:** foreign resolution goes through the dependency boundary,
  including source ownership, pin availability and authorization. A strict
  `auth.Release(recordedVersion)` prevents an empty-to-draft substitution but
  does not perform those other checks.
- **Constrained:** follow-source-draft cannot be implemented until its
  configuration point is decided — per adoption, per project, or per request.
  Only a per-adoption record travels with the data, which is what a reproducible
  export requires.

## References

- Related ADRs:
  [ADR-0010](0010-versioned-reads-through-a-scope-bound-reader.md) (which scope
  a request reads at — this ADR answers whose data it may be pointed at),
  [ADR-0012](0012-whole-project-reads-belong-to-an-assembler.md) (the one place
  that sees every adoption in a project, and so the natural place to apply this
  rule).
- Architecture: [`../architecture/slices.md`](../architecture/slices.md)
  ("Versioned slices").
- Evidence: `pkg/auth/read_scope.go` (strict `Release` constructor and explicit
  request-policy adapter),
  `pkg/auth/project_version.go:17` (`LatestReleaseReader` returns `""` for a
  project with no release), `:63` (`ResolveEffectiveVersion` — the same
  resolution *with* the draft fallback, which is correct at a request boundary
  and wrong across projects), `pkg/weave/publication/publication.go:108` (the
  latest-release CTE this rule generalizes).
