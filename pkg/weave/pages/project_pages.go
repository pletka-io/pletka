package pages

import (
	"context"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/frontendrefs"
	"github.com/pletka-io/pletka/pkg/i18n"
	"github.com/pletka-io/pletka/pkg/session"
	weaveroutes "github.com/pletka-io/pletka/pkg/weave/routes"
	weavetemplates "github.com/pletka-io/pletka/pkg/weave/templates"
)

// Host is the narrow dependency surface needed to mount project shell pages.
type Host struct {
	Logger   *slog.Logger
	Renderer *weavetemplates.Renderer
	Weave    domain.WeaveStore
	I18n     i18n.Manager
	Session  *session.Manager
	// LatestRelease resolves a public project's highest-semver release so
	// ProjectDetailPage can carry the effective version into the initial
	// schema-url. This route is mounted directly with auth.WrapProjectRead
	// (not through a slice's projectRead middleware chain), so
	// auth.ResolveContentVersion never runs for it — the page resolves the
	// version itself, mirroring visualization.Handler.effectiveVersion.
	// Optional; nil disables the lookup (schema-url stays version-less
	// unless the request already had one).
	LatestRelease auth.LatestReleaseReader
}

func (h Host) Validate() error {
	if h.Renderer == nil {
		return fmt.Errorf("project pages host missing renderer")
	}
	if h.Weave == nil {
		return fmt.Errorf("project pages host missing weave store")
	}
	if h.I18n == nil {
		return fmt.Errorf("project pages host missing i18n manager")
	}
	return nil
}

func (h Host) logger() *slog.Logger {
	if h.Logger != nil {
		return h.Logger
	}
	return slog.Default()
}

// ProjectPages renders the parallel chi-first schema-driven island wrappers.
// Only formschema/page-schema based islands belong here.
type ProjectPages struct {
	logger        *slog.Logger
	renderer      *weavetemplates.Renderer
	weave         domain.WeaveStore
	i18n          i18n.Manager
	session       *session.Manager
	latestRelease auth.LatestReleaseReader
}

func NewProjectPages(
	logger *slog.Logger,
	renderer *weavetemplates.Renderer,
	weaveStore domain.WeaveStore,
	i18nManager i18n.Manager,
	sessionManager *session.Manager,
	latestRelease auth.LatestReleaseReader,
) *ProjectPages {
	return &ProjectPages{
		logger:        logger,
		renderer:      renderer,
		weave:         weaveStore,
		i18n:          i18nManager,
		session:       sessionManager,
		latestRelease: latestRelease,
	}
}

// Mount builds and registers the project shell pages.
func Mount(r chi.Router, host Host) error {
	if err := host.Validate(); err != nil {
		return err
	}
	NewProjectPages(
		host.logger(),
		host.Renderer,
		host.Weave,
		host.I18n,
		host.Session,
		host.LatestRelease,
	).Mount(r)
	return nil
}

// Mount registers the parallel wrapper routes. The original routes stay intact.
func (h *ProjectPages) Mount(r chi.Router) {
	r.Get("/projects", h.ProjectsListPage)
	r.Get("/projects/{projectID:[A-Z0-9]+}", auth.WrapProjectRead(h.weave.Projects(), h.ProjectDetailPage))
	// Settings page is admin-only by definition — mirrors the
	// canAdmin gate that hides the Settings nav-link in
	// ProjectPageSchema. WrapRead previously let anon viewers of a
	// public project URL-type into the settings shell and see
	// categories, ontology config, etc. Mutation routes already
	// gated on ProjectEdit, but the page + schema reads leaked the
	// structure.
	r.Get("/projects/{projectID:[A-Z0-9]+}/settings", auth.WrapProjectEdit(h.weave.Projects(), h.ProjectSettingsPage))
}

func (h *ProjectPages) ProjectsListPage(w http.ResponseWriter, r *http.Request) {
	lang := h.currentLang(r)
	page := weavetemplates.IslandPage{
		Title:     h.i18n.T("projects.title", lang),
		Lang:      lang,
		Path:      r.URL.Path,
		Languages: h.i18n.Languages(),
		Heading:   h.i18n.T("projects.title", lang),
		Principal: auth.PrincipalFromContext(r.Context()),
		Labels:    h.renderer.ShellLabels(lang),
		Island: weavetemplates.IslandMount{
			Name: frontendrefs.Island("entity-list"),
			Props: map[string]string{
				"schema-url": "/projects/entity-list-schema",
				"lang":       lang,
			},
			Dependencies: []string{frontendrefs.Island("entity-list")},
			Placeholder:  placeholderHTML(),
		},
	}

	if err := h.renderer.RenderIslandPage(w, page); err != nil {
		h.logger.Error("render weave projects list page", "err", err)
		h.renderer.RespondInternalError(w, r, h.renderer.ErrorContext(r, lang))
	}
}

