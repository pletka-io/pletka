# Unified Field/Override Table Editor — Vision Document

**Status:** Deferred. Implement field-by-field editing first (Plan 6), then evolve to this table view for multi-edit.

**Goal:** A full-screen, Airtable-style table editor for model and collection field overrides. Replaces the legacy `override-table.js` and the per-collection modal pattern with a unified flat table that handles all editing contexts.

---

## Core Principle: Everything is an Override

When you adopt a collection into a model, you create an override layer — not a copy. The base collection and its fields remain untouched. The model gets overrides that can change names, descriptions, categories, hide fields, and add constraints. The only action that creates genuinely new data is **fork/copy**.

---

## Editor Layout

### Full-Screen with Context Header

Dedicated editing view. Dark header shows project + model/collection identity. Always visible — easy to compare by opening two tabs side-by-side.

```
┌─────────────────────────────────────────────────────────────────┐
│ Linked Art › Person  crm:E21_Person    42 fields  [Cancel][Save]│
├─────────────────────────────────────────────────────────────────┤
│ [Search...] [Show hidden] ────── [+ Category][+ Collection][+ Field] │
├─────────────────────────────────────────────────────────────────┤
│ Table content...                                                │
└─────────────────────────────────────────────────────────────────┘
```

### Flat Table with Section Headers

One continuous table. Category boundaries shown as colored header rows (purple). Collection sub-headers shown as lighter rows (blue). Fields are the editable rows.

**Columns (model context):** Drag handle | # | Name | Description | Category | Type | Required | Visibility | Expand

**Columns (collection context):** Drag handle | # | Name | Description | Category | Visibility | Expand

---

## UX Patterns

### 1. Context-Aware Add Buttons

Add buttons positioned close to the source, inheriting context:
- "+" on category row → wizard prefilled with that category
- "+ Field" on collection row → add field prefilled with collection + category
- "+ Collection" on category row → adopt/create collection in that category
- Global toolbar "+ Add Field" → wizard with no prefill (choose everything)

### 2. Guided Add Wizard

Clicking any "+" opens a small dialog that guides the choice:
- Adopt a Collection (search existing, creates override layer)
- Add a Field (search existing fields, adds to collection/category)
- Create New Category (name input)
- Fork Collection (creates a genuinely new collection from existing)

Wizard knows the context (which category/collection was clicked) and prefills accordingly.

### 3. Override Indicators

- Modified cells show a pencil icon (✏️) with a revert button
- Collection header shows override badges ("override: name" if renamed)
- Fields show override source on expand (base → collection → model)

### 4. Hide Fields

- Eye icon toggle per field row
- Hidden fields: grayed out, strikethrough, `is_hidden: true` in override
- "Show hidden" checkbox in toolbar to filter them out entirely
- Unhide = click eye icon again (removes `is_hidden` override)

### 5. Copy vs Override

- **Adopting a collection** = always creates an override layer (default action)
- **Fork** = creates a genuinely new collection entity, copies all fields (explicit action, "Fork" button on collection row)
- **Editing a field** = creates/modifies model-level override (base untouched)

### 6. Drag-and-Drop Reorder

- Drag handles on all levels: categories, collections, fields
- Categories reorder among themselves
- Collections reorder within their category
- Fields reorder within their collection (or within direct fields)

---

## Actions Summary

### On Category Row
| Action | Effect |
|--------|--------|
| Rename | Edit category display name |
| + (wizard) | Add collection or field to this category |
| Reorder | Drag to change category position |

### On Collection Row
| Action | Effect |
|--------|--------|
| Rename | Override collection name for this model |
| + Field | Add field to this collection (model-level) |
| Fork | Create new collection from this one (new entity) |
| Reorder | Drag within category |

### On Field Row
| Action | Effect |
|--------|--------|
| Edit name/desc | Inline edit → creates/modifies override |
| Change category | Dropdown → override category assignment |
| Set constraints | Required, min/max, set value, expected type |
| Hide | Toggle visibility (override is_hidden) |
| Expand (⋯) | Show secondary fields + immutable base info |
| Reorder | Drag within collection |

---

## Implementation Path

1. **Plan 6 (now):** Field-by-field editing in the current Svelte entity view. Edit one field at a time via the expand row or a side panel. Save individual overrides. This establishes the backend save flow (overrides → ChangeLogger → git).

2. **Plan 6b (later):** Evolve to the table editor. Replace the accordion fields tab with the flat table. Same backend, different frontend. Multi-edit, bulk operations, drag-and-drop.

---

## Relationship to Existing Code

- **Replaces:** `pkg/assets/static/js/override-table.js` (legacy vanilla JS table)
- **Evolves:** `frontend/src/lib/components/editor/ModelEditor.svelte` (current Svelte editor)
- **Uses:** `pkg/domain/FieldOverride` + `WeaveStore.Overrides()` (existing backend)
- **Integrates with:** `ChangeLogger` + `GitMaterializer` (Plan 5c) for persistence
- **Component name:** `OverrideTableEditor.svelte` (new, full-screen)
- **Reuses:** `PaginationBar.svelte` for large models (600+ fields)
