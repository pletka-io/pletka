# Schema UI Principles

These principles define the reusable design convention.

## 1. Schema Is The UI Contract

For management-style UI, the backend returns a schema contract that describes:

- layout
- fields or sections
- data to render or data URLs to refresh
- capabilities
- mutation URLs
- user-facing labels and messages
- validation rules

The frontend renders the contract. It does not rediscover permissions, construct
mutation URLs, or infer domain behavior from entity names.

## 2. Domain Logic Stays Explicit

Schema UI is not an entity framework.

Slices still own:

- validation
- permission checks
- storage
- lifecycle rules
- delete guards
- import/export rules
- read model assembly

The reusable core owns generic mechanics: schema types, renderers, widget
dispatch, error shape, mount conventions, and slice composition helpers.

## 3. Two Frontend Contract Families Are Valid

Schema-driven contracts are for generic management surfaces:

- forms
- lists
- settings panes
- admin pages
- overview pages
- lifecycle/action surfaces

Explicit domain DTOs are for rich readers and editors:

- structural editors
- graphs and diagrams
- detail views with substantial state
- derivative previews
- domain-specific workspaces

Do not force structural editors into generic form/list schemas. Do not smuggle
domain-specific branches into generic renderers.

## 4. URLs Flow From The Server

The frontend may know a schema entry-point URL.

All follow-up URLs for create, edit, delete, reorder, stats, imports, active
version changes, and other actions must come from the schema or DTO contract.

This keeps route shape, scoping, slugs, permissions, and deployment differences
server-owned.

## 5. Capabilities Are Server-Gated

If the current actor cannot do something, the schema omits the capability or
endpoint.

Frontend components render whatever exists:

- no endpoint means no submit
- no delete capability means no delete button
- no reorder capability means no drag handle
- no admin action means no admin control

Mutation endpoints still enforce permissions server-side.

## 6. Generic Renderers Stay Generic

A renderer may branch on schema type, widget type, capability, or layout block.

A renderer must not branch on application entity type.

If code needs `entity_type == "foo"` inside a generic renderer, either:

- add a genuine generic capability, or
- move the feature into a dedicated island/DTO.

## 7. Read Models Precede Page Schemas

For page-level UI, assemble a read model first, then build the schema from it.

This keeps schema builders pure and makes the same data reusable for public,
admin, API, and tests.

Preferred flow:

```text
stores/services -> read model -> schema builder -> response
```

## 8. Contract Drift Is A Bug

Schema field names, request structs, validation errors, and generated frontend
types must agree.

If a field is named `ui_name` in the schema, the request body and validation
error keys should use `ui_name` too. Any exception must be documented and tested.

## 9. Reuse Requires Narrow Seams

Reusable slices depend on:

- the core slice dependency contract
- core schema types
- core error contract
- optional app-provided services through narrow interfaces

They should not depend on a whole application object, global route names, or
hard-coded project assumptions unless those are declared as part of the slice's
scope.

## 10. Extraction Follows Proof

Do not tag a reusable library contract until a real application consumes it.

The recommended cycle is:

1. extract locally
2. migrate a real slice onto it
3. fix seams while untagged
4. verify with smoke tests and contract tests
5. tag the reusable package

## 11. Registries Do Not Activate Features

Registration means a contribution exists in this build. It does not mean the
current project, organization, user, license, or payment plan can use it.

Activation is a separate server-owned policy step. A contribution becomes
active only after the host has checked the relevant context:

- build registration
- project or organization configuration
- required secrets or external service setup
- user permissions and capabilities
- paid entitlements or license gates

Reusable core packages may expose registries and contribution contracts. They
must not embed SaaS payment checks, customer-specific license rules, or project
activation policy. Those checks live in the host platform and influence which
capabilities, schema sections, routes, widgets, generators, or integration
actions are exposed for the current request.
