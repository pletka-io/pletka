# Ontology autocomplete engine

`pkg/weave/ontology/autocomplete/` answers one question, over and over, on
every keystroke of the path-builder UI: *given the path so far, what can
come next?* It backs the frontend's `RichPathBuilder.svelte` /
`SimplePathBuilder.svelte` widgets
(`frontend/src/lib/components/form/widgets/path-builder/`) and the
`OntologyLabels` lookup used by breadcrumbs and display renderers. It
replaced an older regex-heuristic classifier; every class/property/literal
distinction now comes from a Store lookup against `weave_ontology_classes` /
`weave_ontology_properties`, never from pattern-matching a name.

This is a load-bearing subsystem with no prior design note — this page is
that note.

## Two engines behind one seam

Both engines satisfy the same one-method interface:

```go
type suggester interface {
    GetSuggestions(ctx context.Context, req Request) ([]Suggestion, error)
}
```

- **`DirectEngine`** (`engine.go`) — composes `Store` + `ProjectReader` and
  answers every request with live DB round-trips: `GetClassByQname`,
  `PropertiesForDomainQname`, `ListRelationsForSourceByType`, walked fresh
  each call. Lineage (`classLineageQnames`) and subclass expansion
  (`collectSubclasses`) both iterate **every linked ontology version**, not
  just one — a qname's `subclass_of` parents can be declared in a sibling
  ontology version (e.g. AAAo's `ZE19_Naming` parents to CRM's
  `E13_Attribute_Assignment`), so single-version walks silently truncate
  the chain and drop every property inherited from further up.
- **`IndexedEngine`** (`indexed_engine.go`) — answers from an in-memory
  pointer graph (`Index`) built once and cached. Zero DB round-trips once
  warm. It mirrors `DirectEngine`'s semantics exactly (same lineage rule,
  same literal-suggestion rule for `DatatypeProperty`) but walks graph edges
  instead of issuing queries.

## `DispatchEngine`: which engine answers a request

`dispatch.go:11-19` defines three modes:

```go
const (
    ModeIndexed             Mode = "indexed"               // default
    ModeDirect              Mode = "direct"
    ModeIndexedWithFallback Mode = "indexed-with-fallback"
)
```

`DispatchEngine.GetSuggestions` picks per `modeFor(req)`:

- `ModeIndexed` (default) — always the `IndexedEngine`.
- `ModeDirect` — always the `DirectEngine` (DB round-trips every request).
- `ModeIndexedWithFallback` — tries `IndexedEngine`; on error, logs a
  warning and falls back to `DirectEngine`.

A super-admin can force a single request onto `DirectEngine` regardless of
the configured mode: `modeFor` returns `ModeDirect` only when
`DispatchConfig.AllowPerRequestOverride` is set, `req.SuperAdmin` is true,
**and** `req.Source == "direct"` — all three conditions, not just one. This
is the escape hatch for debugging an index that looks wrong without
changing global config.

`DispatchEngine.Direct()` also exposes the concrete `DirectEngine` for
surfaces that are direct-only by design (`OntologyLabels`, scope
resolution) — they never go through the index.

## `IndexedEngine`: the project-union ontology DAG

### `IndexKey` is the whole contract

```go
type IndexKey struct {
    VersionSet string // sorted version IDs joined by "|"
    Primary    string // primary ontology-version ID
    Lock       string // "live" | "release:<id>" (MVP: always "live")
}
```

