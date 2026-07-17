# Slice Contribution Registry Design

Date: 2026-06-18

Status: active design convention for the public-core/platform split.

## Goal

Reusable slices should declare what they can contribute. The host application
decides which contributions are active and wires them into routes, admin,
settings, schema providers, widgets, generators, and integrations.

This keeps slices reusable and prevents host-level packages such as routers,
admin shells, settings pages, and billing/entitlement checks from importing
every feature package directly.

## Core Concepts

### Registration

Registration means the binary knows a slice exists and can provide its
contributions.

Registration is static application composition:

```go
registry.RegisterProvider(category.Slice{Host: categoryHost})
registry.RegisterProvider(model.Slice{Host: modelHost})
```

Registration does not imply the feature is enabled for a deployment, project,
organization, or user.

### Activation

Activation means the host application decided a registered slice is available
in a specific context.

Activation may depend on:

- deployment configuration
- organization settings
- project settings
- user capabilities
- paid entitlements
- feature flags
- integration availability

The public core exposes the hook. Platform code owns the policy.

## Public Contract

The public contract lives in `schemaui/registry`.

Main types:

- `SliceDescriptor`
- `DescriptorProvider`
- `Contributions`
- `SliceRegistry`
- `ActivationContext`
- `ActivationPolicy`

`SliceDescriptor` declares a slice:

```go
type SliceDescriptor struct {
    ID            string
    Label         domain.Localizable
    Description   domain.Localizable
    Version       string
    Capabilities  []string
    Contributions Contributions
}
```

`Contributions` declares app surfaces:

```go
type Contributions struct {
    Mounts        []MountSpec
    AdminSections []AdminSection
    Settings      []SettingsPanel
    Widgets       []WidgetContribution
    Schemas       []SchemaContribution
    Generators    []GeneratorContribution
}
```

The host asks for active contributions:

```go
contribs, err := registry.ActiveContributions(ctx, activationContext, policy)
```

If `policy` is nil, all registered descriptors are active. Production platform
wiring should provide a policy once feature flags, org/project settings, or
entitlements matter.

Configured slices should implement `DescriptorProvider`:

```go
type DescriptorProvider interface {
    Descriptor() (SliceDescriptor, error)
}
```

The descriptor method should validate the slice host before returning
contributions. Empty or incomplete hosts should fail at boot, before routes or
schema providers are mounted.

## Slice Responsibility

A slice may define:

- its service/store contracts
- its schema builders
- its route contribution
- its admin/settings entries
- widgets it needs rendered by the frontend
- generator descriptors it can provide

A slice may choose where auth checks apply in its own workflow. For example, a
slice may decide that a route requires `auth.ProjectEdit`, load the project it
is protecting, and build the `auth.Resource` needed for `snap.Can(...)`.

A slice should not decide:

- whether it is enabled in a deployment
- whether an org/project has paid access
- whether the current user can see a host shell section
- platform-specific wiring order beyond its declared contribution order
- auth vocabulary, role-to-capability mappings, or private capability strings

## Host Responsibility

The host application:

- registers descriptors from public core and platform packages
- provides concrete dependencies to descriptors where needed
- applies activation policy
- mounts active routes
- appends active admin/settings sections
- registers active schema providers/widgets/generators

## Design Rules

- Registration and activation are separate.
- Avoid package-level imports from host shells into every slice.
- Prefer explicit descriptor registration over implicit side effects.
- Do not use `init()` registration unless a future plugin model explicitly
  chooses that style.
- Contributions should be data-first. Use functions only where runtime wiring
  is unavoidable, such as route mounting or URL construction.
- Pletka auth semantics are centralized in `pkg/auth`. Slices consume
  capabilities and decide where checks apply, but capabilities themselves are
  defined centrally. See
  `docs/decisions/2026-06-28-pletka-auth-boundary.md`.
- A descriptor should not require the broad legacy `deps.Deps` bag to exist.
- A slice may start grouped during the split, but the long-term target is
  smaller service/store/schema/route adapter boundaries.

## Slice Provider Checklist

Use this checklist when converting a slice to the registry convention:

- The slice exposes a concrete `Host` struct as its explicit dependency
  contract.
- `Host.Validate()` checks every dependency required to build active
  contributions.
- The slice exposes `Slice{Host: ...}.Descriptor() (registry.SliceDescriptor,
  error)`.
- There is no free `Descriptor(host)` helper; validation should be part of the
  provider path.
- The slice package does not import `pkg/weave/deps` or other platform-wide
  dependency bags.
- The slice package does not import the root `pkg/weave` store aggregate unless
  that dependency is intentionally part of the reusable contract.
- The platform router may keep a temporary local adapter from `deps.Deps` to the
  slice `Host`, but that adapter lives outside the reusable slice package.
- Route contributions use stable IDs and explicit patterns.
- Admin/settings contributions declare their surface and required capabilities;
  the slice does not decide final activation.
- Required capabilities are references to centrally defined `pkg/auth`
  capabilities, not slice-local permission strings.
- Tests cover incomplete host validation and the descriptor contributions the
  slice declares.

When reviewing a converted slice, run a package import check and confirm the
slice package has no hidden dependency on legacy platform wiring:

```sh
env GOCACHE=/tmp/codex-go-build go list -f '{{join .Imports "\n"}}' ./pkg/weave/<slice>
```

## First Refactor Target

Use `category` as the first proof slice after this contract:

1. Keep `category.Store` and `category.Service` as reusable slice contracts.
2. Replace `MountWithDeps` dependence on broad `deps.Deps` with a narrower
   descriptor factory or slice host context.
3. Use `schemaui/registry.SchemaContribution` for list/form schema providers.
4. Use `schemaui/registry.MountSpec` for routes.
5. Keep platform-only activation outside the slice.

Implementation note: `pkg/weave/category` now exposes `Slice{Host: ...}` and
`NewHandlerFromHost`. `category.Slice.Descriptor()` validates `Host` before
declaring contributions. There is intentionally no free `Descriptor(host)`
helper; host validation is part of the slice-provider contract. The slice does
not import the legacy `deps.Deps` bag. The platform router owns the temporary
adapter from `deps.Deps` into `category.Host`, then mounts the route
contribution from the descriptor. This keeps the reusable slice copyable while
letting the existing platform router migrate incrementally.

`pkg/weave/namespacebinding` follows the same pattern as the second proof
slice. It contributes both project routes and admin routes from one validated
slice provider. Its list-schema/form-schema endpoints remain route endpoints
for now because the registry does not yet have a first-class list-schema
contribution type.

`pkg/weave/attribution` is the third proof slice. It uses the same provider
shape for a settings-surface route and makes the required project-reader
dependency explicit in `Host`, because its service gates reads and writes on
project permissions.

## Open Questions

- Whether route contributions should be activated only at boot or also per
  project/org at runtime.
- Whether admin/settings contribution filtering should happen before schema
  construction or inside schema builders.
- Whether generators should share one descriptor shape with integrations or
  remain separate contribution types.
- Whether frontend widget registration needs an equivalent manifest generated
  from the Go descriptors.
