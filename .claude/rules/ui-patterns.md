# UI Interaction Patterns — Design Contract

Read `docs-oss/architecture/schema-driven-ui.md` before implementing any UI work.
Read `docs-oss/architecture/svelte-islands.md` for the island mount system and the widget registry.

## Core Principle: Schema Carries Domain Knowledge

Svelte components are generic renderers. They hold UI state and handle interactions, but NEVER contain domain knowledge (entity names, field relationships, business rules, validation logic).

Domain knowledge lives in the Go schema builders (`pkg/weave/<slice>/formschema.go`). The schema tells the frontend what to show, what actions are available, and where to submit. The frontend renders it without knowing what entity type it's dealing with.

Red flags that domain knowledge is leaking into Svelte:
- Hardcoded entity names or field keys in a component
- If/else branches for different entity types
- Validation rules written in TypeScript instead of coming from the schema
- API URLs constructed in the frontend instead of provided by the schema
- Component that only works for one entity type

When you need new behavior:
1. Can it be a new capability in the list/form schema? Add it to the Go types
2. Can it be a new widget? Add the component to the `registerWidgets({...})` entry in `widget-registry.ts`, add a matching `Widget*` constant in `pkg/formschema/types.go`, and add a `frontendrefs.FormWidget("<kind>")` marker next to it — a conformance test checks every marker resolves in the frontend registry
3. Does it need a new schema type entirely? Design the schema contract first
4. None of the above? Only then write a custom Svelte component

## Decision Rules

Before writing UI code, determine which pattern applies:

- Transient feedback (save/delete/copy) -> **Toast** (`islands/toast.ts`)
- Destructive or irreversible action -> **Confirmation Dialog** (`islands/confirm.ts` + `lib/stores/confirm.ts`, call `confirmAction()` — shipped, not planned)
- Editing existing data (inline, few fields) -> **Inline Edit** (schema-driven via inline_rename capability)
- Create/edit entity (any complexity) -> **Schema Form View** (slide transition in ListManager, or standalone FormRenderer)
- Field validation errors -> **Inline Field Errors** (FormRenderer handles via schema)
- Loading or progress -> **Skeleton loader** on the relevant element
- Structured data listing -> **ListManager** (simple) or **DataTable** (paginated, planned)

## Layout Singletons (mounted in `pkg/weave/templates/layout.gohtml`)

- Toast: `addToast()` from Svelte, `islands/toast.ts`
- Confirm: `confirmAction()` from `lib/stores/confirm.ts`, `islands/confirm.ts` — reach for the singleton, do not build a per-page modal
- Aspirational, not yet built: Side Panel, Global Search

## Constraints

- Never use modals for forms — use schema form view transitions
- Never use toasts for validation errors — use inline field errors
- Never use full-page spinners — use skeleton loaders
- All entity CRUD should be schema-driven before considering custom UI
- Primary: `bg-pletka-primary` / `hover:bg-pletka-secondary`
- Destructive: `bg-red-600` / `hover:bg-red-700`
- Focus: `focus:ring-2 focus:ring-pletka-primary focus:ring-offset-2`

## Z-Index Stack

- `z-[9999]`: Toasts
- `z-50`: Confirmation dialog + backdrop
- `z-40`: Global search overlay (planned)
- `z-30`: Side panel + backdrop (planned)
- `z-20`: Dropdowns, popovers
- `z-10`: Sticky headers
