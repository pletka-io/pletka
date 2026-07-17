# ADR-0002: Canonical node identity for generator output

Date: 2026-05-15
Status: Accepted — amended 2026-05-16 (see Amendment)

---

## Amendment (2026-05-16)

The node-identity work shipped, but not as this ADR first described. Two
parts of the original decision are revised:

1. **UUIDv5 derivation is superseded.** Node identity is generated as a
   readable structural id, not a UUID. Every class path element now
   carries two generated, non-persisted fields (`pkg/domain/path_element.go`):
   - `path_node` — the placement-scoped ancestor chain:
     `{categorySlug}/{collectionSlug}/{ordered qnames}` (or
     `{categorySlug}/__direct__/…` for direct fields).
   - `path_node_id` — a short `{class_code}_{occurrence}` id derived from
     `path_node` in a snapshot-wide pass.
   Generators (X3ML variables, exportgraph/mermaid/cytoscape node keys)
   key off `path_node_id`. The "single load-bearing canonical serialiser"
   idea survives as the `path_node` chain builder in
   `pkg/weave/generators/tree.go`. See commits a4f237c0, 73243d01.

> **Amendment (2026-07-09):** the deferred `weave_collection_placements`
> table now exists — built constraints-first (migration
> 071: `project_id/model_id/category_id/collection_id` + `is_required` /
> `min_occurs` / `max_occurs` / `is_hidden`; bigserial PK, semantic-id
> references, no `position`, no `placement_id` FK on overrides). It records
> the constraints of a collection placed in a model, not stored identity:
> the readable structural ids from the earlier amendment superseded the
> original UUID scheme, so the identity integration sketched below remains
> deferred. Absence of a row means optional / 0..unbounded / visible.
> Gitmaterializer serialization of placements is a follow-up.

2. **The `weave_collection_placements` table is deferred** — and
   reclassified. It is *not* a bug fix. The current schema
   (`weave_field_overrides`, keyed by `category_id` + `part_of_collection_id`)
   and the model-view builder (`pkg/weave/resolve.go`, which groups fields
   into `map[categoryID]map[collectionID]`) **cannot represent the same
   collection placed twice in one category** — two such placements'
   override rows are indistinguishable and merge into one collection
   group. Every `(category, collection)` pair is therefore structurally
   unique, so `path_node`'s placement scope is already stable and
   reorder-safe for all representable data. There is no live identity
   gap. The table is a *prerequisite for a future repeated-placement
   feature*; build it with that feature (naturally alongside the
   export → git → re-import / Arches survival work, also deferred).

What stands unchanged: the layered identity model (semantic ids for
entities, a generated id for everything below a placement, nothing
per-node stored), placement-scoped coreference, and the rejected
alternatives. The "Why" and "Considered and rejected" sections below
remain accurate as the reasoning record.

---

## Why this decision was needed

The generators (X3ML, RDF, SHACL, future Arches) emit graph nodes for every
class along a field's ontology path. Each node needs an identity that is:

1. **stable across export → re-import** — Arches ingests resources by UUID; a
   re-import that produces fresh UUIDs would duplicate every resource;
2. **reorder-safe** — dragging a field or collection in the editor must not
   change any node's identity;
3. **zero-config** — curators build ontology paths; they never see or hand-code
   instance identifiers (the legacy Zellij workflow made them type `4_1`-style
   variables by hand);
4. **correct for coreference** — fields that share a path prefix in the same
   context must land on the *same* intermediate node; the same collection placed
   twice must *not* merge.

The X3ML renderer previously keyed `<entity variable=...>` off the lowercased
class local name (`e33_e41_linguistic_appellation`), which over-merged every
field touching that class into one node. `weave_field_overrides.id`
(bigserial) was considered as the anchor but is reassigned on re-import, so it
fails requirement 1.

## What we decided

Node identity is **derived deterministically** from a single stored anchor — the
**collection/field placement** — plus portable semantic data. Nothing per-node or
per-field is stored.

1. **Placement is now a first-class entity.** A new table
   `weave_collection_placements` records each act of dropping a collection (or a
   directly-placed field group) into a model's category:
   `(id uuid, model_id, category_id, collection_id, position, created_at)`.
   `weave_field_overrides` gains a `placement_id uuid` FK. The `id` is a v4 UUID
   minted once at placement creation and is the only identity that is *stored*
   and carried by the gitmaterializer.

