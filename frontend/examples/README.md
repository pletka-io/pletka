# Platform Frontend Manifest Example

This directory is a copyable fixture for platform/private frontend
contributions. It is not part of the active core build graph, but Go tests load
the manifest and validate that the referenced source files exist.

Use this shape when a platform repo needs to add or override schema-driven
frontend assets:

- add new islands with `islands`
- add reusable form widgets with `formWidgets`
- add entity-list row renderers with `entityListRowWidgets`
- override core entries only with an explicit `overrides: "core"`

Backend code that emits these names must use `frontendrefs.*(...)` markers, for
example:

```go
frontendrefs.Island("platform-dashboard")
frontendrefs.FormWidget("platform-lookup")
frontendrefs.EntityListRowWidget("platform-card")
```

Required conformance check:

```bash
go test ./pkg/frontendmanifest
```
