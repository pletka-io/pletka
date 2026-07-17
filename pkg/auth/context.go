package auth

import "context"

type ctxKey struct{}

var snapshotKey = ctxKey{}
var principalKey = struct{ name string }{name: "principal"}

// WithSnapshot returns ctx with s attached. Middleware calls this exactly once
// per request; handlers never call it.
func WithSnapshot(ctx context.Context, s *AuthSnapshot) context.Context {
	return context.WithValue(ctx, snapshotKey, s)
}

// FromContext returns the AuthSnapshot attached to ctx. When no snapshot is
// present (e.g. a request that bypassed the middleware), returns a zero-value
// anonymous snapshot so callers never have to nil-check.
func FromContext(ctx context.Context) *AuthSnapshot {
	if s, ok := ctx.Value(snapshotKey).(*AuthSnapshot); ok && s != nil {
		return s
	}
	return &AuthSnapshot{IsAnonymous: true}
}

// WithPrincipal returns ctx with p attached. Middleware calls this once per
// request when a user session resolves to a weave actor profile.
func WithPrincipal(ctx context.Context, p *Principal) context.Context {
	return context.WithValue(ctx, principalKey, p)
}

// PrincipalFromContext returns the request-scoped Principal or nil when the
// request is anonymous.
func PrincipalFromContext(ctx context.Context) *Principal {
	if p, ok := ctx.Value(principalKey).(*Principal); ok && p != nil {
		return p
	}
	return nil
}
