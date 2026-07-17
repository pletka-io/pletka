# Plan 4b: Schema-Driven ProjectDetail Svelte Component

**Goal:** Replace the 3491-line vanilla JS `detail.gohtml` with a thin Svelte island mount. Everything the page does — header, tab navigation, overview content, tab content routing — moves into Svelte components driven by server-side schemas. Migrates project data access from GORM to WeaveStore along the way.

---

## Problem

The project detail page (`/projects/{id}`) has accumulated ~3000 lines of vanilla JS: tab switching, URL hash state, field search, filter handling, render functions, and inline HTML templates. The page:

- Uses GORM repos (`repos.Projects`, `repos.Fields`) which return empty in this branch's weave database
- Reloads tab content on every tab switch (no client-side caching)
- Has dead JS left over from the Plan 4 fields tab conversion
- Can't be maintained in its current form

## Principles

1. **Schema is the contract** — the server describes the page (tabs, header, widget sections); the frontend is a generic renderer
2. **Page-level header above tabs** — matches the EntityView pattern: entity name/description/scope always visible, tabs switch only the content below
3. **Lazy, cached tab content** — content fetched on first tab activation, cached in memory, no re-fetches
4. **Reusable widgets** — stats cards, info sidebars, README viewer work the same way everywhere they appear
5. **WeaveStore everywhere** — no GORM in the new page handler or data endpoints

---

## API

### Page schema endpoint

`GET /projects/{projectID}/page-schema` → `*ProjectPageSchema`

```json
{
  "entity": {
    "id": "LA",
    "name": {"en": "Linked Art"},
    "description": {"en": "Linked Art ontology for cultural heritage"},
    "scope": null,
    "status": "published"
  },
  "tabs": [
    {
      "id": "overview",
      "label": {"en": "Overview", "nl": "Overzicht"},
      "icon": "home",
      "content_url": "/projects/LA/overview-schema",
      "default": true
    },
    {
      "id": "models",
      "label": {"en": "Models", "nl": "Modellen"},
      "icon": "cube",
      "content_url": "/projects/LA/entity-list-schema/model",
      "count": 15
    },
    {
      "id": "collections",
      "label": {"en": "Collections", "nl": "Collecties"},
      "icon": "collection",
      "content_url": "/projects/LA/entity-list-schema/collection",
      "count": 23
    },
    {
      "id": "fields",
      "label": {"en": "Fields", "nl": "Velden"},
      "icon": "list",
      "content_url": "/projects/LA/entity-list-schema/field",
      "count": 450
    },
    {
      "id": "activity",
      "label": {"en": "Activity", "nl": "Activiteit"},
      "icon": "clock",
      "content_url": "/projects/LA/activity-schema"
    }
  ],
  "nav_links": [
    {
      "label": {"en": "Downloads", "nl": "Downloads"},
      "href": "/projects/LA/downloads",
      "icon": "download"
    },
    {
      "label": {"en": "Settings", "nl": "Instellingen"},
      "href": "/projects/LA/settings",
      "icon": "cog"
    }
  ]
}
```

The schema builder uses WeaveStore to count models, collections, and fields. Tabs and nav links are filtered by user role:

- **Tabs** (overview, models, collections, fields, activity) — visible to all users who can view the project
- **Nav links**:
  - **Downloads** — visible to all users who can view the project
  - **Settings** — visible only to users with admin rights on the project (owner, maintainer, or superadmin role)

The `BuildProjectPageSchema` function takes the current user as a parameter and omits the Settings nav link for non-admin users. If the user has no admin rights, `nav_links` contains only Downloads (or is omitted entirely if both are hidden).

### Overview schema endpoint

`GET /projects/{projectID}/overview-schema` → widget-annotated sections

```json
{
  "sections": [
    {
      "widget": "readme",
      "content": "# Linked Art\n\n..."
    },
    {
      "widget": "stats-cards",
      "title": {"en": "Statistics"},
      "items": [
        {"label": {"en": "Models"}, "count": 15, "icon": "cube", "color": "purple"},
        {"label": {"en": "Collections"}, "count": 23, "icon": "collection", "color": "green"},
        {"label": {"en": "Fields"}, "count": 450, "icon": "list", "color": "blue"},
        {"label": {"en": "Categories"}, "count": 12, "icon": "tag", "color": "amber"}
      ]
    },
    {
      "widget": "info-sidebar",
      "title": {"en": "Project Details"},
      "items": [
        {"key": "license", "label": {"en": "License"}, "value": "CC-BY"},
        {"key": "namespace", "label": {"en": "Namespace"}, "value": "https://linked.art/", "style": "mono"},
        {"key": "created", "label": {"en": "Created"}, "value": "2024-01-01", "type": "date"}
      ]
    }
  ]
}
```

