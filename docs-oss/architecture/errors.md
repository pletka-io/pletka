# Errors

Two rules govern errors in Pletka:

1. **One wire shape.** Every JSON error response in the weave API has the same
   envelope, built by one package.
2. **Explicit over heuristic.** Pass real context; never guess it. Fail loud when
   context is missing rather than inferring it from names or types.

## The canonical envelope: `apierror`

`pkg/weave/apierror` is the single source of truth for error responses. Handlers
do not build `map[string]any` envelopes inline, and slices do not carry their own
`writeError` helpers. The wire shape:

```json
{
  "code": "validation",
  "error": "validation error",
  "message": "Please correct the highlighted fields and try again.",
  "errors": { "field_name": ["message 1", "message 2"] }
}
```

- `error` — human summary (always present).
- `code` — a machine-readable kind the frontend can branch on instead of parsing
  message text. The constant list **is** the contract: `bad_request`,
  `unauthorized`, `forbidden`, `not_found`, `conflict`, `validation`, `in_use`,
  `internal`, `method_not_allowed`, `unprocessable`.
- `message` — optional longer help (the FormRenderer banner).
- `errors` — per-field messages; what FormRenderer renders inline.

### Constructors and status codes

Build errors with the constructors, never by hand:

| Constructor | Status | When |
|---|---|---|
| `BadRequest(msg)` | 400 | malformed request |
| `Unauthorized()` | 401 | not authenticated |
| `Forbidden(msg)` | 403 | authenticated but not allowed |
| `NotFound(msg)` | 404 | resource absent |
| `Conflict(msg)` | 409 | duplicate, optimistic-lock failure |
| `InUse(msg)` | 409 | delete blocked — dependents exist (distinct `in_use` code so the frontend can show dependent-count UI) |
| `Validation(fields)` | 422 | per-field validation errors |
| `Internal()` / `InternalWith(msg)` | 500 | unexpected; body never leaks internal detail |

`apierror.Write(w, e)` emits the envelope at the right status with the JSON
content-type. Successful mutations follow the matching codes — 201 create, 200
update, 204 delete/reorder (see [`schema-driven-ui.md`](schema-driven-ui.md)).

### Slices opt in, they don't import

A slice's typed errors join the mapping by satisfying a small adapter interface —
a one- or two-line method on the existing error struct, with no import dependency
on `apierror`:

```go
// pkg/weave/category/apierror_adapters.go
func (e *ErrValidation) ValidationFields() map[string][]string { return e.Fields }
func (e *ErrForbidden) IsForbidden() bool                      { return true }
```

`apierror.FromError(err)` walks these adapters with `errors.As` and maps to the
right `Error`, falling through to `Internal()` for unknown types. Order matters —
validation first, in-use before generic conflict — so each case keeps its
dedicated code. The handler still logs the original error separately;
`FromError` never puts the underlying Go error in the response body.

## Explicit over heuristic

The second rule is a design stance, not a package. **Deterministic context beats
runtime guessing.** When code needs to know something — a namespace, a prefix, an
entity's type, a scope — it should be given that context explicitly and fail
loudly if it is missing. It should not infer it from a name, a string prefix, or a
shape that "usually" means one thing.

Why: heuristics pass in the common case and fail silently at the edges. A prefix
guesser that is right 95% of the time produces wrong output 5% of the time with no
error — the worst failure mode, because nothing tells you. Explicit context either
works or fails at the boundary where you can see it.

Concretely this looks like:

- pass an immutable, fully-resolved snapshot down a render tree rather than letting
  each node re-derive scope;
- require a namespace/prefix to be supplied, returning an error when it is absent,
  instead of inferring one from a class name;
- key behaviour off an explicit type or capability, never off a parsed identifier.

This is why a recent refactor removed ~200 lines of namespace-prefix guessing in
the Arches generator and replaced it with an explicit snapshot-based namespace
manager. The principle is recorded in
[ADR-0003](../decisions/0003-explicit-errors-over-heuristics.md) and listed among
the core [principles](../principles.md).

A generator renderer takes a fully resolved, immutable `Snapshot` and writes
output — it makes no database calls and guesses nothing. The resolution happened
once, upstream, where the context was known.
