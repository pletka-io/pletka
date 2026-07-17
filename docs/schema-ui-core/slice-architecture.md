# Slice Architecture

The reusable unit is a vertical slice.

A slice is a bounded feature package that owns its entity or workflow from
storage through schema generation. The reusable library should define the shape;
applications keep their domain code explicit.

## Full Slice

A full slice owns an entity or tightly coupled workflow.

Recommended file layout:

```text
<slice>/
  doc.go
  routes.go
  handler.go
  service.go
  store.go
  store_postgres.go
  formschema.go
  apierror_adapters.go
  <slice>_test.go
```

Responsibilities:

- `routes.go`: canonical mount entry point.
- `handler.go`: HTTP parsing and response writing.
- `service.go`: business rules and permissions.
- `store.go`: store interface.
- `store_postgres.go`: database implementation.
- `formschema.go`: form/list schema builders.
- `apierror_adapters.go`: slice error adapters.
- tests: service, schema, route, or store tests according to risk.

## Handler-Only Module

A handler-only module composes other slices.

Use it for:

- page-schema endpoints
- cross-slice overview pages
- exports
- visualizations
- detail/read-only DTO surfaces

Recommended file layout:

```text
<module>/
  doc.go
  routes.go
  handler.go
```

Rules:

- no own table
- no own store
- no reimplementation of another slice's schema builder
- may import sibling services when composition is the purpose

## Dependency Rules

Full slices depend inward on core contracts and narrow interfaces.

Allowed:

- core schema package
- core error package
- app dependency struct passed at mount time
- interfaces declared by the consuming slice
- domain types owned by the app or reusable package

Avoid:

- importing sibling slices directly
- importing router packages into slices
- exposing pgx/sqlc types outside store implementations
- passing a whole app object when a narrow reader would do

## Slice Contributions

A reusable slice may contribute more than HTTP routes. It can register:

- public routes
- admin routes
- settings panels
- page tabs or dashboard sections
- schema providers
- custom widgets
- background jobs
- migrations
- seed data

This is the natural extension point for platform composition. For example, an
ontology slice can contribute:

- public ontology browse pages
- admin ontology management pages
- project settings panels for linked ontology versions
- autocomplete endpoints
- an `ontology-path` widget

The host platform chooses which contributions to enable.

Contributions should be explicit data, not package side effects. Prefer app
wiring like:

```go
platform.Register(ontology.Contributions(deps))
```

over hidden `init()` registration.

## Mount Specs

Slices expose route contributions as mount specs.

Each mount spec names:

- route pattern
- surface kind: public, admin, settings, api, page, health
- mount function
- middleware needs
- capability requirements

This keeps the one-slice mental model while allowing a slice to participate in
multiple app surfaces without ad-hoc functions such as `MountAPI`,
`MountPages`, or `MountAdmin`.

## Service Boundary

The service owns behavior:

- validation
- permission checks
- lifecycle transitions
- delete guards
- usage impact calculations
- cross-store orchestration behind narrow readers

Handlers should be shallow:

```text
parse request -> call service -> build schema/DTO/response -> write JSON
```

## Store Boundary

The store owns persistence:

- SQL queries
- transaction details
- row mapping
- uniqueness lookup
- persistence-specific error details

The store interface should speak in domain/read-model terms, not sqlc row terms.

## Schema Builder Boundary

Schema builders are pure contract assembly.

They may accept:

- read models
- domain entities
- scope helpers
- capability flags
- language settings

They should not:

- query stores
- inspect HTTP requests
- write responses
- perform permission checks directly unless passed precomputed capability inputs

## Read Models

Read models bridge services and schemas.

Use read models when:

- the page combines several stores
- public and admin views share data
- usage counts or impact summaries are needed
- schema construction would otherwise need database access

Read models are not generic framework entities. They are app/slice-specific
plain structs.

## Reusable Slice Requirements

A slice is portable when it declares:

- required database migrations or table assumptions
- required capabilities
- required custom widgets
- required scope shape
- required narrow reader interfaces
- route/mount contributions
- schema provider contributions
- admin/settings/page contributions
- seed data or fixtures, if any
- supported schema contracts

This can be a `doc.go` section initially. A future library may formalize it as a
slice manifest.

## Slice Manifest

A slice manifest is the machine-readable version of the portability declaration.
It does not need to exist on day one, but the design should point toward it.

Conceptual fields:

```yaml
id: ontology
requires:
  capabilities:
    - ontology.manage
  widgets:
    - ontology-path
  scopes:
    - project
contributes:
  mounts:
    - surface: public
      pattern: /ontologies
    - surface: admin
      pattern: /admin/ontologies
  settings:
    - id: project-ontologies
      capability: project.edit
  schema_providers:
    - ontology
migrations:
  - 024_weave_ontology_clean_schema.sql
```

The first implementation can be plain Go data returned from the slice package.
A later scaffold or platform runtime can read a manifest file if that becomes
useful.
