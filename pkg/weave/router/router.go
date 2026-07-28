package router

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/i18n"
	integrationsregistry "github.com/pletka-io/pletka/pkg/integrations/registry"
	"github.com/pletka-io/pletka/pkg/weave/actoradmin"
	"github.com/pletka-io/pletka/pkg/weave/admin"
	"github.com/pletka-io/pletka/pkg/weave/apierror"
	"github.com/pletka-io/pletka/pkg/weave/apikey"
	"github.com/pletka-io/pletka/pkg/weave/attribution"
	"github.com/pletka-io/pletka/pkg/weave/authpages"
	"github.com/pletka-io/pletka/pkg/weave/category"
	"github.com/pletka-io/pletka/pkg/weave/collection"
	"github.com/pletka-io/pletka/pkg/weave/detailview"
	"github.com/pletka-io/pletka/pkg/weave/drafts"
	"github.com/pletka-io/pletka/pkg/weave/entityschema"
	"github.com/pletka-io/pletka/pkg/weave/errortracking"
	"github.com/pletka-io/pletka/pkg/weave/errresp"
	"github.com/pletka-io/pletka/pkg/weave/example"
	"github.com/pletka-io/pletka/pkg/weave/exports"
	"github.com/pletka-io/pletka/pkg/weave/field"
	"github.com/pletka-io/pletka/pkg/weave/gitrestoreadmin"
	"github.com/pletka-io/pletka/pkg/weave/health"
	"github.com/pletka-io/pletka/pkg/weave/materializationadmin"
	"github.com/pletka-io/pletka/pkg/weave/members"
	"github.com/pletka-io/pletka/pkg/weave/model"
	"github.com/pletka-io/pletka/pkg/weave/namespacebinding"
	weaveontology "github.com/pletka-io/pletka/pkg/weave/ontology"
	"github.com/pletka-io/pletka/pkg/weave/organization"
	"github.com/pletka-io/pletka/pkg/weave/orgmembers"
	"github.com/pletka-io/pletka/pkg/weave/pages"
	"github.com/pletka-io/pletka/pkg/weave/project"
	weavecsvexport "github.com/pletka-io/pletka/pkg/weave/project/csvexport"
	"github.com/pletka-io/pletka/pkg/weave/projectontologyversion"
	"github.com/pletka-io/pletka/pkg/weave/projectpage"
	"github.com/pletka-io/pletka/pkg/weave/release"
	"github.com/pletka-io/pletka/pkg/weave/search"
	"github.com/pletka-io/pletka/pkg/weave/settings"
	weavetemplates "github.com/pletka-io/pletka/pkg/weave/templates"
	"github.com/pletka-io/pletka/pkg/weave/version"
	"github.com/pletka-io/pletka/pkg/weave/visualization"
	"github.com/pletka-io/pletka/pkg/weave/vocabulary"
	"github.com/pletka-io/pletka/pkg/weave/workspace"
)

// Options are app-built slice hosts passed through the router.
type Options struct {
	Integrations           *integrationsregistry.Registry
	ActorAdmin             actoradmin.Host
	APIKey                 apikey.Host
	Attribution            attribution.Host
	AuthPages              authpages.Host
	Category               category.Host
	Collection             collection.Host
	DetailView             detailview.Host
	Drafts                 drafts.Host
	EntitySchema           entityschema.Host
	ErrorTracking          errortracking.Host
	Example                example.Host
	MaterializationAdmin   materializationadmin.Host
	Exports                exports.Host
	Field                  field.Host
	GitRestoreAdmin        gitrestoreadmin.Host
	Health                 health.Host
	Members                members.Host
	Model                  model.Host
	NamespaceBinding       namespacebinding.Host
	OntologyAdmin          weaveontology.AdminHost
	OntologyAPI            weaveontology.APIHost
	OntologyPages          weaveontology.PagesHost
	OntologyService        *weaveontology.Service
	Organization           organization.Host
	OrgMembers             orgmembers.Host
	Project                project.Host
	ProjectCSVExport       weavecsvexport.Host
	ProjectOntologyVersion projectontologyversion.Host
	ProjectPages           pages.Host
	ProjectPage            projectpage.Host
	Release                release.Host
	Search                 search.Host
	Settings               settings.Host
	Vocabulary             vocabulary.Host
	Version                version.Host
	Visualization          visualization.Host
	Workspace              workspace.Host
}

