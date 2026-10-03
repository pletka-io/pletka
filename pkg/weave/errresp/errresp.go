// Package errresp carries a request-scoped error responder on the context so
// any handler — including ones in packages that cannot import
// pkg/weave/router (e.g. pkg/auth, which router imports) — can emit a
// consistent negotiated error. pkg/weave/router installs the concrete
// responder; consumers read it via FromContext.
package errresp

import (
	"context"
	"net/http"
	"sync/atomic"
)

// Responder writes a negotiated error (HTML branded page or JSON envelope)
// for the given status/code/message.
type Responder func(w http.ResponseWriter, r *http.Request, status int, code, message string)

type ctxKey struct{}

// WithResponder returns ctx carrying resp.
func WithResponder(ctx context.Context, resp Responder) context.Context {
	return context.WithValue(ctx, ctxKey{}, resp)
}

// FromContext returns the responder stashed by the router, if any.
func FromContext(ctx context.Context) (Responder, bool) {
	resp, ok := ctx.Value(ctxKey{}).(Responder)
	return resp, ok
}

// Error emits a negotiated error (branded HTML shell or JSON envelope) using
// the request-scoped responder the router stashed. The one-line replacement
// for bare http.Error in weave handlers — usable from packages that cannot
// import pkg/weave/router (which imports the slices). Falls back to plain
// http.Error only if no responder is on the context.
func Error(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	if resp, ok := FromContext(r.Context()); ok {
		resp(w, r, status, code, message)
		return
	}
	http.Error(w, message, status) //nolint:forbidigo // fallback when unrouted
}

type causeKey struct{}

// CauseSlot holds the underlying error behind a request's 5xx so the error
// tracker, which only sees the generic response body, can record why.
type CauseSlot struct{ err error }

// Err returns the recorded cause, or nil.
func (s *CauseSlot) Err() error { return s.err }

// WithCauseSlot returns ctx carrying a fresh slot, and the slot itself for
// the caller (the error tracker) to read after the handler returns.
func WithCauseSlot(ctx context.Context) (context.Context, *CauseSlot) {
	s := &CauseSlot{}
	return context.WithValue(ctx, causeKey{}, s), s
}

// RecordCause stores err as the cause of the request's failure. The first
// recorded cause wins; no-op without a slot on ctx.
func RecordCause(ctx context.Context, err error) {
	if s, ok := ctx.Value(causeKey{}).(*CauseSlot); ok && s.err == nil {
		s.err = err
	}
}

// Holder carries the process-wide error Responder, set once after the
// error-page host is assembled and read by StashMiddleware on every request.
// Lets the stash middleware install before any routes (avoiding chi's
// "middleware after routes" panic) while the concrete responder is wired later.
type Holder struct{ v atomic.Pointer[Responder] }

// Set publishes the responder (safe to call once at assembly time).
func (h *Holder) Set(r Responder) { h.v.Store(&r) }

// StashMiddleware puts the holder's current responder on the request context,
// if one is set. Before Set (or with a nil holder), it is a passthrough — the
// weaverouter.Error / notFound fallbacks handle the absent case.
func StashMiddleware(h *Holder) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if h != nil {
				if p := h.v.Load(); p != nil {
					r = r.WithContext(WithResponder(r.Context(), *p))
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}
