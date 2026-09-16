package auth

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/errresp"
)

type projectVersionKey struct{}

// LatestReleaseReader resolves the highest-semver release version for a
// project, or "" when it has none. Injected so pkg/auth needn't import the
// release/publication layer.
type LatestReleaseReader interface {
	LatestReleaseVersion(ctx context.Context, projectID string) (string, error)
}

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
			errresp.Error(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "versioned project views are read-only")
			return
		}
		next.ServeHTTP(w, r.WithContext(WithProjectVersion(r.Context(), version)))
	})
}

// ResolveEffectiveVersion decides which content version a read should serve.
// explicitVersion is the ?version= value (wins for any reader with ProjectRead).
// latestRelease is the project's highest-semver release ("" if none), supplied
// by the caller. See the visibility design doc for the contract.
func ResolveEffectiveVersion(snap *AuthSnapshot, project *domain.Project, explicitVersion, latestRelease string) string {
	if explicitVersion != "" {
		return explicitVersion
	}
	if project == nil {
		return ""
	}
	if project.Visibility != domain.VisibilityPublic {
		return "" // internal/private: only members read here; members see hot
	}
	if snap.Can(ProjectEdit, ProjectResource(project), nil) {
		return "" // public editor sees the working state
	}
	return latestRelease // public non-editor: latest release ("" ⇒ hot fallback)
}

// ResolveContentVersion sets the effective project version on the request
// context when none was given explicitly, so non-editor viewers of a public
// project read the latest release instead of the draft. Install AFTER
// auth.WithProjectResource (needs the loaded project + snapshot). Safe methods
// only — mutations always run on the hot state.
func ResolveContentVersion(reader LatestReleaseReader) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			if ProjectVersionFromContext(ctx) != "" || r.URL.Query().Get("version") != "" || !isSafeMethod(r.Method) {
				next.ServeHTTP(w, r) // explicit version, or a mutation: leave as-is
				return
			}
			project := ProjectFromContext(ctx)
			if project == nil || project.Visibility != domain.VisibilityPublic {
				next.ServeHTTP(w, r)
				return
			}
			snap := FromContext(ctx)
			if snap.Can(ProjectEdit, ProjectResource(project), nil) {
				next.ServeHTTP(w, r) // editor: hot
				return
			}
			latest, err := reader.LatestReleaseVersion(ctx, project.ID)
			if err != nil {
				slog.Default().ErrorContext(ctx, "resolve content version: latest release lookup failed; serving hot", "project", project.ID, "err", err)
				next.ServeHTTP(w, r) // fail safe to hot, never 500
				return
			}
			if v := ResolveEffectiveVersion(snap, project, "", latest); v != "" {
				ctx = WithProjectVersion(ctx, v)
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func isSafeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	default:
		return false
	}
}
