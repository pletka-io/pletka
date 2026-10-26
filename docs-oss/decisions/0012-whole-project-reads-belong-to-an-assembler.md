# ADR-0012: Whole-project reads belong to an assembler, not a slice

Date: 2026-10-23  
Status: Accepted; amended 2026-10-26, implementation incomplete

---

## Why this decision was needed

[ADR-0010](0010-versioned-reads-through-a-scope-bound-reader.md) describes one
read: a caller wants an entity, or a filtered page of them, at a named scope.
That is the right shape for a handler rendering a list or an MCP tool fetching
one entity. It is the wrong shape for the consumers that want a project *whole*
— the git materializer writing a project's entire tree, a generator emitting
Turtle, JSON-LD, SHACL, X3ML or CSV for a whole project, `publication`
comparing one whole project state against another.

Those consumers had no shared abstraction, so each implemented the
live-versus-archive decision itself, and the decisions diverged:

- The materializer writes the whole tree **twice**, through ten paired
  functions (`writeProjectTree`/`writeProjectTreeVersion` and nine step pairs).
  The step sequences are identical, eleven for eleven; the bodies share no code.
  They did not even agree on safety — the draft path guards against page-size
  truncation and the release path has no such guard.
- Its release-path project manifest wrote **no namespace map at all**: the
  release cut archives global bindings deliberately
  (`WHERE project_id = $1 OR project_id IS NULL OR project_id = ''`) and the
  read filtered them straight back out with `WHERE project_id = $1`. Ontology
  and global prefixes carry no `project_id`. Every generator needs them to emit
  a prefixed serialization, and restore reads the map back.
- `generators` holds six narrow reader interfaces of which exactly one takes a
  scope, and says so itself: *"generators has no scope of its own yet: exports
  are draft-only"* (`pkg/weave/generators/service.go:356`, `:490`).

The conformance ratchet cannot see any of this: it parses
`<slice>/service.go` only (`pkg/conformance/readscan.go:60`). Five packages
outside the slices hold their own archive queries across fourteen files.

## Amendment: assemble through a bound session (2026-10-26)

This amendment supersedes the free-standing `(scope, projectID)` API and the
consumer-equivalence claims below. The assembler receives the project-bound read
session of ADR-0010; it cannot combine independently chosen versions and project
IDs. Its readers share one database snapshot, and released foreign dependencies
come from the pins required by ADR-0011. Serializers receive domain values and
have no database access.

Archive ownership and dependency pinning precede claims of reproducibility.
Transitional query adapters must be private, transaction-bound, registered and
scheduled for removal. They cannot retain a pool or independently resolve latest.
The assembler consumes dependency resolution; it is not the only place where a
foreign read can occur (interactive reads and refs also cross projects).

Define coverage against the domain and restore contract before replacing writers:
project metadata, inheritance, fields, models, collections, categories, overrides
and refs, examples, vocabularies/concepts, placements, credits, ontology artifacts,
namespace bindings and aliases. Existing tree parity excludes adopted bodies and
omits domain data; its immutability subtest is skipped. It proves only the covered
subset. Retain per-slice tests and add complete snapshot equivalence plus edits,
deletes and new source releases after a cut.

Whole-project materialization and exports share this assembler. Entity exports
may assemble a smaller dependency graph through the same session. Publication's
batch dirty-state queries have timestamp semantics; replacing them with a full
snapshot diff is a separate behavior and performance decision, not an automatic
consequence of this ADR. Search remains a separate read shape.

This is the intended architecture, not a claim that the assembler already exists.
The original rationale follows, with the qualifications above taking precedence.

## Original decision (2026-10-23; qualified by the amendment)

There are **two read shapes**, and they get different abstractions.

1. **Interactive** — filtered, paginated, one entity or one page. The slice
   `Reader` of ADR-0010. Unchanged.
2. **Whole project** — one assembler, parameterized by scope:

```go
func Snapshot(ctx context.Context, scope auth.ReadScope, projectID string) (*ProjectSnapshot, error)
```

Draft is a scope like any other, so there is **one implementation and one place
that decides which tables answer**. The git materializer, the generators and
`publication` consume the snapshot; they do not query the archive themselves.
The assembler reads through slice `Reader`s where the slice has converted and
through the queries the materializer already owns where it has not, so it does
not serialize behind the remaining conversions.

