# Ontology Context Widget

**Goal:** A reusable Svelte component that renders ontology classes and properties as interactive badges with on-demand metadata popovers, replacing flat text rendering across all surfaces. Phase B delivers read-only popovers; Phase C adds inline navigation. Connected to the project-scoped ontology autocomplete infrastructure.

**Dependency:** Implementation deferred until the weave database-driven ontology autocomplete is finalized. This spec captures the design so it can be planned and built at that point.

---

## Components

### `OntologyBadge`

A single inline badge for one `PathElement`. Renders the prefixed name (e.g., `crm:E21_Person`) with class/property color coding and an (i) icon that opens a metadata popover.

**Props:**
- `element: PathElement` — the ontology element to display (or a string prefixed name for backwards compat)
- `projectId: string` — used to resolve the project's active ontology version
- `style?: "purple" | "blue"` — defaults to purple for classes, blue for properties (auto-detected from `element.type` if not specified)
- `size?: "compact" | "normal"` — compact for list badges, normal for detail views

**Behavior:**
- Renders: `[prefix:LocalName] (i)`
- Color: purple for `type === "class"`, blue for `type === "property"` or `type === "property-class"`
- On (i) click: fetches metadata from `GET /api/ontology/metadata/{versionId}/{uri}` and displays popover

### `OntologyPath`

Renders an array of `PathElement` as a visual chain with arrows between elements. Each element is an `OntologyBadge`.

**Props:**
- `elements: PathElement[]` — the path to display
- `projectId: string` — for metadata lookups
- `compact?: boolean` — condensed rendering for list rows

**Rendering:**
```
[E21_Person](i) → [P1_is_identified_by](i) → [E42_Identifier](i)
```

Classes render purple, properties render blue, alternating as they do in the ontology path.

---

## Popover Content (Phase B)

Fetched on-demand when the user clicks (i). Single API call per element.

| Field | Source |
|---|---|
| Prefixed name | `PathElement.PrefixedName()` (already in data) |
| Full URI | `PathElement.URI` (already in data) |
| Label (current language) | `GET /api/ontology/metadata/{versionId}/{uri}` → label |
| Scope note / comment | metadata endpoint → comment |
| Superclasses | metadata endpoint → superClasses (list of prefixed names) |
| Subclasses | metadata endpoint → subClasses (list, may be empty) |

The popover is read-only. Links are displayed as plain text, not clickable.

---

## Popover Navigation (Phase C — later)

Extends the Phase B popover without changing the component API:

- Superclass/subclass names become clickable links
- Clicking a link re-fetches metadata for that class and replaces the popover content (with back navigation)
- Properties section added: shows domain/range properties valid for this class in the project's ontology
- Property links are also navigable
- Back button returns to previous element (breadcrumb stack within the popover)

No prop changes — Phase C is purely internal to the popover implementation.

---

## Where It Appears

| Surface | Component | Data Source |
|---|---|---|
| Entity list badges (fields, models, collections) | `OntologyBadge` via `EntityListBadge` dispatch | `PathElement` from API response |
| Field detail view — ontology scope | `OntologyBadge` | `field.OntologyScope` |
| Field detail view — path elements | `OntologyPath` | `field.PathElements` |
| Model/collection detail view — scope | `OntologyBadge` | `model.OntologyScope` / `collection.OntologyScope` |
| Path builder preview | `OntologyPath` | Path builder state |
| Ontology playground | `OntologyBadge` | Playground selection |

---

## Schema Integration

### New badge type: `"ontology"`

Add `"ontology"` to the `BadgeConfig.Type` vocabulary. When `EntityListBadge` encounters `Type: "ontology"`, it renders an `OntologyBadge` component instead of a plain text span.

**Schema builder change** (all three entity list schemas):

```go
// Before
{Key: "ontology_scope", Type: "text", Style: "purple"}

// After
{Key: "ontology_scope", Type: "ontology", Style: "purple"}
```

### API response change

For models and collections, the `ontology_scope` field in list API responses should carry the full `PathElement` object (not the flattened string). The `OntologyBadge` component needs the structured data (URI, type, prefix, local_name) to render correctly and call the metadata endpoint.

For fields, `ontology_scope` stays as the flattened string in list responses (the `fieldListItem` projection), but `path_elements` already carries the full `PathElement[]` array for detail views.

**Migration note:** When this widget ships, the `flattenModel` and `flattenCollection` projections added in Plan 5(a) should be updated to pass the raw `PathElement` for `ontology_scope` instead of the `PrefixedName()` string. The `OntologyBadge` component handles display formatting. For fields, the `fieldListItem` projection can either be updated similarly or kept as-is (the component accepts both string and `PathElement`).

---

## Project Ontology Version Resolution

The `OntologyBadge` component needs the project's active ontology version ID to call the metadata endpoint. Resolution path:

1. `projectId` prop is always available (passed from schema or page context)
2. Component calls a lightweight endpoint or uses a cached lookup to resolve `projectId → ontologyVersionId`
3. The `GET /projects/{projectID}/ontology-labels` endpoint already does this resolution internally — the version ID can be exposed or a small lookup endpoint added

**Caching:** The version ID is stable within a session (ontology versions don't change during normal use). Cache client-side per `projectId` — one fetch per project per page load.

---

## What This Does NOT Cover

- Editing ontology scope (that's the path builder's job)
- Creating or importing ontology versions
- The weave database-driven autocomplete migration itself (prerequisite)
- Ontology validation or path correctness checking
- Mermaid diagram integration (model view diagrams use a separate rendering pipeline)

---

## Implementation Phasing

**Phase B (initial implementation):**
1. `OntologyBadge` component with read-only popover
2. `OntologyPath` component using `OntologyBadge`
3. `"ontology"` badge type in `EntityListBadge` dispatch
4. Schema builders updated to use `Type: "ontology"`
5. Integration in entity lists, detail views, path builder preview, playground

**Phase C (follow-up):**
6. Navigable popover (clickable links, back button)
7. Properties section in popover (domain/range from project ontology)
8. Breadcrumb stack for navigation history within popover

---

## Success Criteria

1. `OntologyBadge` renders a colored badge with (i) icon for any `PathElement`
2. Clicking (i) shows a popover with scope note, URI, and hierarchy — fetched from the project's ontology
3. `OntologyPath` renders field paths as a chain of clickable badges
4. Entity list rows for fields/models/collections use `OntologyBadge` instead of plain text
5. Detail views for fields/models/collections use `OntologyBadge`/`OntologyPath`
6. Path builder preview uses `OntologyPath`
7. Ontology playground uses `OntologyBadge`
8. No `[object Object]` rendering anywhere — the component handles both string and `PathElement` input gracefully