func (h *ProjectPages) ProjectDetailPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := h.currentLang(r)

	project := auth.ProjectFromContext(ctx)
	if project == nil {
		h.renderer.RespondNotFound(w, r, h.renderer.ErrorContext(r, lang))
		return
	}

	projectName := project.UIName.Get(lang, project.ID)
	schemaURL := "/projects/" + project.ID + "/page-schema"
	if version := h.effectiveVersion(ctx, project, r); version != "" {
		schemaURL += "?version=" + url.QueryEscape(version)
	}
	page := weavetemplates.IslandPage{
		Title:     projectTabTitle(project),
		Lang:      lang,
		Path:      r.URL.Path,
		Languages: h.i18n.Languages(),
		Principal: auth.PrincipalFromContext(r.Context()),
		Labels:    h.renderer.ShellLabels(lang),
		Breadcrumbs: []weavetemplates.Breadcrumb{
			{Label: h.i18n.T("projects.title", lang), Href: weaveroutes.ProjectBase},
			{Label: projectName},
		},
		Island: weavetemplates.IslandMount{
			Name: frontendrefs.Island("project-detail"),
			Props: map[string]string{
				"project-id": project.ID,
				"schema-url": schemaURL,
				"lang":       lang,
			},
			Dependencies: []string{frontendrefs.Island("project-detail")},
			Placeholder:  placeholderHTML(),
		},
	}

	if err := h.renderer.RenderIslandPage(w, page); err != nil {
		h.logger.Error("render weave project detail page", "project_id", project.ID, "err", err)
		h.renderer.RespondInternalError(w, r, h.renderer.ErrorContext(r, lang))
	}
}

// effectiveVersion applies auth.ResolveEffectiveVersion for the project
// landing page. This route is mounted with only auth.WrapProjectRead (see
// Mount above), so auth.ResolveContentVersion — the middleware every
// mountSlice-based JSON read surface gets — never runs here; the page
// resolves the version itself instead, mirroring
// visualization.Handler.effectiveVersion. The reader lookup is guarded so
// it only ever runs for a public, non-editor read with no explicit
// ?version= — editors and private/internal projects cost no query, and a
// reader error fails safe to hot (never blocks the page render).
func (h *ProjectPages) effectiveVersion(ctx context.Context, project *domain.Project, r *http.Request) string {
	explicit := r.URL.Query().Get("version")
	snap := auth.FromContext(ctx)

	var latest string
	if explicit == "" && h.latestRelease != nil && project != nil && project.Visibility == "public" &&
		!snap.Can(auth.ProjectEdit, auth.ProjectResource(project), nil) {
		v, err := h.latestRelease.LatestReleaseVersion(ctx, project.ID)
		if err != nil {
			h.logger.ErrorContext(ctx, "resolve content version: latest release lookup failed; serving hot", "project", project.ID, "err", err)
		} else {
			latest = v
		}
	}
	return auth.ResolveEffectiveVersion(snap, project, explicit, latest)
}

func (h *ProjectPages) ProjectSettingsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := h.currentLang(r)

	project := auth.ProjectFromContext(ctx)
	if project == nil {
		h.renderer.RespondNotFound(w, r, h.renderer.ErrorContext(r, lang))
		return
	}

	projectName := project.UIName.Get(lang, project.ID)
	projectHref := weaveroutes.ProjectBase + "/" + project.ID
	if version := r.URL.Query().Get("version"); version != "" {
		projectHref += "?version=" + url.QueryEscape(version)
	}
	page := weavetemplates.IslandPage{
		Title:     projectTabTitle(project) + " · " + h.i18n.T("projects.settings.title", lang),
		Lang:      lang,
		Path:      r.URL.Path,
		Languages: h.i18n.Languages(),
		Principal: auth.PrincipalFromContext(r.Context()),
		Labels:    h.renderer.ShellLabels(lang),
		Breadcrumbs: []weavetemplates.Breadcrumb{
			{Label: h.i18n.T("projects.title", lang), Href: weaveroutes.ProjectBase},
			{Label: projectName, Href: projectHref},
			{Label: h.i18n.T("projects.settings.title", lang)},
		},
		Island: weavetemplates.IslandMount{
			Name: frontendrefs.Island("project-settings"),
			Props: map[string]string{
				"project-id":      project.ID,
				"lang":            lang,
				"schema-url-base": "/projects/" + project.ID + "/settings",
			},
			Dependencies: []string{frontendrefs.Island("project-settings"), frontendrefs.Island("list-manager"), frontendrefs.Island("entity-form")},
		},
	}

	if err := h.renderer.RenderIslandPage(w, page); err != nil {
		h.logger.Error("render weave project settings page", "project_id", project.ID, "err", err)
		h.renderer.RespondInternalError(w, r, h.renderer.ErrorContext(r, lang))
	}
}

func (h *ProjectPages) currentLang(r *http.Request) string {
	if h.session != nil {
		if lang := h.session.Language(r.Context()); lang != "" {
			return lang
		}
	}
	if lang := r.URL.Query().Get("lang"); lang != "" {
		return lang
	}
	return "en"
}

func placeholderHTML() template.HTML {
	return template.HTML(`
<div class="bg-white shadow-sm rounded-lg p-6 animate-pulse">
    <div class="h-8 bg-gray-200 rounded w-1/3 mb-4"></div>
    <div class="h-4 bg-gray-200 rounded w-2/3 mb-6"></div>
    <div class="space-y-3">
        <div class="h-10 bg-gray-100 rounded"></div>
        <div class="h-10 bg-gray-100 rounded"></div>
        <div class="h-10 bg-gray-100 rounded"></div>
    </div>
</div>`)
}

// projectTabTitle returns the browser-tab title prefix for any
// project-scoped page. Format: "{IDPrefix} {English UIName}", e.g.
// "LA Linked Art". Always pulls the English name regardless of the
// caller's UI language so the tab is stable across locale switches
// and matches how curators refer to projects in conversation.
// Falls back to the ID alone when UIName is empty.
func projectTabTitle(p *domain.Project) string {
	name := p.UIName.Get("en", "")
	if name == "" {
		return p.ID
	}
	return p.ID + " " + name
}