// ProjectMiddlewareHost contains the shared project middleware dependencies
// used for project-scoped route groups.
type ProjectMiddlewareHost struct {
	Weave domain.WeaveStore
}

// ErrorPageHost contains the global shell error-page dependencies.
type ErrorPageHost struct {
	Templates    *weavetemplates.Renderer
	I18n         i18n.Manager
	LangResolver func(r *http.Request) string
}

// Mount registers every weave slice on parent.
func Mount(parent chi.Router, projects ProjectMiddlewareHost, errors ErrorPageHost, options ...Options) {
	var opts Options
	if len(options) > 0 {
		opts = options[0]
	}

	actoradmin.Mount(parent, opts.ActorAdmin)
	if opts.APIKey.Service != nil {
		apikey.Mount(parent, opts.APIKey)
	}
	authpages.Mount(parent, opts.AuthPages)
	organization.Mount(parent, opts.Organization)
	orgmembers.Mount(parent, opts.OrgMembers)
	projectpage.Mount(parent, opts.ProjectPage)
	entityschema.Mount(parent, opts.EntitySchema)
	search.Mount(parent, opts.Search)
	vocabulary.Mount(parent, opts.Vocabulary)

	// Build identity — /version + /api/v1/version. Public, used by
	// the footer pill so user-test bug reports include a bisect
	// fingerprint (commit + build time + migration + frontend hash).
	version.Mount(parent, opts.Version)

	// Health probes — /healthz, /readyz, /health, /api/v1/health.
	// All return the same JSON liveness payload + ping the pgx pool.
	health.Mount(parent, opts.Health)

	// Error tracking — POST /errors/client (browser reporter) and
	// GET /admin/errors (super-admin viewer). The capture middleware
	// is wired separately in root.go's global stack.
	errortracking.Mount(parent, opts.ErrorTracking)
	// GET /admin/materialization (super-admin viewer) — async save cost + health.
	materializationadmin.Mount(parent, opts.MaterializationAdmin)
	gitrestoreadmin.Mount(parent, opts.GitRestoreAdmin)

	if err := pages.Mount(parent, opts.ProjectPages); err != nil {
		panic(err)
	}

	if errors.Templates != nil && projects.Weave != nil {
		workspace.Mount(parent, opts.Workspace)
	}

	mountSlice(parent, "/projects/{projectID}/categories", projects, func(r chi.Router) {
		mountCategory(r, opts.Category)
	})

	mountSlice(parent, "/projects/{projectID}/settings/attributions", projects, func(r chi.Router) {
		mountAttribution(r, opts.Attribution)
	})

	mountSlice(parent, "/projects/{projectID}/namespace-bindings", projects, func(r chi.Router) {
		mountNamespaceBinding(r, opts.NamespaceBinding)
	})

	mountSlice(parent, "/projects/{projectID}/members", projects, func(r chi.Router) {
		members.Mount(r, opts.Members)
	})

	mountSlice(parent, "/projects/{projectID}/project-ontology-versions", projects, func(r chi.Router) {
		projectontologyversion.Mount(r, opts.ProjectOntologyVersion)
	})

	mountSlice(parent, "/projects/{projectID}/releases", projects, func(r chi.Router) {
		release.Mount(r, opts.Release)
	})

	mountSlice(parent, "/projects/{projectID}/examples", projects, func(r chi.Router) {
		example.Mount(r, opts.Example)
	})

	mountSlice(parent, "/projects/{projectID}/fields", projects, func(r chi.Router) {
		field.Mount(r, opts.Field)
	})

	mountSlice(parent, "/projects/{projectID}/models", projects, func(r chi.Router) {
		model.Mount(r, opts.Model)
	})

	mountSlice(parent, "/projects/{projectID}/collections", projects, func(r chi.Router) {
		collection.Mount(r, opts.Collection)
	})

	// Project CSV export — verification dump. Two slices share the
	// /projects/{projectID}/exports prefix:
	//   - csvexport: HTML index + 8 typed project-wide CSVs + all.zip
	//   - exports:  per-entity CSV at /{kind}/{id}.csv
	// Both gate on project membership (Phase 1 of the CSV export/
	// restore plan).
	mountSlice(parent, "/projects/{projectID}/exports", projects, func(r chi.Router) {
		weavecsvexport.Mount(r, opts.ProjectCSVExport)
		exports.Mount(r, opts.Exports)
	})

	// Backward-compat: the legacy /projects/{id}/downloads URL the
	// migration team trained against now redirects to the new
	// /exports surface. Drop after one release cycle.
	parent.Get("/projects/{projectID}/downloads", redirectToExports)
	parent.Get("/projects/{projectID}/downloads/", redirectToExports)
	parent.Get("/projects/{projectID}/downloads/*", redirectToExports)

	// Drafts — POST /api/v1/drafts (unified inline-create endpoint).
	// Mounted directly on parent because the project ID is in the body,
	// not the URL.
	drafts.Mount(parent, opts.Drafts)

	// Visualization — generator-output endpoints under /gen/. Mounted
	// directly on parent (entity IDs are global, no projectID in URL);
	// the slice's handlers do project-read auth gating themselves.
	visualization.Mount(parent, opts.Visualization)

	// Settings — settings-v2 surface mounted under
	// /projects/{projectID}/settings/. Each sub-path mounts via chi.Mount.
	settings.Mount(parent, opts.Settings)

	// Project slice mounts directly on parent (it owns several sibling
	// sub-paths under /projects, not a single mount point).
	project.Mount(parent, opts.Project)

	// Master ontology slice. One app-built Service backs admin CRUD,
	// API autocomplete/labels, public browse pages, and detailview
	// preloading so there is exactly one IndexCache + DispatchEngine.
	ontologySvc := opts.OntologyService
	if ontologySvc == nil {
		panic("router: ontology service is required")
	}

	// Mount detailview after the ontology service is fully wired so app
	// composition can pass the shared autocomplete preloader to the slice host.
	if err := detailview.Mount(parent, opts.DetailView); err != nil {
		panic(err)
	}

	adminNamespaces := chi.NewMux()
	adminNamespaces.Use(admin.RequireSuperAdmin)
	mountNamespaceBindingAdmin(adminNamespaces, opts.NamespaceBinding)
	parent.Mount("/admin/namespaces", adminNamespaces)

	adminOntologies := chi.NewMux()
	adminOntologies.Use(admin.RequireSuperAdmin)
	weaveontology.MountAdmin(adminOntologies, opts.OntologyAdmin)
	parent.Mount("/admin/ontologies", adminOntologies)

	// Autocomplete + ontology-labels endpoints keep their stable URLs
	// so the OntologyPathBuilder Svelte widget + ontology-labels client
	// keep working without frontend changes. Backed by the shared Service.
	weaveontology.MountAPI(parent, opts.OntologyAPI)

	// Public ontology browse pages (/ontologies/...) — Phase F2 of
	// the master ontology slice plan. Read-only views of families /
	// ontologies / versions / classes / properties, replacing the
	// gohtml templates dropped in Phase F1.
	weaveontology.MountPages(parent, opts.OntologyPages)

	// Global 404 + 405 handlers — replace chi's default plain
	// "404 page not found" / "405 method not allowed" with friendly
	// shell-rendered error pages for HTML traffic, while keeping
	// API/JSON callers on a machine-readable JSON response. Must run
	// AFTER all routes mount; chi only fires these when no registered
	// pattern matches (or matches but with a different verb).
	if errors.Templates != nil {
		parent.NotFound(errorDispatch(errors, errorKindNotFound))
		parent.MethodNotAllowed(errorDispatch(errors, errorKindMethodNotAllowed))
	}
}