2. **Everything below the placement is computed**, never stored, via UUIDv5
   against a fixed Pletka namespace constant:
   - field-in-placement: `UUIDv5(ns, placementID + ":" + fieldSemanticID)`
   - intermediate path node: `UUIDv5(ns, placementID + ":" + canonicalPrefix)`
     where `canonicalPrefix` is the ordered `prefix:local_name` qnames of the
     path steps from the placement root down to that node;
   - terminal value node: `UUIDv5(ns, placementID + ":" + fieldSemanticID + ":value")`.

3. **The canonical ancestor-chain serializer** is the single load-bearing
   primitive. It produces a byte-stable string from an ordered list of steps
   (`semantic-id` or `prefix:local_name`), joined by a fixed separator, with a
   direction tag when inverse properties are in play. It is versioned: if the
   serialisation ever changes, every derived UUID moves, so changes are an
   explicit, deliberate event.

4. **Spine sharing is scoped to a placement.** Because every node under a
   placement seeds off that placement's UUID, two fields in placement A that
   share a path prefix coreference (same intermediate UUID); placement B of the
   same collection has a different placement UUID, so its nodes never merge with
   A's. This is the "traverse the export tree with shared nodes" behaviour — the
   tree traversal accumulates placement context, and shared-node logic runs
   within a placement subtree, not globally.

`weave_field_overrides.id` (bigserial) keeps its job: it is the runtime key that
lets the tree builder produce distinct subtrees. It never enters a UUID seed.

## What we considered and rejected

- **Seed UUIDs from `weave_field_overrides.id` (bigserial).** Rejected: the
  bigserial is reassigned on every re-import (a fresh `INSERT` into a new
  database), so any UUID derived from it changes across environments — it fails
  the export/import-survival requirement. It is reorder-safe but not portable.

- **Store a UUID on every override row (per-occurrence).** Rejected: it stores
  far more than the irreducible minimum, and a collection placement spans many
  override rows that must all agree on one placement identity — a per-row column
  has no structural guarantee that the rows of one placement share a value. The
  invariant would be enforced only by application code and silently corruptible.

- **Pure deterministic, no storage at all** (key everything off
  `model/category/collection/field` semantic IDs). Rejected: the same collection
  may be placed twice in the *same* category (confirmed allowed). Those two
  placements have an identical `model/category/collection` ancestor chain — the
  structure alone cannot disambiguate them, so at least the placement needs a
  stored handle. Everything *below* the placement, however, genuinely needs no
  storage, and we kept that.

- **`placement_uuid` as a denormalised column on `weave_field_overrides`**
  instead of a `weave_collection_placements` table. Rejected: it would mean less
  migration, but nothing structural enforces "all override rows of one placement
  share the uuid". A real placement row makes the invariant a foreign key, not a
  convention, and gives the gitmaterializer a single clean entity to serialise.
  The table is the slightly larger migration but the honest model — a placement
  *is* a domain entity (it has a model, a category, a collection, a position).

## Consequences

- **Easier:** generators get correct, stable, reorder-safe node identity with no
  per-node bookkeeping. X3ML coreference, RDF blank-node grouping, and Arches
  UUID survival all fall out of one mechanism. The release/version system is the
  natural freeze boundary — within a release the paths are immutable, so derived
  UUIDs are stable; the gitmaterializer stores the placement rows + paths and the
  UUIDs recompute identically on import.

- **Harder:** adding a collection to a model is no longer "insert override rows";
  it must also create a `weave_collection_placements` row and stamp `placement_id`
  on each override. The adopt-collection and add-field handlers need updating,
  and a backfill migration must group existing override rows into placements.

- **Constrained:** the canonical-ancestor-chain serialiser is now load-bearing
  for cross-environment identity. Any change to it is a breaking change to every
  derived UUID and must be versioned and called out explicitly. Editing a field's
  ontology path inside a *draft* will move that field's derived node UUIDs — this
  is acceptable because published releases freeze the path; identity drift across
  releases is correct (a different version of the model is a different node).

## References

- Related ADRs: ADR-0001 (two module shapes inside pkg/weave/)
- Related work: X3ML downloads (variable naming), SPARQL renderer (stable
  middle variable), Arches support context
- Migration: `weave_collection_placements` table + `placement_id` on
  `weave_field_overrides` (052 was the next free migration number at the time
  of writing; migrations have since advanced past it — check
  `pkg/database/migrations/` for the actual next free number)
