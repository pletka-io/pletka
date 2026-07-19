# Composition and Extension — Design Contract

Read `docs-oss/architecture/composition-and-extension.md` before adding or
changing any extension point, and `docs-oss/decisions/0007-host-injection-over-global-registration.md`
for why the model is the way it is.

## Core Principle: Hosts Inject, Core Never Discovers

Core is runnable standalone. A hosting binary (reference consumer:
`pletka-platform`'s `internal/platformwiring`) extends core by passing
**explicit constructor arguments** through two seams — `app.Options` (via
`serverruntime.Config.ConfigureApp`) and `cmd.OperationalOptions` (via
`cmd.AddOperationalCommands`). Core reads those fields during assembly; every
one has a nil/empty fallback to a core default, so a core-only build is a
first-class configuration. No extension point is discovered through a package
global or an `init()` side effect ([ADR-0007](../../docs-oss/decisions/0007-host-injection-over-global-registration.md)).

## Where a New Extension Point Goes

| Kind of contribution | Injected through |
|---|---|
| Integrations | `app.Options.IntegrationRegistry` (`registry.NewRegistry(integrations.CoreIntegrations()..., extra...)`) |
| Generator formats (server) | `app.Options.GeneratorRenderers` — empty ⇒ `genwiring.CoreRenderers()` |
| Generator formats (CLI) | `cmd.OperationalOptions.GeneratorRenderers` — empty ⇒ `genwiring.CoreCLIRenderers()` |
| Routes | `app.Contributions.Routes` (`app.RouteContribution{ID, Mount}`) |
| Admin sections | `app.Contributions.AdminSections` (`formschema.AdminSection`) |
| Static assets + frontend | `app.Contributions.StaticAssets` / `FrontendManifests` (`app.StaticAssetSet`, `app.FrontendManifestSet`) |
| Managed-instance hooks | `app.Options.ManagedConfigResolver` / `ManagedLabelResolver` / `ManagedConflictCheck` |

A genuinely new *kind* of extension is a new explicit option field plus its
assembly wiring in core, documented in the composition catalog — never a new
global.

## Capability Presence Flows Through Schema

A capability's availability is emitted into the schema and read by generic
renderers; it is never hardcoded in the frontend. The worked example is
`detailview.Host.HasFormat` gating derivative URLs (`urlIfFormat` +
`omitempty`), so a core-only build omits the URL and the Svelte control
(`{#if derivatives?.shacl_url}`) simply does not render — instead of a button
that 500s. Adding a format is a wiring change, not a frontend change.

## Red Flags

- A package-global registry populated by `init()` self-registration (import
  for effect) instead of an explicit constructor call.
- A new extension routed around the two seams — e.g. a slice reading an
  environment variable or a global to decide whether a host feature is active.
- The frontend hardcoding a capability's URL or branching on a format name,
  instead of rendering only when the schema provides the URL.
- A host-specific dependency widened into `app.Host` when only one
  contribution needs it — keep `Host` narrow; add fields only when genuinely
  required.
- Reintroducing a core default that panics or errors when an option is unset
  instead of falling back to the core-only behavior.
- `app.Services` consumed anywhere except a host's wiring layer is a red
  flag — core code never imports it, slices receive narrowed interfaces
  ([ADR-0008](../../docs-oss/decisions/0008-services-out-seam.md)).