Two rules follow from what a snapshot means:

- **Foreign content is carried as its source published it**, not normalized into
  the asking project's release. The status rewrite a release cut applies to a
  project's own entities does not apply to adopted content; otherwise the
  adopting project republishes its source's work under its own status instead of
  citing it.
- **The assembler resolves each adoption once.** It is the only moment that sees
  every adoption in a project, which makes it the place ADR-0011's rule is
  applied.

The slice contract gains the corresponding red flag: a whole-project or
export-shaped read **on a slice** has taken on the assembler's job.

## Original alternatives (2026-10-23; qualified by the amendment)

- **Thread a scope through each consumer's existing readers** (add
  `auth.ReadScope` to all six generator reader interfaces and to the
  materializer's loaders): rejected because it multiplies the decision instead
  of removing it. Every reader method would independently decide
  live-versus-archive, which is the state that produced the divergences above —
  the per-slice shape makes the count of deciding sites go *up* for these
  consumers, not down. It would have been *better* at one thing: no new
  abstraction, so nothing to learn and no intermediate object to keep correct.
- **Leave the materializer alone because it has its own versioned loaders.**
  This was the previously recorded position and it is rejected here. Having a
  second implementation of the read model is the reason to include it, not the
  reason to exempt it: the two implementations disagreed, and the disagreement
  reached released artifacts.
- **Make the assembler the only read shape** and drop the slice `Reader`:
  rejected because a handler rendering page one of a filtered list would load
  the whole project to do it. The two shapes have genuinely different access
  patterns and both earn their keep.
- **Extend the conformance ratchet to every package instead** (teach it to scan
  beyond `<slice>/service.go`): rejected as a substitute, not as an idea. It
  would report the duplication without removing it, and the duplication is the
  defect. A wider scanner is worth having once the consumers share one input.

## Original consequences (2026-10-23; qualified by the amendment)

- **Easier:** exports become version-aware because their input already is, and
  a pick resolving to a foreign draft becomes unrepresentable rather than
  guarded against.
- **Easier:** enforcement collapses. Instead of a differential test per slice
  per method, one statement covers every whole-project consumer — undo the
  cut's status rewrite and `Snapshot(draft)` immediately after a cut must equal
  `Snapshot(release(v))`. This was measured before the decision was taken: a
  draft tree and a release tree already produce the same file set, byte-identical
  once the status rewrite is undone, for everything except adopted entities. The
  paired writers are duplication, not divergence, which is what makes the change
  a refactor.
- **Easier:** `publication`'s job becomes `diff(Snapshot(draft), Snapshot(release(v)))`.
- **Harder:** a snapshot is a whole project in memory. That is acceptable at
  current project sizes and it is a real ceiling to remember.
- **Harder:** one more layer between a consumer and the data, and a consumer
  that genuinely needs one entity must not reach for a snapshot to get it.
- **Constrained:** `search` is explicitly **not** in scope. Its archive reads
  answer selector queries across projects and are not a whole-project shape.
  Saying so stops the assembler being read as the answer to every archive read.
- **Constrained:** no consumer of whole-project data may add its own archive
  query once the assembler exists, and no slice may grow a `ProjectTree`- or
  `ExportView`-shaped method.

## References

- Related ADRs:
  [ADR-0010](0010-versioned-reads-through-a-scope-bound-reader.md) (the
  interactive shape, whose `Reader`s the assembler consumes),
  [ADR-0011](0011-cross-project-reads-resolve-at-the-source-release.md) (the
  rule the assembler applies once per adoption),
  [ADR-0002](0002-canonical-node-identity.md) (the canonical identity the
  serialized output depends on),
  [ADR-0004](0004-namespace-resolution-boundary.md) (the namespace map whose
  loss is cited above).
- Architecture: [`../architecture/slices.md`](../architecture/slices.md)
  ("Versioned slices").
- Evidence: `pkg/service/gitmaterializer/versioned_tree.go:14`
  (`writeProjectTreeVersion`, against `writeProjectTree` in
  `project_manifest.go`'s neighbourhood — identical step sequences, no shared
  code); `pkg/service/gitmaterializer/versioned_loaders.go:300`, `:335`, `:362`,
  `:396` (adopted loaders reading the source's live tables when an adoption
  records no version); `pkg/weave/generators/service.go:356`, `:490`;
  `pkg/conformance/readscan.go:60`.