`IndexKey` **fully determines** a project's autocomplete answers: two
projects that resolve to the same set of ontology versions, the same
primary version, and the same lock, get the same cached `Index` — the
cache never keys on project ID directly (`keyForRequest`, `index.go:82-89`,
sorts the version-ID list so key order doesn't matter). `Lock` is a
placeholder for a future frozen-release mode; every live request today
uses `"live"`.

### `buildIndex`: two passes, ghost nodes, cross-version edges

`buildIndex` (`index.go:100-209`) materialises the union of a project's
selected + inherited ontology versions into one pointer graph:

1. **Pass 1** — `upsertNode` inserts one `Node` per qname across all
   versions, collapsing duplicates. On a collision, the **primary**
   version's `Meta` (prefix, version string) wins; a `PropertyType` is
   carried onto property nodes so `IndexedEngine` can reproduce
   `DirectEngine`'s literal-detection logic without a DB call.
2. **Pass 2** — walks `subclass_of` / `domain` / `range` relations across
   every linked version and wires `Node.Superclasses` / `Subclasses` /
   `Domains` / `AsDomainOf` / `Ranges` / `AsRangeOf`. A relation whose
   target qname has no corresponding class/property row anywhere becomes a
   **ghost node** — `Node{Qname: ..., Ghost: true}` — so the edge still
   exists (a dangling parent, or a `DatatypeProperty`'s literal range) but
   carries no label/type data. Three separate dedup maps (one per edge
   kind) mean a class that is both a property's domain and its range keeps
   both edges.

`detectCycles` (`index.go:269-287`) logs (never fails) when `subclass_of`
loops back on itself — a malformed import signal; the closure walker
tolerates cycles regardless because it is pointer-keyed and O(graph size).

### `closure`: the shared BFS walker

`closure.go` implements one breadth-first walk used by both suggestion
paths (properties-for-class via `Superclasses`, classes-for-property via
`Subclasses`):

- Returns every reachable node **excluding root** — the caller already has
  root and doesn't want it echoed back as its own answer.
- `Include`/`Cut` are deliberately two separate predicates: `Include`
  filters what's emitted in the *output* (a hidden node's descendants can
  still surface); `Cut` prunes the *walk* (nothing past a cut node is ever
  visited). Collapsing them into one mask was the bug pattern this split
  exists to avoid.
- The `seen` set is keyed by pointer, so a cycle in the graph terminates
  the walk at O(graph size) instead of looping forever.

## `IndexCache`: singleflight, TTL, epoch-guarded invalidation

`cache.go` owns one `*Index` per `IndexKey`:

- **Singleflight** (`golang.org/x/sync/singleflight`) collapses concurrent
  cold-cache builds for the same key into one `buildIndex` call.
- **10-minute live TTL** (`defaultLiveTTL`) — a `warm()` hit past TTL is
  evicted and rebuilt; release entries (`Lock != "live"`) never expire.
- **200-entry soft cap** (`defaultSoftCap`) — `evictLocked` drops the
  least-recently-accessed entry (linear scan; fine at this scale).
- **Epoch guard against a build/invalidate race**: every invalidation
  (`InvalidateForProject`, `InvalidateForVersion`, `Drop`) increments
  `c.epoch` under `c.mu`. `For()` captures the epoch *before* the
  singleflight build starts; `put()` compares the captured value to the
  current epoch and skips the insert if they differ. The in-flight
  request still gets its freshly built index — it just isn't cached — so
  an ontology import that lands mid-build can never leave a stale `Index`
  sitting in the cache until the next TTL sweep.

### Cache invalidation is event-driven, not polled

`Subscribe(bus domain.EventBus)` (`cache.go:156-167`) wires two handlers at
boot:

```go
bus.Subscribe(domain.EventOntologyVersionImported, ...)        // → InvalidateForVersion(e.EntityID)
bus.Subscribe(domain.EventProjectOntologyVersionsChanged, ...) // → InvalidateForProject(e.ProjectID)
```

An ontology import or a project's version-link change publishes on the
shared `EventBus`; the cache reacts by dropping every live entry whose
version set contains the changed version (or that the project resolved
to) — no cron, no manual `Drop` needed in the common path. `Drop()` remains
as an operator escape hatch, and `Preload(projectID)` warms a project's
index in the background (best-effort; logs on failure, never returns one).

## The path-validation story

There is **no save-time alternation validator**. `pkg/domain/validate.go:24`
checks only that `Field.PathElements` is non-empty — it does not check that
classes and properties alternate correctly, or that each step's
domain/range is honored. Correctness is enforced at two other points
instead:

1. **Build time** — the autocomplete engine *is* the constraint. Because
   `GetSuggestions` only ever offers properties whose domain includes the
   current class (or its full lineage) and only classes that are the
   property's range (or a subclass of it), a path assembled through
   `RichPathBuilder`/`SimplePathBuilder` cannot go wrong — the next step is
   drawn from a domain/range-filtered suggestion list, not free text
   (`engine.go:142-152` — `DirectEngine.GetSuggestions` doc comment). The
   `PathBuilder` name in older docs refers to this Svelte widget family,
   **not** a validator; it has no independent alternation-checking logic of
   its own.
2. **Audit time** — `weave verify-paths` (`cmd/weave_verify_paths.go`)
   is the after-the-fact check. It walks every `weave_fields.path_elements`
   entry (and legacy `subfield_paths` entries) and confirms each
   non-literal element's stored `type` agrees with which table its
   `prefix`/`local_name` actually appears in —
   `weave_ontology_classes` or `weave_ontology_properties` — against the
   project's linked ontology versions. A row can fail as `malformed`
   (empty local_name), `type-mismatch` (stored as one type, found in the
   other), `missing` (qname not in any linked version), or `ambiguous`
   (qname exists in both tables). It is report-only — nothing is modified
   — and exits non-zero when any error is found, so it's suitable as a
   CI/cron gate:

   ```
   pletka weave verify-paths                              # every project
   pletka weave verify-paths --project LA                  # one project
   pletka weave verify-paths --exclude-standard --format json
   ```

   `--format csv` emits one row per field with a type-mismatch, including
   the raw `path_elements` JSON, for bulk repair.

So: autocomplete constrains what a curator *can* build; `verify-paths`
confirms what already exists still agrees with the ontology it was built
against (versions get re-imported; a class can become a property between
versions). Neither is a save-time gate — there isn't one.

## Where this fits

| Concern | Where |
|---|---|
| Path-builder suggestions (frontend) | `RichPathBuilder.svelte` / `SimplePathBuilder.svelte` |
| Suggestion + dispatch logic | `pkg/weave/ontology/autocomplete/` (this page) |
| Cache invalidation trigger | `domain.EventBus` — `EventOntologyVersionImported`, `EventProjectOntologyVersionsChanged` |
| Path storage | `Field.PathElements` — see [`domain-model.md`](domain-model.md) |
| Post-hoc path audit | `pletka weave verify-paths` (`cmd/weave_verify_paths.go`) |