// errorKind selects which renderer + JSON status the dispatcher uses.
// One dispatcher with a kind switch keeps the JSON-vs-HTML triage
// (WantsJSON) in a single place. NotFound + MethodNotAllowed are
// chi-dispatched; Forbidden + InternalError are wired by handlers
// that want a styled in-shell error page instead of plain http.Error.
type errorKind int

const (
	errorKindNotFound errorKind = iota
	errorKindMethodNotAllowed
	errorKindForbidden
	errorKindInternalError
)

// errorDispatch returns a chi NotFound / MethodNotAllowed handler.
// Pre-builds the per-request ErrorPageDeps from the error-page host, then
// delegates to one of the renderer's Respond* methods, which apply the
// JSON-vs-HTML triage themselves. This keeps the dispatch logic in one place
// (the renderer) so handlers calling Respond* directly behave identically to
// the chi-dispatched handlers here.
func errorDispatch(h ErrorPageHost, kind errorKind) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		errDeps := BuildErrorPageDeps(h, r)
		switch kind {
		case errorKindMethodNotAllowed:
			// 405 has no Renderer.Respond* shortcut because it's only
			// chi-dispatched. Emit the canonical apierror envelope on
			// JSON paths; render the in-shell page for HTML.
			if weavetemplates.WantsJSON(r) {
				apierror.Write(w, apierror.MethodNotAllowed())
				return
			}
			h.Templates.RenderMethodNotAllowed(w, r, errDeps)
		case errorKindForbidden:
			h.Templates.RespondForbidden(w, r, errDeps)
		case errorKindInternalError:
			h.Templates.RespondInternalError(w, r, errDeps)
		default:
			h.Templates.RespondNotFound(w, r, errDeps)
		}
	}
}

