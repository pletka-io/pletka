# ADR-0009: Server errors go through one door that takes the cause

Date: 2026-10-03  
Status: Accepted

---

## Why this decision was needed

The error tracker (`weave_error_events`) recorded 5xx responses without their
cause. Handlers logged the underlying error and then wrote a generic
"internal error" body, so the tracker only ever saw that body. On top of that
it captured gzip bytes (it sat inside response compression) and no actor (auth
runs inside it). About 140 call sites wrote 500s in several different ways.
Fixing them one by one would not stop the next site from repeating the gap.

## What we decided

A 5xx response can only be written through a function that requires the
causing `error`. The function records the error on the request, and the
tracker stores it as `error.cause`. Three parts:

1. **Request slots, installed by the tracker.** `errortracking.Middleware`
   puts a cause slot (`errresp.WithCauseSlot`) and an actor slot on the
   request context before calling the next handler. `errresp.RecordCause`
   fills the cause slot; the first recorded cause wins.
   `errortracking.CaptureActor`, mounted right after auth, fills the actor
   slot. The tracker sits inside `Compress`, so it sees the plain body.
2. **The doors.** `errresp.Internal(w, r, err)` / `errresp.InternalWith(w, r,
   err, msg)` for negotiated responses; `apierror.Write(w, r,
   apierror.Internal(err))` / `apierror.InternalWith(msg, err)` for the JSON
   envelope; `RespondInternalError(..., err)` for the branded HTML page;
   `recoverMiddleware` records panics. Every one of them takes `err`, so the
   compiler flags a call without one.
3. **The ban.** forbidigo forbids `http.StatusInternalServerError`,
   `StatusNotImplemented`, `StatusBadGateway`, `StatusServiceUnavailable` and
   `StatusGatewayTimeout` outside the door packages (`errresp`, `apierror`)
   and tests. A deliberate non-failure 5xx (a health check reporting 503)
   needs `//nolint:forbidigo` with a reason.

The cause is never sent to the client. Handlers keep logging as they did
before; the door only records the cause, it doesn't replace the log line.

## What we considered and rejected

- **Capture from logging:** a `slog.Handler` that records any
  `ErrorContext(ctx, …)` call as the cause. No new API, and logging the
  error would be enough. Rejected because most error logs here don't pass a
  context (51 of the sites did), so every site would need editing anyway.
  It also turns any error log on a request that ends in a 4xx into that
  request's "cause", and the rule is invisible in the handler code.
- **Handlers that return `error`:** `func(w, r) error` plus one adapter that
  maps the error to a status, logs it and records it. This is the strongest
  form: a 5xx without a cause can't even be expressed. Rejected for this
  codebase only because it changes the signature of every handler. Prefer it
  for new codebases.
- **Recording the cause at each site without a door:** add
  `RecordCause(ctx, err)` next to each existing 500. Smallest diff, but
  nothing enforces it, so new sites drift back.

## Consequences

- **Easier:** every 5xx row in the tracker carries why it happened and who
  hit it; triage no longer needs server logs.
- **Harder:** writing a 500 requires an error value. Where there is none (an
  impossible state) the site has to build one, which is usually the right
  thing anyway.
- **Constrained:** new response helpers that can emit 5xx must take `err`
  and record it. A bare `w.WriteHeader(500)` with a numeric literal gets
  past the lint rule; reviews should reject it.

**Copying this to another codebase:** take the three parts above (per-request
slot installed by the error tracker, one function for 5xx that requires the
error, a lint rule banning the other paths). None of them is specific to Go
or to this app.

## References

- Related ADRs: ADR-0003 (explicit errors over heuristics), ADR-0007
- Code: `pkg/weave/errresp`, `pkg/weave/apierror`, `pkg/weave/errortracking`,
  `pkg/app/root.go`, `.golangci.yml` (forbidigo)
