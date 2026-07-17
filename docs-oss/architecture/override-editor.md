# Override Editor Design

## Purpose

This note defines the intended design for contextual override editing in the weave architecture.

It exists to align ongoing slice work before more implementation lands in parallel branches.

## Core Rules

### Structured ontology contract

Use `PathElement` directly across active weave APIs.

- `ontology_scope` stays structured as a single `PathElement`
- `path_elements` stays structured as `[]PathElement`
- handlers should not flatten ontology scope/path into strings as the primary contract
- display formatting belongs in Svelte components/utilities
- string labels are allowed only as explicit convenience fields when a surface truly needs them

This rule applies to:

- override editor payloads
- search results used by the override editor
- detailview payloads
- formschema payloads

### Base entities

Base entities own intrinsic definition only.

- `Field`
  - `ExpectedValueType`
  - ontology scope/path
  - base name/description/system name
- `Collection`
  - intrinsic collection definition
- `Model`
  - intrinsic model definition

### Override-owned state

Contextual composition belongs to overrides.

- `FieldOverride`
  - `category_id`
  - `set_value`
  - display overrides
  - description overrides
  - collection placement
  - required/hidden/min/max
- `OverrideRef[]`
  - concrete expected model/collection targets

### Important ownership boundary

- `ExpectedValueType` is field-owned
- it is not override-owned
- concrete expected targets are override-owned

## Editing Model

Override editing is parent-owned.

That means:

- model contextual editing is owned by the model slice
- collection contextual editing is owned by the collection slice
- the `pkg/weave/override` package remains an internal service/store layer
- `pkg/weave/override` does not expose standalone HTTP routes

This matches the current package intent in `pkg/weave/override/doc.go`.

## Route Design

### Read current override editor state

- `GET /projects/{projectID}/models/{modelID}/overrides`
- `GET /projects/{projectID}/collections/{collectionID}/overrides`

These endpoints should return editor-oriented JSON, not detailview JSON.

The read payload should be treated as the canonical frontend editing contract.

Recommended top-level shape:

- `entity_type`
- `entity_id`
- `project_id`
- `version`
- `scope`
- `categories`
- `available`
- `capabilities`

Where:

- `scope` is the parent entity ontological scope/class used to filter add/search
- `available` contains the search endpoints the editor should call
- `categories` is the ordered editor structure, not a raw persistence dump

Suggested shape:

```json
{
  "entity_type": "model",
  "entity_id": "LAM.1",
  "project_id": "LA",
  "version": 1,
  "scope": {
    "prefix": "crm",
    "local_name": "E22_Human-Made_Object",
    "display": "crm:E22_Human-Made_Object"
  },
  "categories": [
    {
      "category_id": "LA.CAT.1",
      "category_name": {"en": "Core"},
      "position": 1,
      "items": [
        {
          "widget": "field-group",
          "id": "__direct__",
          "name": {"en": "Direct Fields"},
          "position": 1,
          "field_count": 1,
          "fields": [
            {
              "field_id": "LAF.10",
              "override_id": 123,
              "position": 1,
              "display_name": {"en": "Title"},
              "description": {"en": "Preferred title"},
              "category_id": "LA.CAT.1",
              "expected_value_type": "Model",
              "expected_resource_models": ["LAM.20"],
              "expected_collection_models": [],
              "set_value": "",
              "is_required": true,
              "is_hidden": false
            }
          ]
        },
        {
          "widget": "collection-group",
          "id": "LAC.5",
          "name": {"en": "Birth"},
          "position": 2,
          "field_count": 1,
          "shared_path_prefix": [],
          "fields": [
            {
              "field_id": "LAF.11",
              "override_id": 124,
              "position": 1,
              "display_name": {"en": "Birth Date"},
              "category_id": "LA.CAT.1",
              "expected_value_type": "Literal",
              "expected_resource_models": [],
              "expected_collection_models": [],
              "set_value": "",
              "is_required": false,
              "is_hidden": false
            }
          ]
        }
      ]
    }
  ],
  "available": {
    "search_url": "/api/v1/projects/LA/search",
    "path_suggestions_url": "/api/v1/projects/LA/path-suggestions"
  },
  "capabilities": {
    "can_add_field": true,
    "can_add_collection": true,
    "can_reorder": true,
    "can_edit_overrides": true,
    "can_hide_fields": true
  }
}
```

### Read payload rules

- categories are ordered and must include `position`
- items within categories are ordered and must include `position`
- model context may contain both:
  - `field-group`
  - `collection-group`
- collection context should only contain:
  - `field-group`
- `field-group` in model context must use:
  - `id = "__direct__"`
