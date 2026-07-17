# Frontend Contributions

Frontend extension points are registered through build-time manifests. Schemas
refer to logical names; manifests bind those names to Svelte or TypeScript
implementations. The core inventory lives in `frontend/core.frontend.json`.

Set `PLETKA_FRONTEND_MANIFESTS` to one or more platform manifest paths separated
by the platform path delimiter (`:` on Linux/macOS). Relative environment paths
are resolved from `frontend/`, matching Vite's config directory. Manifest-
relative `source` paths are resolved from the manifest file directory. The older
`PLETKA_FRONTEND_CONTRIBUTIONS` variable is still accepted as an alias.
Widget registrations are generated into the global frontend entry, so schema
renderers can use contributed widget names without custom imports in each
island.

```json
{
  "id": "platform-example",
  "name": "Platform Example",
  "description": "Platform-specific frontend entrypoints and reusable widgets.",
  "globalEntries": [],
  "islands": [
    {
      "name": "platform-dashboard",
      "source": "./src/islands/platform-dashboard.ts"
    }
  ],
  "formWidgets": [
    {
      "name": "platform-lookup",
      "source": "./src/widgets/PlatformLookup.svelte"
    }
  ],
  "entityListRowWidgets": [
    {
      "name": "platform-card",
      "source": "./src/widgets/PlatformCard.svelte"
    }
  ],
  "entityListEditorWidgets": [
    {
      "name": "platform-editor",
      "source": "./src/widgets/PlatformEditor.svelte"
    }
  ],
  "contentWidgets": [
    {
      "name": "platform-block",
      "source": "./src/widgets/PlatformBlock.svelte"
    }
  ]
}
```

Build-time validation catches missing files, invalid names, invalid file types,
and duplicate contribution names. Replacing a core island or widget must be
explicit:

```json
{
  "formWidgets": [
    {
      "name": "select",
      "source": "./src/widgets/PlatformSelect.svelte",
      "overrides": "core"
    }
  ]
}
```

Go tests load the same manifest through `pkg/frontendmanifest`, so backend
templates and schema builders fail tests when they emit an unregistered island,
global entry, or widget name.

Server startup validates the same manifest set through `pkg/assets`. Invalid
duplicates, missing source files, and manifest-declared global/island entries
that are absent from the built Vite manifest fail before the app serves pages.
Content pages loaded from the embedded bundle or `PLETKA_CONTENT_OVERLAY` are
also validated against `contentWidgets`, including implicit `prose` blocks from
Markdown body content.

## Backend References

Backend code must emit manifest-controlled names through `frontendrefs` marker
helpers. The helpers return strings, so schema JSON stays simple, but the Go
conformance tests can discover the references before runtime.

```go
page.Island = templates.IslandMount{
	Name: frontendrefs.Island("platform-dashboard"),
	Dependencies: []string{
		frontendrefs.Island("platform-dashboard"),
	},
}

schema.RowWidget = frontendrefs.EntityListRowWidget("platform-card")
```

Do not use raw string literals for registry-backed frontend names in page or
schema emitters. The core conformance test rejects raw literals for island
mounts, entity-list row widgets, view-mode widgets, and entity-list editor
widgets. Form widget constants may stay string constants, but each exported
registered widget constant needs a matching `frontendrefs.FormWidget(...)`
marker.

Required check:

```bash
go test ./pkg/frontendmanifest
```

Runtime validation without opening a database:

```bash
cd frontend
PLETKA_FRONTEND_MANIFESTS=examples/platform.frontend.json npm run build
cd ..
PLETKA_FRONTEND_MANIFESTS=examples/platform.frontend.json go run . serve --validate-only
```

When validating a platform content overlay:

```bash
PLETKA_FRONTEND_MANIFESTS=examples/platform.frontend.json PLETKA_CONTENT_OVERLAY=/path/to/content go run . serve --validate-only
```
