# Go style

The codebase follows a consistent Go style — the same for human- and AI-written
code. The rules below are the ones that come up most; they are non-negotiable for
new code.

## Core

- **Glanceability over cleverness.** Code is read far more than written. Optimise
  for the reader scanning at a glance.
- **Happy path hugs the left margin.** Guard clauses return early; errors and edge
  cases are handled first and exited. The success flow is never nested in `else`.
- **Open-source standard.** Every exported symbol is understandable without reading
  its implementation. Godoc on all exported symbols — complete sentence, starts
  with the symbol name.

## Errors

- Wrap with `%w` and a lowercase context prefix: `fmt.Errorf("parse manifest: %w", err)`.
- Error strings are lowercase, no trailing punctuation.
- Never discard an error silently; if intentional, say why: `_ = cache.Invalidate(k) // best-effort`.
- Custom error types for domain errors callers must distinguish; plain `%w` for
  transport/infra errors. In the weave HTTP layer, map to the canonical envelope —
  see [`../architecture/errors.md`](../architecture/errors.md).

## Structure

- `context.Context` is always the first parameter, named `ctx`. Never store it in
  a struct.
- No global state — pass loggers, pools, and config as dependencies. The only
  acceptable globals are constants and `var ErrXxx = errors.New(…)` sentinels.
- No `init()` functions; the codebase complies (zero `func init()`).
- No `panic` on a request path — return errors. Wiring-time fail-fast panics
  are sanctioned: route registration (`Mount(...)`), constructor
  nil-dependency guards, and `mustJSON` over static data. These run once at
  boot, before any request is served, so a broken wire-up fails immediately
  and loudly instead of surfacing as a 500 later. `Must*` constructors are
  otherwise confined to `main` or test setup.
- Define interfaces at the point of use (consumer side), not next to the
  implementation. Don't create speculative interfaces — a concrete type with no
  interface is correct until a second implementation or a test stub needs one.
- Prefer composition (embedding) over inheritance.

## Logging

- `log/slog` only. Never `fmt.Println` or `log.Printf` for operational output.
- Key-value attributes, not formatted strings. Error key is always `"err"`
  (not `"error"`).
- Prefer the `*Context` variants (`InfoContext`, `ErrorContext`) so trace
  context propagates — but this is aspirational, not a hard gate: current code
  predominantly uses the plain variants (`Info`, `Warn`, `Error`). Known debt;
  don't block a review on it, but use the `*Context` form in new call sites
  where a `context.Context` is already in scope.

## Testing

- `github.com/google/go-cmp/cmp` for comparisons — it produces structured diffs.
  Never `reflect.DeepEqual`; not testify.
  `if diff := cmp.Diff(want, got); diff != "" { t.Errorf(…) }`.
- Table-driven tests where there are multiple meaningful cases; a direct test for
  single-path functions. Subtest names read as sentences.

## Naming

Follows the boundary conventions in
[`naming-conventions.md`](naming-conventions.md): Go PascalCase exports,
camelCase locals, acronyms all-caps (`recordID`, `parseRDF`, `httpClient`).

Data-access types in a slice are named `Store` / `NewPostgresStore` (not `Repo`) —
the slice owns its store. Services are `*Service` / `NewService`, handlers
`*Handler` / `NewHandler`.

## Avoid

`else` after `return`; `if/else if` chains on a value (use `switch` with a
`default`); `any` without a strong reason; catch-all `util.go` / `helpers.go`
files (name files after what they contain); logic in `func main()` (delegate to a
`run` function or `cmd.Execute()`).