- each field entry must expose effective editable override values only
- `expected_value_type` is present for display and validation, but is not editable in override context
- model-context payload may include `is_hidden`
- collection-context payload should not rely on `is_hidden` editing

### Save payload rules

- save payload should preserve the same grouping structure as the read payload
- the backend may flatten it into override rows and refs internally
- reorder is represented by the item and field `position` values supplied by the client

### Legacy example retained for comparison

The earlier minimal example shape below is still conceptually valid, but the richer grouped contract above should be treated as the actual target:

```json
{
  "entity_type": "model",
  "entity_id": "LAM.1",
  "project_id": "LA",
  "version": 1,
  "categories": [
    {
      "category_id": "LA.CAT.1",
      "category_name": {"en": "Core"},
      "items": [
        {
          "kind": "field",
          "field_id": "LAF.10",
          "override_id": 123,
          "position": 1,
          "display_name": {"en": "Title"},
          "set_value": "",
          "expected_value_type": "Model",
          "expected_resource_models": ["LAM.20"],
          "expected_collection_models": []
        },
        {
          "kind": "collection",
          "collection_id": "LAC.5",
          "collection_name": {"en": "Birth"},
          "position": 2,
          "fields": [
            {
              "field_id": "LAF.11",
              "override_id": 124,
              "position": 1,
              "category_id": "LA.CAT.1"
            }
          ]
        }
      ]
    }
  ],
}
```

### Save full override composition

- `PUT /projects/{projectID}/models/{modelID}/overrides`
- `PUT /projects/{projectID}/collections/{collectionID}/overrides`

These endpoints should treat the payload as authoritative replacement for that parent's override composition.

Suggested shape:

```json
{
  "commit_message": "Update model overrides",
  "categories": [
    {
      "category_id": "LA.CAT.1",
      "items": [
        {
          "kind": "field",
          "field_id": "LAF.10",
          "display_name": {"en": "Title"},
          "description": {"en": "Preferred title"},
          "category_id": "LA.CAT.1",
          "set_value": "",
          "is_required": true,
          "is_hidden": false,
          "position": 1,
          "expected_resource_models": ["LAM.20"],
          "expected_collection_models": []
        },
        {
          "kind": "collection",
          "collection_id": "LAC.5",
          "collection_name": {"en": "Birth"},
          "position": 2,
          "fields": [
            {
              "field_id": "LAF.11",
              "display_name": {"en": "Birth Date"},
              "category_id": "LA.CAT.1",
              "position": 1,
              "expected_resource_models": [],
              "expected_collection_models": [],
              "set_value": ""
            }
          ]
        }
      ]
    }
  ]
}
```

## Persistence Mapping

The editor payload can be nested, but persistence stays flat.

On save, the parent handler should transform editor items into:

- one `FieldOverride` row per contextual field occurrence
- `OverrideRef[]` per override row
- collection grouping represented via:
  - `part_of_collection_id`
  - `collection_order`
  - `collection_name`

The save flow should use:

- `override.Service.SaveForEntity(...)`
- then `override.Service.SetRefs(...)` for expected target refs

## Add Flows

The override editor never creates base entities.

Base `Field` and `Collection` entities must already exist before they can be composed into a parent context.

That means the override editor is a search-first adopt/composition surface, not a creation surface.

### Global base entity creation

- `POST /projects/{projectID}/fields`
- creates a base field only
- outside the override editor

- `POST /projects/{projectID}/collections`
- creates a base collection only
- outside the override editor

### Contextual add inside the override editor

The entrypoint is a unified search/select flow presented in a modal.

When the user clicks `+` inside a contextual editor:

1. open add/adopt modal
2. allow selecting:
   - existing field
   - existing collection where that parent supports adopting collections
3. modal supports explicit search modes:
   - text
   - path
4. insert the selected entity into the current editor state
5. close modal and return focus to the composition/table editor
6. immediately reveal its contextual editor view
7. save the parent override state through `PUT .../overrides`

### Add existing field from a context

Example: `+` on a category inside model detail or collection detail.

Flow:

1. open add/adopt modal
2. select an existing field
3. append a contextual field override item into the current editor state
4. prefill `category_id` from the clicked category context when applicable
5. close modal and return to the composition/table editor
6. save the parent override state through `PUT .../overrides`

This does not create a base field. It only composes an existing field into the current parent context.

### Add existing collection from a context

Flow:

1. open add/adopt modal
2. select an existing collection
3. append a collection composition block into the parent editor state
4. close modal and return to the composition/table editor
5. immediately open the collection override editor block
6. prefill category context for contained overrides where relevant
7. save the parent override state through `PUT .../overrides`

This does not create a base collection. It composes an existing collection into the current parent context.

