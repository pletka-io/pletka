# ADR-0010: Versioned slice reads go through a scope-bound Reader

Date: 2026-10-23  
Status: Accepted; amended 2026-10-26, implementation incomplete

---

## Why this decision was needed

A release is a readable state, not just an audit record: tables with an
`_archive` counterpart can be asked for their rows at a version. The reads that
answer those questions grew one at a time, and each grew its own mechanism — an
optional type assertion onto a capability interface, a paired `Get`/`GetVersion`
method, a version pulled from the request context inside a service. Every one of
them had the same failure mode: a read that *could* name a version but didn't
silently answered from the live tables, so a page or an export rendered under a
release tag could carry draft content. Nothing in the type system distinguished
"this read is at the draft" from "this read never named a scope at all".

## Amendment: bind project and transaction (2026-10-26)

This amendment supersedes the unbound `At(scope)` API below as the target
contract. That API describes the current migration stage, not complete isolation.
A read-session factory must bind project identity, an explicit draft/release
selection and a transaction before exposing slice readers. Reader methods cannot
retarget another project. Zero/empty release selections fail; the existing
`Release("")` compatibility behavior must not be used as a strict constructor.

The factory owns database access and reader lifetime. Read-only draft assembly
uses repeatable read. Release cuts use one repeatable-read write transaction for
release metadata, archive contents and outbox. Request authorization stays in
`pkg/auth`; database session construction is wired by `pkg/app`.

An archive reader must receive narrow archive query capabilities, not the entire
write store, pool or unrestricted generated query set. Current collection and
override readers do retain these paths; a scope parameter alone does not prevent
live-table access. Existence checks for released content also use the bound
reader, so deleting a live entity does not make its archive unreadable.

Mutations write to the live destination. Clone/adopt source reads may target a
release; they must not be forced to draft merely because the operation writes.
Authorization is independent of content selection. Latest-release lookup errors
stop the request; a successful lookup finding no release is a distinct policy
case. HTTP, page, visualization and MCP boundaries share that resolution.

Enforcement needs an explicit monotonic conversion registry, checks of all
production files and reader capabilities, and actual-cut tests that mutate and
delete live data afterward. The current signature ratchet and parity tests alone
cannot establish isolation. The implementation of this stronger boundary is
pending; the bullets below describe the earlier migration shape.

## Original decision (2026-10-23; qualified by the amendment)

A slice owning an `_archive` table is a **versioned slice**, and its reads live
on a scope-bound `Reader`:

- `Store` keeps writes plus `At(scope auth.ReadScope) (Reader, error)`. Reads
  live on the `Reader` and nowhere else.
- `auth.ReadScope`'s zero value is **invalid**, and `At` fails closed on it
  (`pkg/weave/collection/scoped_reader.go:147`). The zero value means a caller
  never named draft or a release, which is a bug at that call site rather than a
  default to be filled in.
- A `Reader` bound to a release reads the archive and **never falls back** to
  the live tables. A release that archived nothing answers "not here".
- Scope is resolved **once at the request boundary** — an HTTP handler or an MCP
  tool — and passed inward. `auth.ReadScopeFromContext`
  (`pkg/auth/read_scope.go:85`) returns the invalid zero value when no scope is
  present, deliberately, so a service that reaches for ambient scope fails
  instead of guessing.
- **Mutations stay hot structurally**: they sit on `Store`, take no scope, and a
  clone or remap path passes `auth.Draft()` explicitly so reading the draft is
  visible in the diff.

Opting out is per method, with a written reason, in `pkg/conformance`'s
allowlist. A conformance test fails the build on an unscoped read in a versioned
slice and on an `_archive` table no slice claims.

## Original alternatives (2026-10-23; qualified by the amendment)

- **A wrapper type that resolves the version per call** (a `versionedStore`
  decorating each slice's `Store`, reading the scope from context): this was the
  original plan and it was retired before any slice converted. Rejected because
  it keeps the ambiguity it was meant to remove — a wrapper still has to decide
  what an absent scope means, and the only answers are "draft" (the bug) or
  "error" (which is `At` failing closed, with an extra layer). It would have
  been *better* at one thing: slices convert without signature changes, so the
  migration is cheaper. The cost was that the end state is no safer than the
  start.
- **Optional capability interfaces** (`s.store.(versionedFieldReader)`):
  rejected because an optional type assertion degrades silently. Change the
  interface's signature and the assertion stops matching; the compiler says
  nothing and the read quietly answers from live rows. Capability belongs in the
  type, checked at build time.
- **Paired public methods** (`Get` alongside `GetByIDVersion`): rejected because
  a caller that can choose is a caller that can choose wrong, and the wrong
  choice is the silent one. The versioned variants leave the public surface; the
  scoped reader is their only caller.
- **Version in the request context, read inside services**
  (`auth.ProjectVersionFromContext` at the point of use): rejected because it is
  what the code already did and what produced the drift. An ambient value that
  every read may or may not consult cannot be audited by reading a signature.
- **A nil-safe default scope** (treat the zero value as draft): rejected because
  it reintroduces the original defect as a documented feature. Failing at the
  call that forgot to name a scope is the whole point.

## Original consequences (2026-10-23; qualified by the amendment)

- **Easier:** a read's signature says what it reads. `store.At(scope)` is
  greppable and the conformance test can enumerate every read that is not yet
  behind one.
- **Easier:** a release-scoped `Reader` cannot accidentally serve live rows,
  once the capability separation in the amendment is implemented.
- **Harder:** each slice must write a second query per read against its
  `_archive` table, mirroring the live query's rule including any deliberate
  quirk. The compiler does not check that mirroring. The counter-measure is a
  differential test per converted slice: cut a real release, then require
  `At(Draft())` and `At(Release(v))` to answer alike.
- **Harder:** `ReadScope` is a comparable struct compared with `==` in tests.
  Adding a field to it breaks those comparisons — loudly, which is the intent,
  but it makes the type expensive to extend.
- **Constrained:** no new versioned slice may expose a read outside its
  `Reader`, and no slice may reintroduce a paired `XVersion` method on a public
  surface. Both are named red flags in the slice contract.

## References

- Related ADRs: [ADR-0005](0005-pletka-auth-boundary.md) (`pkg/auth` owns the
  scope type and the request-boundary middleware),
  [ADR-0003](0003-explicit-errors-over-heuristics.md) (fail loud rather than
  guess — the zero value's invalidity is that principle applied to a type),
  [ADR-0011](0011-cross-project-reads-resolve-at-the-source-release.md) (whose
  project a scope may be pointed at),
  [ADR-0012](0012-whole-project-reads-belong-to-an-assembler.md) (the read shape
  this ADR does not cover).
- Architecture: [`../architecture/slices.md`](../architecture/slices.md)
  ("Versioned slices"), [`../architecture/data-layer.md`](../architecture/data-layer.md).
- Evidence: `pkg/auth/read_scope.go:18` (the type), `:38` (`Valid`), `:85`
  (context accessor returning the invalid zero);
  `pkg/weave/collection/scoped_reader.go:147`,
  `pkg/weave/override/store_postgres.go:77`,
  `pkg/weave/category/scoped_reader.go:120`,
  `pkg/weave/example/store_postgres.go:84` (four converted slices);
  `pkg/conformance/scoped_reads_test.go` (the ratchet),
  `pkg/conformance/readscan.go:60` (it reads signatures in
  `<slice>/service.go`, not bodies).
