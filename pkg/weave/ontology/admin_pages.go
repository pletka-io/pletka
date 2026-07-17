package ontology

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/frontendrefs"
	"github.com/pletka-io/pletka/pkg/i18n"
	"github.com/pletka-io/pletka/pkg/session"
	weavetemplates "github.com/pletka-io/pletka/pkg/weave/templates"
)

// AdminPages renders thin management shells that consume the same ontology
// page schemas as the public browse pages, with admin actions enabled by the
// schema endpoints.
type AdminPages struct {
	logger    *slog.Logger
	renderer  *weavetemplates.Renderer
	svc       *Service
	i18n      i18n.Manager
	session   *session.Manager
	languages []i18n.Language
}

func NewAdminPages(
	logger *slog.Logger,
	renderer *weavetemplates.Renderer,
	svc *Service,
	i18nManager i18n.Manager,
	sessionManager *session.Manager,
) *AdminPages {
	if logger == nil {
		logger = slog.Default()
	}
	return &AdminPages{
		logger:    logger,
		renderer:  renderer,
		svc:       svc,
		i18n:      i18nManager,
		session:   sessionManager,
		languages: i18nManager.Languages(),
	}
}

func (p *AdminPages) Mount(r chi.Router) {
	r.Get("/page", p.landingPage)
	r.Get("/families/{familyID}/page", p.familyPage)
	r.Get("/versions/{versionID}/page", p.versionPage)
	r.Get("/{ontologyID}/versions/import/page", p.importVersionPage)
	r.Get("/{ontologyID}/page", p.ontologyPage)
}

func (p *AdminPages) landingPage(w http.ResponseWriter, r *http.Request) {
	p.renderOntologySchemaShell(w, r, "Ontology management", "Manage ontology families, registered ontologies, and imported versions.", "/admin/ontologies/page-schema", []weavetemplates.Breadcrumb{
		{Label: "Admin", Href: "/admin"},
		{Label: "Ontologies"},
	})
}

// familyPage redirects the legacy ULID-keyed admin family page to the
// canonical slug page (see ontologyPage).
func (p *AdminPages) familyPage(w http.ResponseWriter, r *http.Request) {
	lang := p.currentLang(r)
	family, err := p.svc.GetFamily(r.Context(), chi.URLParam(r, "familyID"))
	if err != nil {
		p.renderer.RespondNotFound(w, r, p.renderer.ErrorContext(r, lang))
		return
	}
	http.Redirect(w, r, "/ontologies/families/"+family.Slug, http.StatusFound)
}

// ontologyPage redirects the legacy ULID-keyed admin detail page to the
// canonical slug/prefix page. The public page now surfaces admin actions
// (Edit / Import) to super-admins, so there is one URL space (the ULID
// pages confused curators).
func (p *AdminPages) ontologyPage(w http.ResponseWriter, r *http.Request) {
	lang := p.currentLang(r)
	ontology, err := p.svc.GetOntology(r.Context(), chi.URLParam(r, "ontologyID"))
	if err != nil {
		p.renderer.RespondNotFound(w, r, p.renderer.ErrorContext(r, lang))
		return
	}
	http.Redirect(w, r, "/ontologies/"+ontology.Prefix, http.StatusFound)
}

// versionPage redirects the legacy ULID-keyed admin version page to the
// canonical slug/prefix page (see ontologyPage).
func (p *AdminPages) versionPage(w http.ResponseWriter, r *http.Request) {
	lang := p.currentLang(r)
	version, err := p.svc.GetVersion(r.Context(), chi.URLParam(r, "versionID"))
	if err != nil {
		p.renderer.RespondNotFound(w, r, p.renderer.ErrorContext(r, lang))
		return
	}
	ontology, err := p.svc.GetOntology(r.Context(), version.OntologyID)
	if err != nil {
		p.renderer.RespondNotFound(w, r, p.renderer.ErrorContext(r, lang))
		return
	}
	http.Redirect(w, r, "/ontologies/"+ontology.Prefix+"/"+version.VersionString, http.StatusFound)
}

func (p *AdminPages) importVersionPage(w http.ResponseWriter, r *http.Request) {
	lang := p.currentLang(r)
	ontology, err := p.svc.GetOntology(r.Context(), chi.URLParam(r, "ontologyID"))
	if err != nil {
		p.renderer.RespondNotFound(w, r, p.renderer.ErrorContext(r, lang))
		return
	}
	p.renderOntologySchemaShell(w, r, ontology.Prefix+" import", "Version import management", "/admin/ontologies/"+ontology.ID+"/versions/import/page-schema", []weavetemplates.Breadcrumb{
		{Label: "Admin", Href: "/admin"},
		{Label: "Ontologies", Href: "/admin/ontologies/page"},
		{Label: ontology.Prefix, Href: "/admin/ontologies/" + ontology.ID + "/page"},
		{Label: "Import version"},
	})
}

func (p *AdminPages) renderOntologySchemaShell(w http.ResponseWriter, r *http.Request, title, subheading, schemaURL string, breadcrumbs []weavetemplates.Breadcrumb) {
	lang := p.currentLang(r)
	page := weavetemplates.IslandPage{
		Title:       title,
		Lang:        lang,
		Subheading:  subheading,
		Breadcrumbs: breadcrumbs,
		Island: weavetemplates.IslandMount{
			Name: frontendrefs.Island("ontology-page"),
			Props: map[string]string{
				"schema-url": schemaURL,
			},
			Dependencies: []string{frontendrefs.Island("ontology-page")},
			Placeholder:  ontologyPagePlaceholder(),
		},
	}
	page.Path = r.URL.Path
	page.Languages = p.languages
	page.Principal = auth.PrincipalFromContext(r.Context())
	page.IsAnonymous = page.Principal == nil
	page.Labels = p.renderer.ShellLabels(lang)
	if err := p.renderer.RenderIslandPage(w, page); err != nil {
		p.logger.Error("render admin ontology page", "path", r.URL.Path, "err", err)
		p.renderer.RespondInternalError(w, r, p.renderer.ErrorContext(r, lang))
	}
}

func (p *AdminPages) currentLang(r *http.Request) string {
	if p.session != nil {
		if lang := p.session.Language(r.Context()); lang != "" {
			return lang
		}
	}
	if lang := r.URL.Query().Get("lang"); lang != "" {
		return lang
	}
	return "en"
}