### Context support by parent

- model override editor search should return:
  - existing fields
  - existing collections
- collection override editor search should return:
  - existing fields

This keeps one consistent mental model:

- base entities are created elsewhere
- override editing is always adoption/composition of existing entities into context

## Constraint: no direct mutation of adopted collections

In v1, users should not directly add fields into an already adopted existing collection inside a parent context.

If that is needed, the real operation is:

1. fork collection
2. edit fork
3. save fork
4. compose fork into the parent

That should be treated as a later, explicit flow.

## UI Composition

The reusable units should be widgets/components, not whole editors.

Suggested reusable pieces:

- `CategorySectionEditor`
- `CollectionOverrideEditor`
- `FieldOverrideEditor`

Then:

- model override editor uses all of them
- collection override editor reuses collection/field editor pieces with a narrower scope

This matches the desired reuse pattern where collection override editing is reused inside the model override editor.

## Interaction Rules

### Reordering

Model contextual editing must support reordering at all three levels:

- fields within a category
- collections within a category
- fields inside an adopted collection block

Collection contextual editing must support reordering of fields within that collection context.

Reordering is local editor-state mutation and is persisted through the parent `PUT .../overrides` save.

### Direct fields

Ungrouped fields should be presented consistently as a pseudo-collection/group such as `Direct Fields`.

The current read-only detailview already distinguishes direct field groups from collection groups via:

- `field-group`
- `collection-group`

The editor should use the same conceptual grouping so that display and editing follow the same structure.

For model-context read and edit APIs, direct fields should be represented explicitly as:

```json
{
  "widget": "field-group",
  "id": "__direct__",
  "name": { "en": "Direct Fields" },
  "field_count": 3,
  "fields": [...]
}
```

Adopted collections should continue to use:

```json
{
  "widget": "collection-group",
  "id": "LAC.5",
  "name": { "en": "Birth" },
  "field_count": 4,
  "fields": [...],
  "shared_path_prefix": [...]
}
```

This should be treated as part of the stable response contract, not just an incidental frontend convention.

### Hiding

Hiding is only meaningful in model context.

- model context may use `is_hidden`
- collection context should not support hiding

If a field is not wanted in a collection context, it should be removed from that collection context rather than hidden.

### Add/search scope filtering

Add/search filtering uses only the parent entity's ontological scope/class.

- category-triggered add does not further narrow search results
- category-triggered add only prefills `category_id`

### Search modes

The add/adopt modal should expose explicit modes:

- text search
- path search

Path search should reuse the existing path suggestion/path-builder backend flow rather than trying to overload a single text box with implicit syntax detection.

The composition editor itself should remain focused on arrangement and override editing, not on housing a large inline search panel.

## Validation Rules

Server-side validation should enforce:

- `entity_type` inferred from route, not trusted from payload
- every referenced field/collection/category exists in the project
- expected refs match the field's `ExpectedValueType`
- contextual category assignment is valid for the parent
- v1 does not allow implicit mutation of adopted collection structure

## Current State

What already exists:

- base field schema and field slice
- override store/service layer
- detailview save URL contract
- field override schema builder
- parent slice HTTP handlers for model/collection override editing
- mounted `GET/PUT .../overrides` endpoints on model and collection slices
- editing UI components in detailview (OverrideEditor.svelte, OverrideSidebar.svelte, FieldOverride.svelte, and supporting state management)

What does not exist yet:

- full integration testing of the complete override composition workflow

## Search API Relationship

The current backend already has the right conceptual pieces for contextual adopt/search:

- `GET /api/v1/projects/{projectID}/search`
- `GET /api/v1/projects/{projectID}/path-suggestions`

The search endpoint already supports:

- field and collection result types
- scope-aware project-chain resolution
- free text search
- path-derived filtering
- ontology class filtering
- linked-item marking for current context

This should be treated as an active weave API and is a good candidate to move into a dedicated `pkg/weave/search` package mounted from `pkg/weave/router`.

## Recommended v1 Scope

Implement first:

- model override editor read/save
- collection override editor read/save
- contextual add of existing field
- contextual add of existing collection
- category-prefill on add-from-context

Do not implement yet:

- collection fork flow
- standalone override route family
- edit lock
- final draft persistence strategy

## Relationship to Detailview

Detailview already advertises:

- `/projects/{projectID}/models/{modelID}/overrides`
- `/projects/{projectID}/collections/{collectionID}/overrides`

This document treats those as the canonical parent-owned save endpoints.

The editing UX may live:

- inside detailview
- or inside a dedicated override editor surface launched from detailview

That UX choice is secondary to the backend contract and ownership model defined here.