The overview schema is computed from `domain.Project` + WeaveStore counts. README comes from the project's `README` field.

### Migrated fields data endpoint

`GET /projects/{projectID}/fields` (existing URL, rewired)

Currently implemented in `FieldHandler.ProjectFieldsAPI` using GORM. Rewire to use WeaveStore:
- Fetch fields via `h.weave.Fields().List(ctx, WithProjectID(projectID))`
- Build response in the same `{"fields": [...], "count": N}` shape expected by `entity-list` island
- Preserve pagination and filtering params

The entity-list island is the sole consumer; keeping the URL unchanged means no frontend changes beyond Plan 4.

---

## Backend

### New files

| File | Purpose |
|------|---------|
| `pkg/formschema/project_page.go` | `ProjectPageSchema` type, `BuildProjectPageSchema()` |
| `pkg/formschema/project_overview.go` | `ProjectOverviewSchema` type, `BuildProjectOverviewSchema()` |

### Modified files

| File | Change |
|------|--------|
| `pkg/handlers/projects.go` | Replace `ProjectDetailPage` body to use WeaveStore; add `ProjectPageSchemaAPI`, `ProjectOverviewSchemaAPI` |
| `pkg/handlers/fields.go` | Rewrite `ProjectFieldsAPI` to use WeaveStore (`h.weave.Fields().List()`) |
| `pkg/router/router.go` | Register new schema endpoints |

### Route registration

```go
r.Get("/{projectID}/page-schema", h.ProjectPageSchemaAPI)
r.Get("/{projectID}/overview-schema", h.ProjectOverviewSchemaAPI)
```

### ProjectPageSchema type

```go
type ProjectPageSchema struct {
    Entity    ProjectPageEntity    `json:"entity"`
    Tabs      []ProjectPageTab     `json:"tabs"`
    NavLinks  []ProjectPageNavLink `json:"nav_links"`
    UI        SchemaUI             `json:"ui"`
}

type ProjectPageEntity struct {
    ID          string               `json:"id"`
    Name        domain.Translations  `json:"name"`
    Description domain.Translations  `json:"description,omitempty"`
    Status      string               `json:"status"`
}

type ProjectPageTab struct {
    ID          string               `json:"id"`
    Label       domain.Translations  `json:"label"`
    Icon        string               `json:"icon"`
    ContentURL  string               `json:"content_url"`
    Count       int                  `json:"count,omitempty"`
    Default     bool                 `json:"default,omitempty"`
}

type ProjectPageNavLink struct {
    Label domain.Translations `json:"label"`
    Href  string              `json:"href"`
    Icon  string              `json:"icon"`
}
```

### ProjectOverviewSchema type

```go
type ProjectOverviewSchema struct {
    Sections []ProjectOverviewSection `json:"sections"`
    UI       SchemaUI                 `json:"ui"`
}

type ProjectOverviewSection struct {
    Widget  string                `json:"widget"` // "readme" | "stats-cards" | "info-sidebar"
    Title   domain.Translations   `json:"title,omitempty"`
    Content string                `json:"content,omitempty"`       // for readme
    Items   []ProjectOverviewItem `json:"items,omitempty"`         // for stats-cards, info-sidebar
}

type ProjectOverviewItem struct {
    Key   string               `json:"key,omitempty"`   // for info-sidebar
    Label domain.Translations  `json:"label"`
    Value string               `json:"value,omitempty"` // for info-sidebar
    Count int                  `json:"count,omitempty"` // for stats-cards
    Icon  string               `json:"icon,omitempty"`
    Color string               `json:"color,omitempty"` // for stats-cards
    Style string               `json:"style,omitempty"` // "mono" for monospace values
    Type  string               `json:"type,omitempty"`  // "date" triggers date formatting
}
```

---

## Frontend

### New files