// BuildErrorPageDeps resolves the per-request shell context (lang,
// languages, principal, nav labels) handlers need to render an
// in-shell error page. Exposed publicly so individual handlers can
// build it once at the top of their function and pass it to
// Templates.Respond* — keeps the wiring consistent with the chi
// dispatcher above.
func BuildErrorPageDeps(h ErrorPageHost, r *http.Request) weavetemplates.ErrorPageDeps {
	lang := "en"
	if h.LangResolver != nil {
		lang = h.LangResolver(r)
	}
	var languages []i18n.Language
	if h.I18n != nil {
		languages = h.I18n.Languages()
	}
	deps := weavetemplates.ErrorPageDeps{
		Lang:      lang,
		Languages: languages,
		Principal: auth.PrincipalFromContext(r.Context()),
		RequestID: chimiddleware.GetReqID(r.Context()),
	}
	if h.Templates != nil {
		deps.Labels = h.Templates.ShellLabels(lang)
	}
	return deps
}

// RespondForbidden is the public helper handlers call when they
// detect an auth-deny on a surface where 403 (rather than the
// existence-hiding 404) is appropriate. Same JSON-vs-HTML triage as
// the chi dispatcher.
//
// Usage:
//
//	if !snap.Can(cap, res, nil) {
//	    weaverouter.RespondForbidden(errors, w, r)
//	    return
//	}
func RespondForbidden(h ErrorPageHost, w http.ResponseWriter, r *http.Request) {
	errorDispatch(h, errorKindForbidden)(w, r)
}

// RespondInternalError is the in-shell counterpart to http.Error(...,
// 500). Caller logs the actual error first; this function only
// renders the styled response. Same JSON-vs-HTML triage.
func RespondInternalError(h ErrorPageHost, w http.ResponseWriter, r *http.Request) {
	errorDispatch(h, errorKindInternalError)(w, r)
}

// RespondNotFound is the in-shell counterpart to http.Error(..., 404)
// for handlers that 404 explicitly (e.g. "project not found"). The
// global chi NotFound dispatcher catches unrouted paths; this helper
// handles handler-internal 404s consistently.
func RespondNotFound(h ErrorPageHost, w http.ResponseWriter, r *http.Request) {
	errorDispatch(h, errorKindNotFound)(w, r)
}

