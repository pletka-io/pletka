// Package errresp carries a request-scoped error responder on the context so
// any handler — including ones in packages that cannot import
// pkg/weave/router (e.g. pkg/auth, which router imports) — can emit a
// consistent negotiated error. pkg/weave/router installs the concrete
// responder; consumers read it via FromContext.
package errresp

import (
	"context"
	"net/http"
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
