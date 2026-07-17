# Schema UI Contracts

This document defines the reusable contract families. Concrete projects may add
domain-specific schemas, but they should keep the same boundaries.

## Management Schema Contracts

Use schema contracts when the UI can be rendered generically.

Core contract families:

- form schema
- list schema
- entity-list schema
- page/composite schema
- action/capability schema

The schema should contain enough information for the frontend to render and act
without domain-specific code.

## Form Contract

A form schema describes:

- entity type
- mode: create, edit, view, override, or app-defined modes
- sections
- fields
- widgets
- field values
- validation hints
- submit endpoint
- delete endpoint when applicable
- UI labels and messages

Rules:

- Field names match request JSON keys.
- Validation errors are keyed by field names.
- Read-only mode is expressed by missing endpoint and read-only fields.
- Widgets define value shape and interaction, not domain entity behavior.

## List Contract

A list schema describes:

- entity type
- data URL or embedded items
- columns or row layout
- row actions
- create/edit/delete/reorder/stats capabilities
- empty state
- grouping when needed
- UI labels and languages

Rules:

- Initial data may be embedded when simple.
- `data_url` is used to refresh after mutations.
- Row actions are declared by capability, not inferred by entity type.
- Reassignment/delete guards are explicit in the delete capability.

## Page Or Composite Contract

A page schema describes a composed screen:

- breadcrumbs
- hero/header
- stats
- metadata
- tabs
- sections
- child schema URLs
- action buttons
- callouts

Page schemas should be narrow and app-appropriate. A project management page and
an ontology browse page do not need one universal mega-schema. They should share
primitives where useful and keep domain-specific contracts where that preserves
clarity.

## Domain DTO Contract

Use a domain DTO when the UI is too rich for generic schema rendering.

A domain DTO may include:

- domain-specific graph data
- nested structural views
- derivative URLs
- editor state
- tab definitions
- resolved override chains
- read-only inspection data

Rules:

- DTOs are not database rows.
- DTOs are purpose-built frontend contracts.
- Dedicated components may know the DTO domain.
- Generic schema renderers must not grow branches for DTO-specific behavior.

## Error Contract

JSON endpoints use one error shape family:

```json
{
  "error": "validation error",
  "message": "Please correct the highlighted fields and try again.",
  "errors": {
    "ui_name": ["Name is required"]
  }
}
```

Recommended status mapping:

- `400` for malformed requests that cannot be attributed to a field
- `401` for unauthenticated
- `403` for authenticated but not allowed
- `404` for not found
- `409` for conflicts or in-use/delete guards
- `422` for validation errors
- `500` for unknown server failures

Field-level validation errors use schema field names.

## Widget Contract

Widgets are registered by name.

Every widget receives the same generic inputs:

- field definition
- current value
- form values
- language
- available languages
- field errors

Rules:

- Unknown widgets render a visible unsupported-widget state.
- Apps may register custom widgets.
- Server-side widget registration is not required; the schema is plain JSON.
- A custom widget becomes reusable only when its value shape and behavior are
  domain-independent.

## Widget Registry Contract

The frontend renderer gets its widgets from a registry, not from a hard-coded
dispatcher.

Base registry:

```ts
const widgets = {
  text: TextWidget,
  select: SelectWidget,
  'multilingual-text': MultilingualTextWidget,
}
```

An application extends it once at mount/setup time:

```ts
const widgets = {
  ...baseWidgets,
  'ontology-path': OntologyPathBuilder,
  'vocabulary-entry': VocabularyEntryPicker,
}
```

Rules:

- Generic renderers resolve by widget name.
- Apps and slices may contribute custom widgets.
- A missing widget renders an explicit unsupported-widget state.
- Widget registration is frontend-side; schemas remain plain JSON.
- A slice that requires a custom widget declares that requirement in its slice
  manifest or `doc.go`.

## Registry Activation Contract

Registries answer what is available in the current build. They do not decide
what is usable in a specific request.

Use these terms consistently:

- **Registered**: the contribution is compiled into the host and present in a
  registry.
- **Configured**: the current project or organization has supplied required
  settings, bindings, or secrets.
- **Entitled**: the current project, organization, account, or user is allowed
  to use the contribution under a license, payment plan, feature flag, or other
  commercial policy.
- **Capable**: the current actor has permission to use the contribution in this
  scope.
- **Active**: all required checks pass, so the server exposes the contribution
  through schema sections, capabilities, routes, actions, widgets, or generated
  options.

Rules:

- Core registries accept both open source and platform/private contributions.
- Open source packages may register their built-in contributions directly in
  core host wiring.
- Platform packages register private contributions from platform host wiring.
- Activation checks are server-owned and context-aware.
- Payment, license, customer, and plan checks are activation policy, not
  registry behavior.
- Frontend code renders active schema/capability output; it does not re-run
  entitlement logic.
- Missing entitlement should normally remove or disable the capability
  server-side with a clear reason when the UI needs to explain the state.

## Capability Contract

Capabilities express available actions. A capability generally contains:

- action id
- label
- method and URL, or schema URL
- confirmation/reassignment metadata when needed
- danger/read-only/disabled state when applicable

The server owns capability filtering. The frontend renders capability presence.

## Schema Provider Contract

An app may need dynamic schema lookup by entity type, settings section, admin
section, or page region. That lookup should be registry-driven, not a central
switch that imports every slice.

Provider shape, conceptually:

```go
type SchemaProvider interface {
    EntityType() string
    BuildList(ctx context.Context, in ListRequest) (*schema.ListSchema, error)
    BuildForm(ctx context.Context, in FormRequest) (*schema.FormSchema, error)
}
```

Each owning slice registers its provider during app wiring. A dispatcher may
exist, but it only looks up providers and delegates; it does not know how to
build another slice's schema.

The same idea applies to page regions:

- settings panels
- admin sections
- dashboard cards
- project tabs
- profile/org pages

A slice contributes a section through a provider, and the host page composes the
registered contributions.

Rules:

- Owning slices build their own entity schemas.
- Central dispatchers do lookup only.
- Provider registration happens in app wiring, not through global mutable state
  hidden inside packages.
- Providers declare required capabilities and scopes.

## Scope Contract

Reusable builders should receive a scope object instead of hard-coding route
prefixes.

Examples:

- no scope: `/widgets`
- project scope: `/projects/{projectID}/widgets`
- organization/project slug scope: `/orgs/{orgSlug}/projects/{projectSlug}/widgets`

Builders should emit URLs through the scope helper so the same slice can run in
different host applications.

Scope helpers should cover all URL families a schema may emit:

- page schema URLs
- data URLs
- create form schema URLs
- edit form schema URL templates
- mutation URLs
- stats/import/action URLs
- option/autocomplete URLs when they are app-routable

The frontend may replace item placeholders such as `{id}` in URL templates, but
it should not know the route prefix or route family.

## Mount Contract

A reusable slice should be mounted through explicit mount specs rather than a
single undifferentiated function map.

Conceptual shape:

```go
type MountSpec struct {
    ID          string
    Surface     string // public, admin, settings, api, page, health
    Pattern     string
    Mount       MountFunc
    Middleware  []Middleware
    RequiredCap []string
}
```

This lets a slice contribute multiple surfaces without inventing multiple
ad-hoc mount functions:

- public browse routes
- admin management routes
- settings panels
- JSON API routes
- background/health routes

The host app decides which mount specs to include and which middleware stack
wraps each surface.
