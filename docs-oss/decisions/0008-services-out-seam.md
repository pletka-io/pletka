# ADR-0008: Services-out seam for host contributions

Date: 2026-07-19  
Status: Accepted

---

## Why this decision was needed

ADR-0007 established that hosts inject extensions INTO core through explicit
options. Hosting binaries also need the opposite direction: core handing its
assembled slice services OUT, so a host can compose whole features (first
consumer: the commercial MCP server, which lives in the hosting repo) over
core's services without core knowing the feature exists. Route contributions
already mount after slice assembly, so the timing requires no inversion.

## What we decided

`app.Contributions` route contributions receive `Host.Services *app.Services`
— one deliberate struct of core's assembled service singletons (concrete
pointers plus a few narrow interfaces), populated by `app.New` after
assembly, immediately before contributions mount.

Contract:

- read-only composition surface: one instance per slice; contributions never
  construct duplicate services or stores;
- consumers define their own narrow reader interfaces over the concrete
  services (the same discipline generators use);
- every field is non-nil in a real `app.New`; contributions may rely on it;
- additions to `Services` require a composition-catalog entry and review —
  the struct is a curated surface, not a dumping ground.

`app.Services` consumed anywhere except a host's wiring layer is a red flag —
core code never imports it, slices receive narrowed interfaces. This is the
fence that keeps the seam from regressing into a god object.

## What we considered and rejected

- **Widening `Host` with per-feature fields:** rejected because it is the
  same anti-pattern the composition rules already red-flag — a host-specific
  dependency added to the shared `Host` type when only one contribution needs
  it. `Services` is one deliberate field instead of an unbounded set of
  feature-specific ones.
- **A `ConfigureApp`-time callback:** rejected because route contributions
  already run at the right point in assembly (after every slice service is
  built) — a separate callback hook would duplicate timing that already
  exists for no benefit.
- **Exposing the private slice-host structs:** rejected as over-broad — the
  per-slice `Host` types carry routing and mounting concerns a consuming
  feature has no business touching; `Services` exposes only the assembled
  service singletons.

## Consequences

- **Easier:** features can move core↔host with mechanical import changes
  (the promotion path documented in the composition page): the slice
  consumes only exported core APIs; only its wiring differs per home.
- **Harder:** core's public API surface grows — the slice service types
  become load-bearing for hosts. Breaking their signatures is a
  host-visible change.
- **Constrained:** no future contribution may reach into `app.Services` from
  core code or a slice; consumption is confined to a host's wiring layer,
  and every addition to the struct needs a composition-catalog entry.

## References

- Related ADRs: [ADR-0007](0007-host-injection-over-global-registration.md)
  (the inverse direction — hosts injecting into core).
- Architecture: [`../architecture/composition-and-extension.md`](../architecture/composition-and-extension.md)
  — composition catalog entry and promotion-path paragraph this ADR governs.
- Rule: [`../../.claude/rules/composition.md`](../../.claude/rules/composition.md).
- Evidence: `pkg/app/contributions.go` (`Host.Services`, `Services`),
  `pkg/app/app.go` (`app.New`, `mountRouteContributions` call site).