// Error emits a negotiated error (branded HTML shell or JSON envelope) using
// the request-scoped responder the router stashed. It is the one-line
// replacement for bare http.Error in weave handlers. Falls back to plain
// http.Error only if no responder is on the context (route mounted outside
// the router's stash middleware).
func Error(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	if resp, ok := errresp.FromContext(r.Context()); ok {
		resp(w, r, status, code, message)
		return
	}
	http.Error(w, message, status) //nolint:forbidigo // fallback when unrouted
}

// BuildResponder builds the concrete errresp.Responder backed by h: JSON
// requests (per weavetemplates.WantsJSON) get the canonical apierror
// envelope; everything else gets the branded shell at the given status via
// RenderErrorPage. Falls back to plain http.Error if h has no renderer wired
// (e.g. a degraded/test host). Callers stash the result on the request
// context via errresp.WithResponder so any handler in the mounted tree can
// call weaverouter.Error (or read the responder directly via
// errresp.FromContext, which is how pkg/auth reaches it without importing
// this package) instead of a bare http.Error.
func BuildResponder(h ErrorPageHost) errresp.Responder {
	return func(w http.ResponseWriter, r *http.Request, status int, code, message string) {
		if weavetemplates.WantsJSON(r) {
			apierror.Write(w, &apierror.Error{Status: status, Code: apierror.Code(code), Message: message})
			return
		}
		if h.Templates == nil {
			http.Error(w, message, status) //nolint:forbidigo // no renderer wired
			return
		}
		heading := http.StatusText(status)
		if heading == "" {
			heading = "Error"
		}
		h.Templates.RenderErrorPage(w, r, BuildErrorPageDeps(h, r), weavetemplates.ErrorPageContent{
			StatusCode:  status,
			Code:        strconv.Itoa(status),
			Heading:     heading,
			Body:        message,
			RequestPath: r.URL.Path,
		})
	}
}

// mountSlice mounts a slice's sub-mux at pattern. Using chi.Mount with a
// fresh sub-mux isolates each slice's route tree from siblings (and from
// any legacy routes still registered on the same parent), preventing
// "trying to mount on existing path" panics during the migration.
//
// When h.Weave is non-nil, the auth.WithProjectResource middleware is installed
// on the sub-mux so every slice route under a {projectID} path picks up the
// per-request *domain.Project + populated auth.Resource (with Visibility) for
// free. Slices read it via auth.ProjectResourceFromContext.
func mountSlice(parent chi.Router, pattern string, h ProjectMiddlewareHost, attach func(chi.Router)) {
	sub := chi.NewMux()
	if h.Weave != nil {
		sub.Use(auth.WithProjectVersionContext)
		sub.Use(auth.WithProjectResource(h.Weave))
	}
	sub.Use(withChangeSetHint)
	attach(sub)
	parent.Mount(pattern, sub)
}

// withChangeSetHint seeds a change-set hint (project + actor) for mutating
// requests so any change_log a slice records is stamped with the real project
// (the materializer routes commits by project) and real authorship. Read-only
// requests are untouched. The hint is lazy: resolveChangeSet only consults it
// if a mutation actually opens a change set.
func withChangeSetHint(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
			if projectID := chi.URLParam(r, "projectID"); projectID != "" {
				hint := domain.ChangeSetHint{
					ProjectID:     projectID,
					CommitMessage: r.Method + " " + r.URL.Path,
				}
				if p := auth.PrincipalFromContext(r.Context()); p != nil {
					hint.ActorID = p.ActorID
					hint.ActorName = p.DisplayName
					hint.ActorEmail = p.Email
				}
				r = r.WithContext(domain.ContextWithChangeSetHint(r.Context(), hint))
			}
		}
		next.ServeHTTP(w, r)
	})
}

// redirectToExports forwards legacy /projects/{id}/downloads URLs to
// the new /projects/{id}/exports surface. The migration team's
// bookmarks keep working for at least one release cycle.
func redirectToExports(w http.ResponseWriter, r *http.Request) {
	target := strings.Replace(r.URL.Path, "/downloads", "/exports", 1)
	if r.URL.RawQuery != "" {
		target += "?" + r.URL.RawQuery
	}
	http.Redirect(w, r, target, http.StatusMovedPermanently)
}
