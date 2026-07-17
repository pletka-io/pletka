# Composition and Extension

How a hosting platform extends core without forking it.

Core is a runnable application on its own. A hosting binary (the reference
consumer is `pletka-platform`, which builds `pletkactl`) adds
customer-specific surfaces — extra integrations, extra generator formats,
extra routes, admin sections, and frontend bundles — by **passing them into
core as explicit constructor arguments**. Core never discovers an extension
by scanning a global registry or running an `init()`; every extension point
below is a field on an options struct that a host fills in and core reads.

The governing decision is
[ADR-0007](../decisions/0007-host-injection-over-global-registration.md):
host injection over global registration. Read it for the rationale; this page
is the catalog.

> **Terminology.** "Core" is this repository. "A host" (or "hosting
> platform") is any binary that embeds core — the only current one is the
> private `pletka-platform` wrapper, whose composition package
> `internal/platformwiring` supplies the wiring examples below. The examples
> are generic composition code, not customer configuration.

---

## The two seams a host wires through

Everything a host contributes reaches core through one of two entry points:

| Seam | Type | nil / zero value means | File |
|---|---|---|---|
| Server | `serverruntime.Config.ConfigureApp func(*app.Options, *pgxpool.Pool, *integrations.Cipher) error` | core-only wiring | `pkg/app/serverruntime/server.go` |
| CLI | `cmd.OperationalOptions` passed to `cmd.AddOperationalCommands(root, opts)` | core-only operational surface | `cmd/root.go` |

The host's `ConfigureApp` receives a partly-built `*app.Options` and fills in
the fields for the extension points it uses. Core calls it from
`buildApp` just before `app.New`; when it is nil, core builds the
core-only integration registry and proceeds
(`serverruntime/server.go`, `buildApp`).

```go
// cmd/platform/main.go — the host installs both seams.
cfg := serverruntime.ConfigFromViper()
cfg.ConfigureApp = platformwiring.ConfigureApp   // server seam

cmd.AddOperationalCommands(rootCmd, cmd.OperationalOptions{
    GeneratorRenderers: platformwiring.CLIRenderers(), // CLI seam
})
```

A third server seam, `serverruntime.Config.MigrateExtra func(db *sql.DB) error`,
lets a host run its own goose migration set immediately after core's, on the
same connection — see [extension point
8](#8-extra-migrations-platform-goose-chain) below for the contract and
wiring example.

---

## Extension points

Each section states the **contract** (the exact exported types/functions and
where they live), the **owner** (who defines the type vs. who supplies the
value), and a trimmed **real wiring example** from `platformwiring`.

### 1. Integrations registry

- **Contract:** `app.Options.IntegrationRegistry *registry.Registry`, built
  with `registry.NewRegistry(...registry.Integration)`. Core exposes its own
  set via `integrations.CoreIntegrations() []registry.Integration` and the
  convenience `integrations.CoreRegistry()`.
  Files: `pkg/integrations/integrations.go`,
  `pkg/integrations/registry/registry.go` (`NewRegistry`),
  `pkg/integrations/registry/types.go` (`Integration`).
- **Owner:** core owns the `Integration` interface and the core set; the host
  composes core's set with its own and supplies the registry. There is no
  package global and no `init()` — `CoreIntegrations` is an explicit
  constructor call every time.

```go
// internal/platformwiring/wiring.go
func Integrations() []registry.Integration {
    return append(integrations.CoreIntegrations(), archesintegration.New())
}

// inside ConfigureApp:
integrationRegistry, err := registry.NewRegistry(Integrations()...)
if err != nil {
    return err
}
opts.IntegrationRegistry = integrationRegistry
```

When a host does not supply a registry, `serverruntime` falls back to
`integrations.CoreRegistry()` so a core-only build still has its integrations.

### 2. Generator renderers — server

- **Contract:** `app.Options.GeneratorRenderers []generators.Renderer`. When
  empty, `pkg/app` falls back to `genwiring.CoreRenderers()`
  (`pkg/app/app.go`, the `if len(generatorRenderers) == 0` guard). The
  registered set also drives derivative-URL availability — see
  [Schema-driven capability](#schema-driven-capability-availability).
- **Owner:** core owns the `generators.Renderer` interface and the core
  renderer set (turtle, jsonld, mermaid, sparql, x3ml, researchspace,
  exportgraph); the host appends the formats it owns (shacl, cytoscape,
  arches).

```go
// internal/platformwiring/wiring.go
func Renderers() []generators.Renderer {
    return append(genwiring.CoreRenderers(),
        shacl.NewRenderer(),
        cytoscape.NewRenderer(),
        weavearches.NewRenderer(),
    )
}

// inside ConfigureApp:
opts.GeneratorRenderers = Renderers()
```

### 3. Generator renderers — CLI

- **Contract:** `cmd.OperationalOptions.GeneratorRenderers []generators.Renderer`,
  passed to `cmd.AddOperationalCommands(root, opts)`. Empty means
  `genwiring.CoreCLIRenderers()`. The injected set is the entire `weave
  generate` format surface: `--format` validation, help text, and the runtime
  are all derived from it. File: `cmd/root.go`.
- **Owner:** core owns the command tree and its core CLI renderer set; the
  host injects the same server set plus command-only formats (e.g. csv).

```go
// cmd/platform/main.go
cmd.AddOperationalCommands(rootCmd, cmd.OperationalOptions{
    GeneratorRenderers: platformwiring.CLIRenderers(),
})

// internal/platformwiring/wiring.go
func CLIRenderers() []generators.Renderer {
    return append([]generators.Renderer{weavecsv.NewRenderer()}, Renderers()...)
}
```

A core-only binary (empty options) accepts exactly the core formats and
cleanly rejects `shacl` with `unsupported format`, instead of registering a
renderer that would fail at runtime.

### 4. Route contributions

- **Contract:** `app.RouteContribution{ID string; Mount func(parent chi.Router, host app.Host)}`,
  collected in `app.Contributions.Routes`. `app.Host` is the narrow app-level
  dependency surface (pool, logger, templates, i18n, session, integration
  registry + cipher) passed to each `Mount`. Files:
  `pkg/app/contributions.go` (`RouteContribution`, `Host`, `Contributions`).
- **Owner:** core owns `RouteContribution`/`Host` and mounts every
  contribution during assembly; the host supplies the mount closure. `ID` and
  `Mount` are both required (`RouteContribution.Validate`).

```go
// internal/platformwiring/wiring.go — inside ConfigureApp
opts.Contributions.Routes = append(opts.Contributions.Routes, app.RouteContribution{
    ID: "arches-fleet",
    Mount: func(parent chi.Router, host app.Host) {
        archesfleet.Mount(parent, archesfleet.Host{
            Pool:              host.Pool,
            IntegrationCipher: host.IntegrationCipher,
            Integrations:      host.IntegrationRegistry,
            Logger:            host.Logger,
            Templates:         host.Templates,
            I18n:              host.I18n,
            Session:           host.Session,
        })
    },
})
```

### 5. Admin-section contributions

- **Contract:** `formschema.AdminSection`, collected in
  `app.Contributions.AdminSections`. Core appends them to the built-in
  sections in `formschema.BuildAdminSchema(snap, extraSections...)`. Files:
  `pkg/formschema/admin.go` (`AdminSection`, `BuildAdminSchema`);
  `pkg/app/app.go` wires `Sections: opts.Contributions.AdminSections` into
  `admin.Mount`.
- **Owner:** core owns the `AdminSection` schema type and renders it with the
  generic schema-driven admin shell; the host supplies the section entries.
  A section is data (id, label, icon, kind, href/schema URL) — no
  host-specific admin Svelte.

```go
// internal/platformwiring/wiring.go — inside ConfigureApp
opts.Contributions.AdminSections = append(opts.Contributions.AdminSections, formschema.AdminSection{
    ID:    "arches-fleet",
    Label: i18n.L("admin.arches_fleet.title", "Arches Fleet"),
    Icon:  "server-stack",
    Kind:  "external",
    Href:  "/admin/arches",
})
```

### 6. Static assets and frontend manifests

- **Contract:**
  `app.StaticAssetSet{ID string; FS fs.FS}` and
  `app.FrontendManifestSet{ID string; FS fs.FS; Path string}`, collected in
  `app.Contributions.StaticAssets` / `app.Contributions.FrontendManifests`.
  Files: `pkg/app/contributions.go`, `pkg/assets/islands.go` (island
  resolver).
- **Owner:** core owns the overlay and the island resolver; the host supplies
  its embedded bundle.

**Overlay order.** Contributed static sets take precedence over the core
embedded assets, and **earlier sets win over later ones** (documented on
`StaticAssetSet`). The island resolver reflects the same order: it reads each
contributed static filesystem's `dist/.vite/manifest.json` before the core
bundle and keeps the first entry seen for a given key
(`IslandResolver.loadFromEmbed` / `mergeManifest` in `pkg/assets/islands.go`).
`ResolveCSS` returns only an entry's own entry-level `css` array — it does not
walk imported-chunk CSS.

```go
// internal/platformassets/assets.go
func StaticAssets() []app.StaticAssetSet {
    return []app.StaticAssetSet{{ID: "platform", FS: staticFS}}
}
func FrontendManifests() []app.FrontendManifestSet {
    return []app.FrontendManifestSet{
        {ID: "platform", FS: files, Path: "frontend/platform.frontend.json"},
    }
}
```

**Cross-bundle singleton caveat.** A host frontend is a *separate* Vite
bundle from core's. Svelte `$lib` module state — including the module-level
stores behind layout singletons like the confirmation dialog — exists **once
per bundle**. Core's `confirm` island registers a `ConfirmDialog` against
*core's* store copy; a `confirmAction()` call from a host island reads the
*host's* store copy, which has no dialog registered, and falls through to the
`window.confirm` fallback (losing, e.g., the type-the-name guard on
destructive actions). A host that relies on such a singleton must **mount its
own instance** into its own bundle. The reference implementation is
`pletka-platform`'s `frontend/src/islands/confirm-dialog.ts`
(`mountPlatformConfirmDialog`), which mounts a host-owned `ConfirmDialog` so
the host store copy has a live subscriber. Two dialogs on the page is fine —
each store copy drives only its own instance.

### 7. Managed-instance hooks

- **Contract:** `app.Options.ManagedConfigResolver` (`hub.ManagedConfigResolver`),
  `app.Options.ManagedLabelResolver` (`hub.ManagedLabelResolver`), and
  `app.Options.ManagedConflictCheck` (`hub.ManagedConflictChecker`). `pkg/app`
  installs each onto the integrations hub via the hub's
  `SetManagedConfigResolver` / `SetManagedLabelResolver` /
  `SetManagedConflictChecker` (idempotent setters). Files:
  `pkg/integrations/hub/service.go`, `pkg/app/app.go`.
- **Owner:** core owns the hub and the resolver function types; the host
  supplies functions backed by its own managed-instance store. These let a
  host overlay managed credentials/config, label a managed instance, and veto
  a conflicting binding — without core knowing anything about the host's
  fleet model.

```go
// internal/platformwiring/wiring.go — inside ConfigureApp, when pool+cipher present
fleetStore := archesfleet.NewStore(pool, cipher)
opts.ManagedConfigResolver = fleetStore.ManagedConfigOverrides
opts.ManagedLabelResolver = fleetStore.InstanceLabel
opts.ManagedConflictCheck = func(ctx context.Context, instanceID string) (*hub.ManagedConflictLink, error) {
    // maps a host link record to the core hub.ManagedConflictLink shape
    ...
}
```

### 8. Extra migrations (platform goose chain)

- **Contract:** `serverruntime.Config.MigrateExtra func(db *sql.DB) error`,
  set directly on `Config` (not routed through `ConfigureApp` — this is a
  third top-level field alongside it). Core's `runMigrations` calls
  `database.Migrate(sqlDB)` first and, when `!cfg.SkipMigrations`, invokes
  `cfg.MigrateExtra(sqlDB)` immediately after on the same `*sql.DB`,
  wrapping any error as `extra migrations: %w`. `nil` means no host
  migrations run. File: `pkg/app/serverruntime/server.go`
  (`Config.MigrateExtra`, `runMigrations`).
- **Owner:** core owns the invocation point and the ordering guarantee (host
  migrations always run after core's, on the same connection, only when
  migrations aren't skipped); the host owns its own goose migration set and
  version table so the two chains never collide.

```go
// cmd/platform/main.go
cfg := serverruntime.ConfigFromViper()
cfg.MigrateExtra = platformdb.Migrate
```

`platformdb.Migrate` runs its own goose chain
(`internal/platformdb/migrations`) against a separate
`goose_db_version_platform` table, so platform-owned tables (the arches
fleet) migrate independently of core's `goose_db_version` chain.

---

## Schema-driven capability availability

**A capability's presence flows through the schema, never through hardcoded
frontend knowledge.** A generic renderer shows a control only because the
backend put a URL in the schema; it never knows which formats a build
registered.

The worked example is derivative URLs on the detail view. `detailview.Host`
carries a narrow reader:

```go
// pkg/weave/detailview/handler.go
HasFormat func(format generators.Format) bool
```

`pkg/app` wires it from the *same* renderer set it hands the generators
service (`pkg/app/app.go` builds `formatAvailable`/`hasFormat`;
`pkg/app/weave_slice_hosts.go` passes `HasFormat: hasFormat`). The
`DerivativesCap` builder then emits each renderer-backed URL only when its
format is registered, via `urlIfFormat` — an unavailable format yields `""`
and the field's `omitempty` drops it from the JSON entirely
(`handler.go`, `derivativesFor` / `urlIfFormat`):

```go
SHACLURL:     h.urlIfFormat(generators.FormatSHACL, withVersion(base+"/shacl", activeVersion)),
CytoscapeURL: h.urlIfFormat(generators.FormatCytoscape, withVersion(base+"/cytoscape", activeVersion)),
ArchesURL:    h.urlIfFormat(generators.FormatArches, archesURLFor(snap, kind, base, activeVersion)),
```

The frontend renders a control only when the schema provides its URL — e.g.
`{#if derivatives?.shacl_url}` in `DiagramTab.svelte`. There is no format
knowledge in Svelte. Result: on a core-only build the shacl/cytoscape/arches
buttons simply vanish (rather than rendering a button that would 500 with
`generator renderer "shacl": not registered`); on a host build that
registered those renderers, the buttons appear. Adding a format is a wiring
change, not a frontend change.

Non-renderer URLs (csv export, snapshot, ascii tree, exportgraph) keep their
existing role/capability rules — this rule is specifically about
renderer-backed capabilities tracking the registered renderer set.

---

## Future Work

The extension points above are the stable, shipping seams. The following
registry/composition refinements are deliberately deferred and remain future
work:

- **First-class list-schema contributions** — list schemas are a recurring
  contribution type but are still served through route endpoints (backlog:
  *Registry And Slice Provider*).
- **Form-schema contribution shape** — decide whether form schemas should
  always be registry contributions or whether route-local schema endpoints
  stay valid for settings/admin surfaces.
- **A capability registry** — a central, namespaced capability vocabulary is
  deferred until plugin/platform extension pressure justifies it; capabilities
  stay in `pkg/auth` for now.
- **Request-time activation granularity for route contributions** — boot-time
  activation is enough today; project/org/user-level activation is future
  work.
- **A consumed frontend widget manifest** — the registry has
  `WidgetContribution` but the frontend does not yet consume a generated
  widget manifest.
- **Descriptor-driven route mounting** — replace direct router slice imports
  and router-local `deps.Deps` adapters with descriptor registration so the
  mount path has a single source of truth (backlog: *Router And Host
  Wiring*).

## References

- Decision: [ADR-0007 — Host injection over global registration](../decisions/0007-host-injection-over-global-registration.md)
- Rule: [`.claude/rules/composition.md`](../../.claude/rules/composition.md)
- Related: [`slices.md`](slices.md) (how a slice is assembled by `pkg/app`),
  [`schema-driven-api.md`](schema-driven-api.md) (the frontend-never-builds-a-URL rule)
