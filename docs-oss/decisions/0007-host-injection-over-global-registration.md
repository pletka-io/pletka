# ADR-0007: Host injection over global registration

Date: 2026-07-13  
Status: Accepted

---

## Why this decision was needed

Splitting the public core from the private hosting platform forced a choice
about *how* a host adds surfaces core does not own — extra integrations, extra
generator formats, extra routes, admin sections, and a frontend bundle. The
common Go idioms for this are package-global registries populated by `init()`
side effects (import-for-effect). That pattern conflicts with two standing
core rules — "no global state" and "no `init()` functions" (see
[`.claude/rules/go-style.md`](../reference/go-style.md)) — and it makes a
core-only build's behavior depend on which packages happen to be imported. We
needed one documented composition model so the extension points added during
the split are all wired the same way, and so future contributions do not
reintroduce a global registry "just this once".

## What we decided

Extensions are **explicit constructor arguments** carried through option
structs, not global registration. A host supplies everything it adds by
filling fields on `app.Options` (via the `serverruntime.Config.ConfigureApp`
hook) and `cmd.OperationalOptions` (via `cmd.AddOperationalCommands`). Core
reads those fields during assembly; when they are unset, core wires its own
defaults and runs standalone. No extension point discovers a contribution by
scanning a package global or by relying on an `init()` side effect. The full
catalog of these injection points lives in
[`../architecture/composition-and-extension.md`](../architecture/composition-and-extension.md);
`pletka-platform`'s `internal/platformwiring` is the reference consumer.

## What we considered and rejected

- **Package-global registry + `init()` self-registration** (each extension
  package registers itself into a shared registry on import; the host selects
  behavior by importing packages for effect): rejected because it makes the
  composed set invisible at the call site — you cannot tell what a binary
  contains without tracing its whole import graph — and it makes builds
  order- and import-sensitive. It would also violate the existing no-globals /
  no-`init()` rules. It *would* have been more convenient for the host: adding
  a renderer would be one blank import instead of an explicit append. We chose
  visible, greppable wiring over that convenience.
- **A single aggregate "app dependencies" god object** passed to every
  contribution: rejected because it hands each contribution far more than it
  needs and couples unrelated surfaces through one type. The narrow
  `app.Host` passed to a `RouteContribution.Mount` (pool, logger, templates,
  i18n, session, integration registry + cipher) is deliberately minimal;
  fields are added only when a contribution truly needs them.
- **Build tags / separate main packages per customer** (compile the host
  surface in with `//go:build`): rejected because it moves composition into
  the build system, scatters the wiring across tag-gated files, and makes the
  core-only vs. host difference invisible in ordinary reading and testing. An
  options struct filled at runtime is testable with a plain unit test.

## Consequences

- **Easier:** a binary's full surface is readable at one call site —
  `platformwiring.ConfigureApp` and the `OperationalOptions` passed to
  `AddOperationalCommands` name every added integration, renderer, route, and
  admin section. Composition is unit-testable without build tags.
- **Easier:** core stays runnable standalone. Every injection point has a
  nil/empty fallback to a core default (e.g. `GeneratorRenderers` →
  `genwiring.CoreRenderers()`, `ConfigureApp == nil` → core-only wiring), so a
  core-only build is a first-class configuration, not a degraded one.
- **Harder:** adding an extension is explicit work — a host must thread the
  new value through `Options`/`OperationalOptions`; there is no
  import-for-effect shortcut. Adding a genuinely new *kind* of extension means
  adding a field and its assembly wiring in core, not just dropping a package.
- **Constrained:** no future contribution may introduce a package-global
  registry or an `init()`-based self-registration to work around threading a
  value through options. A new extension point is a new explicit option field
  with a documented default, added to the composition catalog.

## References

- Related ADRs: [ADR-0003](0003-explicit-errors-over-heuristics.md) (explicit
  context over runtime discovery — the same fail-loud, no-guessing stance
  applied to composition).
- Architecture: [`../architecture/composition-and-extension.md`](../architecture/composition-and-extension.md)
  — the full extension-point catalog this ADR governs.
- Rule: [`../../.claude/rules/composition.md`](../../.claude/rules/composition.md).
- Backlog: deferred registry/composition refinements are tracked in the
  `pletka-platform` host repo, where the originating design spec,
  `2026-07-13-registration-extension-points-design.md`, also lives.
- Evidence: `pkg/app/serverruntime/server.go` (`Config.ConfigureApp`),
  `cmd/root.go` (`OperationalOptions`), `pkg/app/contributions.go`
  (`RouteContribution`, `Contributions`), `internal/platformwiring/wiring.go`
  (reference consumer, in `pletka-platform`).
