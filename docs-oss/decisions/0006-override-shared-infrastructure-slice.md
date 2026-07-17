# ADR-0006: Override is a shared infrastructure slice

Date: 2026-07-06  
Status: Accepted

---

## Why this decision was needed

Peer-slice isolation is stated as a flat rule: no full slice imports another
full slice's concrete types. In practice `field`, `model`, `collection`, and
`project` import `override` concretely — `field`'s `Service` holds an
`overrides override.Store` field (`field/service.go:56`), `model`'s and
`collection`'s services hold an `overrides *override.Service` field
(`model/service.go:67`, `collection/service.go:54`), and `SaveOverrides`
methods return `override.Diff` — so the field/model/collection write paths can
persist or resolve the base override in the same transactional boundary as the
entity write. The flat rule as stated would forbid this; the code needs it.
Left undocumented, the exception reads as an unenforced violation instead of a
deliberate boundary, and future slice work could either "fix" it into an
unnecessary reader interface or cite it as precedent for importing other peer
entity slices concretely.

## What we decided

`override` is shared infrastructure, not a peer entity slice, even though it is
implemented as a full slice (it owns the `overrides`/junction tables and has
its own Store and Service). Peer entity slices — `field`, `model`,
`collection`, `project`, and any future entity slice — may import
`override.Store`, `*override.Service`, and `override.Diff` directly. The
peer-entity prohibition itself is unchanged and stays hard:

> No peer entity slice imports another peer entity slice (e.g. `field` must
> never import `model`). Shared infrastructure slices (`override`) may be
> imported directly.

The exception is scoped to `override` alone.

## What we considered and rejected

- **Per-consumer reader interfaces** (each of `field`/`model`/`collection`/
  `project` declares its own narrow `OverrideReader`/`OverrideWriter`
  interface instead of importing `override` concretely): rejected because the
  surface `override` exposes to these consumers isn't narrow. Write paths need
  `Store`-shaped CRUD on the base override row, and `SaveOverrides` callers
  need `Diff`-shaped change reporting. A reader interface earns its keep when
  it trims a wide dependency down to the few methods a caller actually needs;
  here the callers need most of what `Store`/`Service` already offer, so the
  interface would just restate them with an extra layer of indirection and no
  isolation benefit.
- **Demote `override` into `pkg/domain`** (treat it as a shared domain type
  rather than a slice): rejected because `override` owns its own table
  (`weave_field_overrides`) and its own Store/Service — it has the shape and
  lifecycle of a full slice, not a domain-level value type. Moving it into
  `pkg/domain` would strand its data-access code outside the slice layout
  convention for no structural gain, and `pkg/domain` types are not expected
  to own tables or issue queries.

## Consequences

- **Easier:** the constructor pattern already used by `field`, `model`,
  `collection`, and `project` (a typed `overrides override.Store` or
  `overrides *override.Service` dependency) is documented as correct instead
  of looking like undetected drift.
- **Easier:** future entity slices that need to read or write base overrides
  can follow the same pattern without inventing a reader interface first.
- **Harder:** the peer-isolation rule now has one named exception to remember
  and enforce. Code review must still catch a *second* exception (e.g. `field`
  importing `model` "because it's convenient") — only `override` is exempted.
- **Constrained:** no other peer entity slice may claim shared-infrastructure
  status without its own ADR. A future package that wants the same treatment
  needs its own decision record, not silent extension of this one.

## References

- Related ADRs: [ADR-0001](0001-weave-module-shapes.md) (two module shapes
  inside `pkg/weave/` — the peer-isolation rule this ADR narrows).
- Architecture: [`../architecture/slices.md`](../architecture/slices.md) —
  states the narrowed rule this ADR justifies.
- Evidence: `pkg/weave/field/service.go:56`, `pkg/weave/model/service.go:67`,
  `pkg/weave/collection/service.go:54`, `pkg/weave/project/override_write.go`.
