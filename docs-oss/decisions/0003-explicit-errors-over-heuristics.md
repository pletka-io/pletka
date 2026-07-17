# ADR-0003: Explicit context over runtime heuristics

Date: 2026-06-27
Status: Accepted

---

## Why this decision was needed

Several generators and importers inferred context they were never given —
deriving an ontology namespace from a class's local name, guessing a prefix from a
string shape, resolving an entity's kind by parsing its identifier. These
heuristics passed in the common case and failed silently at the edges: wrong
output with no error, discovered only when downstream integration broke. The
trigger was the Arches generator's namespace handling, where ~200 lines of
prefix-guessing produced subtly wrong IRIs for inputs that didn't match the
assumed pattern (namespace work, 2026-06-27).

## What we decided

Code that needs context — a namespace, a prefix, an entity type, a scope — must be
**given that context explicitly** and **fail loud** when it is absent. It must not
infer the context from a name, a string prefix, or a "usually means" shape.
Resolution happens once, upstream, where the context is known; the result is
passed down as an immutable snapshot. Renderers and leaf code receive resolved
context and guess nothing. A missing input is an error returned at the boundary,
not a silent fallback.

## What we considered and rejected

- **Keep the heuristics, add more special cases.** Rejected: each patch narrowed
  the failure window without closing it. A heuristic that is right 95% of the time
  still emits wrong output 5% of the time with no signal — the worst failure mode,
  because nothing tells you it happened. More cases means more silent edges, not
  fewer.
- **Heuristic with a logged warning on the uncertain path.** Rejected: a warning
  in a log nobody reads is not failing loud. The output still ships wrong. If the
  context is genuinely required for correct output, its absence must stop the
  operation, not annotate it.
- **Resolve context lazily inside each renderer/node.** Rejected: it scatters the
  same resolution across every node, invites drift between them, and makes
  renderers do I/O. Resolving once into a snapshot keeps renderers pure and gives
  one place to get the resolution right.

## Consequences

- **Easier:** failures surface at the boundary where the context was supposed to
  arrive, with a real error, instead of as corrupt output found weeks later.
  Renderers become pure functions of a snapshot — no DB calls, no guessing,
  trivially testable.
- **Harder:** callers must thread real context down to where it's used. The
  upstream resolver (e.g. the snapshot-based namespace manager) is more code than
  an inline guess, and new call sites must supply the context rather than relying
  on inference.
- **Constrained:** new generators, importers, and resolvers may not reintroduce
  name/prefix/type guessing as a convenience. If a value is needed, it is a
  required input. This shapes the generator architecture: a renderer receives a
  fully resolved immutable `Snapshot` and never reaches back to the database.

## References

- Related ADRs: [ADR-0001](0001-weave-module-shapes.md) (slice boundaries),
  [ADR-0002](0002-canonical-node-identity.md) (derived identity, no guessing).
- Architecture: [`../architecture/errors.md`](../architecture/errors.md),
  [`../principles.md`](../principles.md).
