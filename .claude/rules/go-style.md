# Go Style — Design Contract

Read `docs-oss/reference/go-style.md` before writing or reviewing any Go code — it is
the authoritative style guide. Read `docs-oss/architecture/errors.md` for the
canonical error envelope and the explicit-over-heuristic stance.

## Core Principles

- **Guard clauses**: Return early for error/edge cases. No deep nesting.
- **Error handling**: Lowercase messages, wrap with `fmt.Errorf("context: %w", err)`. Never ignore errors silently.
- **Logging**: Use `log/slog` with structured fields. No `log.Printf` or `fmt.Println` for logging.
- **Testing**: Use `go-cmp` for structural comparisons. Table-driven tests where appropriate.
- **Naming**: Follow Go conventions (PascalCase exports, camelCase locals). See `docs-oss/reference/naming-conventions.md` for project-specific naming.
- **No global state**: Pass dependencies explicitly via constructors or function parameters.
- **Context**: `context.Context` is always the first parameter, prefer the `*Context` slog variants (`InfoContext`, `ErrorContext`) where a context is already in scope — this is aspirational, not a hard gate; current code predominantly uses the plain variants and that's known debt, not a review blocker.

## Quick Reference

| Pattern | Do | Don't |
|---------|-----|-------|
| Error wrapping | `fmt.Errorf("fetch user: %w", err)` | `fmt.Errorf("Error: %s", err.Error())` |
| Guard clause | `if err != nil { return err }` | `if err == nil { /* happy path */ }` |
| Logging | `slog.Error("query failed", "err", err)` | `log.Printf("ERROR: %v", err)` |
| Test comparison | `if diff := cmp.Diff(want, got); diff != ""` | `if !reflect.DeepEqual(want, got)` |
| Nil check | `if x == nil { return nil, nil }` | Omitting nil checks on pointer returns |

## Constraints

- All exported types and functions must have doc comments (complete sentence, starts with symbol name)
- Errors are lowercase, no punctuation at end
- No `panic` on a request path — return errors. Wiring-time fail-fast panics are a
  sanctioned carve-out: route registration (`Mount(...)`), constructor
  nil-dependency guards, and `mustJSON` over static data. These run once at boot,
  before any request is served, so a broken wire-up fails immediately instead of
  surfacing as a 500 later. `Must*` constructors are otherwise confined to `main`
  or test setup.
- No `init()` functions — explicit initialization only
- Prefer composition over inheritance (embedding)
