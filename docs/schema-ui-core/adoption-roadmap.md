# Adoption Roadmap

This roadmap describes how to use Schema UI in an existing project and how to
move toward a reusable library.

## Phase 1: Document The Current Pattern

Create project-local docs that identify:

- schema contract types
- generic renderers
- slice/module layout
- error contract
- capability model
- widget registry
- current exceptions

For Pletka, this directory plus `docs/reference/schema-ui-conventions.md` serve
that role.

## Phase 2: Pick One Reference Slice

Choose a slice that is useful but not the most complex.

Good reference-slice traits:

- owns one table or a small table set
- has create/edit/delete/list
- uses permissions
- has tests
- uses generic widgets
- has limited cross-slice dependencies

In Pletka, `category` is the clearest reference shape. Ontology management is a
good reference for page/read-model schemas, but it is too broad for the first
library extraction slice.

## Phase 3: Stabilize Core Contracts In-Repo

Before extracting packages, stabilize:

- schema structs
- widget contract
- widget registry
- schema provider registry
- island mount contract
- error contract
- dependency shape
- mount spec shape
- scope URL helper
- slice contribution/manifest shape
- TypeScript generation/drift checks

Do this inside the real app first. Avoid tagging library APIs before one real
consumer has migrated.

## Phase 4: Extract Locally

Extract reusable pieces into local modules/packages without publishing:

- Go schema contracts
- Go slice/deps contracts
- Go error contract
- Svelte generic renderers
- Svelte widget registry
- island mount helpers
- type generation pipeline

Consume them through local replace/link mechanisms.

## Phase 5: Migrate The Real App Onto The Local Library

Migrate one slice at a time.

For each migrated slice:

- bring it to slice conformance
- replace local schema types with library schema types
- replace local generic frontend imports with library imports
- register app-specific widgets explicitly
- register schema providers explicitly
- register admin/settings/page contributions explicitly
- keep domain code in the app
- run schema, service, route, frontend smoke checks

Any seam that fights the migration is a design signal. Fix it before publishing.

## Phase 6: Add Conformance Tooling

Automate what should not rely on review discipline:

- canonical mount function shape
- no pgx/sqlc imports outside stores
- no raw `http.Error` in JSON handlers
- schema builders in owning slice
- central schema dispatchers delegate to registered providers only
- no forbidden frontend entity-type branches in generic renderers
- frontend widgets come from the registry
- generated TypeScript has no diff

Start with simple static checks. They can grow into a proper analyzer later.

## Phase 7: Publish Versioned Packages

Publish only after:

- at least one real app consumes the local packages
- the reference slice passes
- one richer page-schema surface passes
- frontend smoke tests pass
- contract drift checks pass
- migration pain points are documented

Initial packages may include:

- Go contracts and helpers
- Svelte renderers and base widgets
- scaffold/generator
- conformance analyzer

## Phase 8: Scaffold New Apps

A new app scaffold should include:

- runnable Go server
- database setup
- one reference CRUD slice
- schema/list/form endpoints
- generic Svelte mount setup
- widget registry
- schema provider registry
- mount spec composition
- error handling
- health/errortracking slices if desired
- tests and smoke scripts

The scaffold should produce a boring, complete slice rather than a clever demo.

## Existing Project Integration Strategy

For an existing project:

1. Add schema contracts beside existing UI; do not rewrite everything.
2. Replace one management surface with a generic renderer.
3. Move schema builders into owning slices.
4. Introduce a scope helper and stop hard-coding route families in builders.
5. Replace central schema switches with provider registration.
6. Stop extending raw model endpoints.
7. Add read models for composed pages.
8. Gradually migrate old UI surfaces behind schema contracts or explicit DTOs.
9. Delete compatibility endpoints when no screen uses them.

## Signs The Design Needs Work

Treat these as feedback, not failures:

- too many app-specific fields in core schema types
- custom widgets needed for common input behavior
- slices require the whole app dependency object
- scope URL construction leaks into frontend
- generic renderers need domain branches
- conformance requires many exceptions
- a page schema becomes a universal mega-schema
- TypeScript drift becomes common

When this happens, narrow the seam or split the abstraction. Do not keep adding
flags until the generic layer becomes domain-aware.
