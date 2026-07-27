package auth

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/errresp"
)

type projectVersionReader interface {
	GetByIDVersion(ctx context.Context, id, version string) (*domain.Project, error)
}

// ProjectReader is the minimal project lookup contract required by
// project-scoped auth middleware.
type ProjectReader interface {
	GetByID(ctx context.Context, id string) (*domain.Project, error)
}

// projectKey is the context key under which the per-request domain.Project
// is stored. Unexported so callers go through ProjectFromContext /
// ProjectResourceFromContext.
type projectKey struct{}

// WithProject returns ctx with p attached. Used both by the
// WithProjectResource middleware and by tests that bypass the middleware.
func WithProject(ctx context.Context, p *domain.Project) context.Context {
	return context.WithValue(ctx, projectKey{}, p)
}

// ProjectFromContext returns the domain.Project attached by
// WithProjectResource for the current request, or nil when no project
// is in scope (e.g. routes without a {projectID} URL parameter).
func ProjectFromContext(ctx context.Context) *domain.Project {
	if p, ok := ctx.Value(projectKey{}).(*domain.Project); ok && p != nil {
		return p
	}
	return nil
}

// ProjectResource constructs a project-scoped auth resource from p.
func ProjectResource(p *domain.Project) Resource {
	if p == nil {
		return Resource{ScopeType: "project"}
	}
	visibility := "private"
	if p.Visibility != "" {
		visibility = p.Visibility
	}
	return Resource{
		ScopeType:  "project",
		ID:         p.ID,
		OrgID:      p.OwnerID,
		Visibility: visibility,
	}
}

// ProjectResourceFromContext returns the auth.Resource for the project
// in scope. Visibility is populated from the loaded project so callers
// don't have to re-load and don't have to remember to set it.
//
// When no project is in context (route doesn't carry {projectID} or the
// middleware was skipped), the returned Resource has only ScopeType set —
// snap.Can() against this will fail closed for any project capability.
func ProjectResourceFromContext(ctx context.Context) Resource {
	return ProjectResource(ProjectFromContext(ctx))
}

// WithProjectResource is the chi middleware that loads the project
// referenced by the {projectID} URL parameter once per request and
// attaches it to context. Slice service gates then read the populated
// Resource via ProjectResourceFromContext instead of constructing it
// (incorrectly) from the projectID alone.
//
// Routes that don't carry a {projectID} parameter pass through
// untouched — the middleware no-ops. Routes that do but reference a
// missing project return 404 immediately, before any handler runs.
//
// Apply at the slice mount boundary (see pkg/weave/router.mountSlice)
// so every project-scoped slice picks it up uniformly. Drafts mount
// directly at /api/v1/drafts and don't use this middleware — they
// load the project from the request body themselves.
func WithProjectResource(weave domain.WeaveStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			projectID := chi.URLParam(r, "projectID")
			if projectID == "" {
				next.ServeHTTP(w, r)
				return
			}
			version := ProjectVersionFromContext(r.Context())
			if version == "" {
				version = r.URL.Query().Get("version")
			}
			projects := weave.Projects()
			project, err := projects.GetByID(r.Context(), projectID)
			if version != "" {
				if vr, ok := projects.(projectVersionReader); ok {
					project, err = vr.GetByIDVersion(r.Context(), projectID, version)
				}
			}
			if err != nil {
				http.Error(w, "failed to load project", http.StatusInternalServerError)
				return
			}
			if project == nil {
				http.Error(w, "Project not found", http.StatusNotFound)
				return
			}
			next.ServeHTTP(w, r.WithContext(WithProject(r.Context(), project)))
		})
	}
}

// RequireProjectRead gates a route on ProjectRead and attaches the loaded
// project to context for downstream handlers.
func RequireProjectRead(projects ProjectReader) func(http.Handler) http.Handler {
	return requireProject(projects, ProjectRead)
}

// RequireProjectEdit gates a route on ProjectEdit and attaches the loaded
// project to context for downstream handlers.
func RequireProjectEdit(projects ProjectReader) func(http.Handler) http.Handler {
	return requireProject(projects, ProjectEdit)
}

// WrapProjectRead adapts a handler function behind RequireProjectRead.
func WrapProjectRead(projects ProjectReader, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		RequireProjectRead(projects)(next).ServeHTTP(w, r)
	}
}

// WrapProjectEdit adapts a handler function behind RequireProjectEdit.
func WrapProjectEdit(projects ProjectReader, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		RequireProjectEdit(projects)(next).ServeHTTP(w, r)
	}
}

func requireProject(projects ProjectReader, capability Capability) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			projectID := chi.URLParam(r, "projectID")
			if projectID == "" {
				notFound(w, r)
				return
			}
			version := ProjectVersionFromContext(r.Context())
			if version == "" {
				version = r.URL.Query().Get("version")
			}
			project, err := projects.GetByID(r.Context(), projectID)
			if version != "" {
				if vr, ok := projects.(projectVersionReader); ok {
					project, err = vr.GetByIDVersion(r.Context(), projectID, version)
				}
			}
			if err != nil || project == nil {
				notFound(w, r)
				return
			}
			if !FromContext(r.Context()).Can(capability, ProjectResource(project), nil) {
				notFound(w, r)
				return
			}
			next.ServeHTTP(w, r.WithContext(WithProject(r.Context(), project)))
		})
	}
}

// notFound emits the existence-hiding 404 through the request's negotiated
// responder (branded page for HTML, envelope for JSON), falling back to the
// bare 404 when no responder is stashed (e.g. a route mounted outside the
// weave router).
func notFound(w http.ResponseWriter, r *http.Request) {
	if resp, ok := errresp.FromContext(r.Context()); ok {
		resp(w, r, http.StatusNotFound, "not_found", "not found")
		return
	}
	http.NotFound(w, r) //nolint:forbidigo // fallback outside the weave router
}
