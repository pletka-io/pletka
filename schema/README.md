# Schema

The Pletka schema is the JSON contract between the server and the renderer.

There is no separate schema language. The Go server emits a documented JSON shape; the Svelte renderer reads it. This directory holds:

- **`README.md`** — this file: the protocol at a high level.
- **`version.txt`** — current schema version (semantic versioning, tracked independently of the Go module).
- **`CHANGELOG.md`** — schema-specific changelog.
- **`envelopes/`** — response and request envelope shapes (page, error, pagination, validation errors).
- **`widgets/`** — one Markdown spec per widget type, describing fields, semantics, and an example.
- **`relations/`** — one Markdown spec per link relation (`edit`, `filter`, `next-page`, ...) describing meaning, HTTP method, and request / response shape.
- **`examples/`** — full JSON fixtures used as executable specs. Both server and renderer have tests that load every file in `examples/` and verify behaviour.

## Authorization variants in fixtures

Because authorization is hypermedia-driven (`docs/authorization-model.md`), every resource that supports multiple permission levels has multiple fixtures:

```
schema/examples/
├── article-anonymous.json
├── article-reader.json
├── article-editor.json
└── article-admin.json
```

The diff between these fixtures **is** the authorization spec. It also serves as the conformance suite for alternative renderers.

## Versioning

- **Minor** versions add new fields, widgets, or link relations. Existing renderers continue to work (they ignore unknown fields).
- **Major** versions remove or change existing fields. Renderers must be updated.
- The Go module is versioned independently — see the top-level `CHANGELOG.md`.

## Adding a widget or relation

See [`docs/adding-a-widget.md`](../docs/adding-a-widget.md) and [`docs/adding-a-relation.md`](../docs/adding-a-relation.md).
