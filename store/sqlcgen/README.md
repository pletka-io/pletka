# sqlcgen

Generated query layer. Do not edit by hand — `sqlc generate`
overwrites this directory.

Source files driving generation:

- `sqlc.yaml` at repo root — sqlc configuration
- `store/migrations/` — schema (sqlc reads the migrations to derive
  table types)
- `store/sqlcgen/queries/*.sql` — query definitions (one file per
  subsystem: `weave.sql`, `ontology.sql`, ...)

Run generation via:

```sh
go tool sqlc generate
```

Always use `go tool sqlc`, not a globally-installed binary — the
correct version is pinned in `go.mod` as a tool dependency.
Generated code lands in this directory and is committed.