| File | Purpose |
|------|---------|
| `frontend/src/lib/types/project-page.ts` | TypeScript types matching Go schemas |
| `frontend/src/lib/stores/project-detail.svelte.ts` | `ProjectDetailState` class — schema fetch, tab state, content cache |
| `frontend/src/lib/components/project/ProjectDetail.svelte` | Entry component — header + tab bar + active tab content |
| `frontend/src/lib/components/project/ProjectHeader.svelte` | Entity name, description, status badge |
| `frontend/src/lib/components/project/TabBar.svelte` | Tab navigation with counts and icons |
| `frontend/src/lib/components/project/ProjectOverview.svelte` | Overview tab renderer, dispatches widgets |
| `frontend/src/lib/components/project/widgets/ReadmeViewer.svelte` | Markdown README renderer |
| `frontend/src/lib/components/project/widgets/StatsCards.svelte` | 4-up stats card grid |
| `frontend/src/lib/components/project/widgets/InfoSidebar.svelte` | Key/value sidebar (license, namespace, etc.) |
| `frontend/src/islands/project-detail.ts` | Island entry point |

### Deleted/rewritten

| File | Action |
|------|--------|
| `pkg/assets/templates/pages/projects/detail.gohtml` | Shrink from 3491 lines to ~50 lines: breadcrumb + island mount + loading skeleton |

### ProjectDetailState

```typescript
class ProjectDetailState {
  projectId: string
  pageSchema: ProjectPageSchema | null = $state(null)
  activeTabId: string = $state('')
  tabContent: Map<string, unknown> = $state(new Map())  // cache per tab
  loading: boolean = $state(false)
  error: string | null = $state(null)

  async init()                          // fetch page schema, determine default tab, restore from URL hash
  async setActiveTab(tabId: string)     // set active, fetch content if not cached, update URL hash
  private async fetchTabContent(tab)    // fetch content_url, store in cache
}
```

### URL hash handling

`#tab=fields` selects the fields tab on load. Component reads the hash on mount and updates it on tab change.

### Content dispatch

Once a tab's content is loaded, `ProjectDetail` dispatches to the right renderer based on the schema shape:

- `entity_type` field present → mount the existing `EntityList` component
- `sections[]` with widget types → mount `ProjectOverview` (for overview schema)
- Else → placeholder "Not yet implemented"

This keeps the dispatch logic in the outer component, so the inner components don't need to know about tabs.

---

## Data flow

1. User navigates to `/projects/LA`
2. Server renders `detail.gohtml` — a thin shell with `<div data-island="project-detail" data-prop-project-id="LA">`
3. Svelte island mounts `ProjectDetail` with the project ID
4. `ProjectDetail.init()` fetches `/projects/LA/page-schema`
5. Header renders from `pageSchema.entity`, tab bar from `pageSchema.tabs`
6. Active tab is determined from URL hash or `default: true` in the schema
7. Active tab's `content_url` is fetched lazily
8. Content renders via widget dispatch
9. Tab switch: fetches new content if not cached, updates URL hash, preserves state of previously-visited tabs
10. Nav links (downloads, settings) are plain anchors — full page navigation as before

---

## Files

### Create

- Backend: `pkg/formschema/project_page.go`, `pkg/formschema/project_overview.go`
- Frontend types: `frontend/src/lib/types/project-page.ts`
- Frontend state: `frontend/src/lib/stores/project-detail.svelte.ts`
- Frontend components: `frontend/src/lib/components/project/*.svelte` (6 files)
- Frontend island: `frontend/src/islands/project-detail.ts`

### Modify

- `pkg/handlers/projects.go` — migrate `ProjectDetailPage` to WeaveStore, add schema handlers
- `pkg/handlers/fields.go` — migrate `ProjectFieldsAPI` to WeaveStore
- `pkg/router/router.go` — register new schema endpoints
- `pkg/assets/templates/pages/projects/detail.gohtml` — shrink to ~50-line shell

### Success criteria

1. `/projects/LA` loads with the Svelte island (no vanilla JS)
2. Overview tab shows README, stats cards, and info sidebar
3. Models, Collections, Fields tabs show entity-list islands with real data from WeaveStore
4. Fields tab data API returns actual fields (not empty)
5. Tab switching is instant after first load (cached)
6. URL hash persists active tab across reloads
7. `detail.gohtml` is under 100 lines
8. All GORM references removed from project detail handler
9. Settings nav link is hidden for users without admin rights on the project
