package auth

import (
	"context"
	"net/http"
)

type projectVersionKey struct{}

// WithProjectVersion stores the requested project version (typically from
// ?version=...) in the request context. Empty means "hot draft view".
func WithProjectVersion(ctx context.Context, version string) context.Context {
	return context.WithValue(ctx, projectVersionKey{}, version)
}

// ProjectVersionFromContext returns the requested released version for the
// current request, or "" when the request targets the hot draft view.
func ProjectVersionFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(projectVersionKey{}).(string); ok {
		return v
	}
	return ""
}

// WithProjectVersionContext attaches the ?version= query value to context and
// rejects mutating requests when a released version is in scope.
func WithProjectVersionContext(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		version := r.URL.Query().Get("version")
		if version == "" {
			next.ServeHTTP(w, r)
			return
		}
		if !isSafeMethod(r.Method) {
			http.Error(w, "versioned project views are read-only", http.StatusMethodNotAllowed)
			return
		}
		next.ServeHTTP(w, r.WithContext(WithProjectVersion(r.Context(), version)))
	})
}

func isSafeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	default:
		return false
	}
}
