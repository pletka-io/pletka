# ADR-0005: Pletka Auth Boundary

Date: 2026-06-28  
Status: Accepted

---

## Why this decision was needed

The cleanup around `pkg/auth` and `pkg/weave/auth` exposed an unclear split:
some code treated auth as generic infrastructure, while weave slices also had
auth-specific helpers, route handlers, and resource builders. Without a clear
rule, future slice work could keep spreading auth semantics across packages and
make capability checks harder to audit during the public-core/platform split.

## What we decided

`pkg/auth` is the canonical Pletka auth layer. It owns Pletka identity,
sessions, request auth context, resources, capability vocabulary, role mappings,
and capability evaluation. Slices consume `pkg/auth`; they may choose where to
apply checks and may load domain objects needed to build an auth resource, but
they must not define a separate auth model or private capability vocabulary.

## Design Rules

- `pkg/auth` is Pletka core auth, not a generic library split across packages.
- Auth vocabulary lives in `pkg/auth`: `Principal`, `AuthSnapshot`,
  `Capability`, `Resource`, and `snap.Can(...)`.
- Request plumbing lives in `pkg/auth`: session-derived identity, middleware,
  context helpers, and principal/snapshot accessors.
- Role-to-capability semantics live in `pkg/auth`. If a role grants a
  capability, that mapping must be visible from the central auth package.
- Slices may decide which operation requires which capability, because the
  slice owns its workflow.
- Slices may load the protected domain object and build or attach the relevant
  auth resource.
- Slices must not define private auth enums, private capability strings, or
  alternative snapshot/resource models.
- New capabilities are added centrally in `pkg/auth`, then consumed by slices.
- Capability registration or extension may be added later for plugin/platform
  needs, but only with explicit namespace, validation, documentation, and role
  mapping rules.
- `pkg/weave/auth` was transitional and has been removed. Useful HTTP/resource
  helpers now live in `pkg/auth`.

## Operational Rule

The normal slice pattern is:

```go
snap := auth.FromContext(ctx)
resource := /* build auth.Resource from the protected project */
if !snap.Can(auth.ProjectEdit, resource, nil) {
    return forbidden
}
```

The slice chooses that this operation requires `ProjectEdit`. `pkg/auth` owns
what `ProjectEdit` means, which roles grant it, how the caller is represented,
and how `Can` evaluates the request.

## What we considered and rejected

- **Keep `pkg/auth` generic and put app-specific auth in `pkg/weave/auth`:**
  rejected because the current core auth code is already Pletka-specific. It
  depends on sessions, actors, project/org roles, resources, and
  `domain.WeaveStore`. Calling it generic would create a false split.
- **Let each slice define its own capabilities:** rejected because global role
  mappings, auditability, admin UI, and split documentation all need one
  inspectable capability vocabulary.
- **Introduce a capability registry now:** rejected as premature. A registry
  needs capability namespaces, duplicate validation, default grants,
  documentation export, and admin tooling semantics. Central constants are
  clearer until plugin/platform extension pressure is real.
- **Keep the `pkg/weave/auth` forwarding shim indefinitely:** rejected because
  permanent shims hide ownership. The shim is useful only as a migration aid
  while code moves into the canonical package.

## Consequences

- **Easier:** capability checks are auditable because the vocabulary and role
  maps are centralized.
- **Easier:** slices can stay focused on workflow and resource loading instead
  of auth semantics.
- **Easier:** public-core/platform split reviews have one auth package boundary
  to inspect.
- **Harder:** adding a slice-specific permission requires editing `pkg/auth`
  instead of adding a local constant.
- **Harder:** plugin/platform capability extension must wait for a proper
  registry design if static central constants become insufficient.
- **Constrained:** new auth semantics and helpers belong in `pkg/auth`, not in
  slice-local packages.

## References

- `docs/specs/2026-06-18-slice-contribution-registry-design.md`
