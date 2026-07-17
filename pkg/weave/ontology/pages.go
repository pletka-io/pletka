package ontology

import (
	"html/template"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/formschema"
	"github.com/pletka-io/pletka/pkg/frontendrefs"
	"github.com/pletka-io/pletka/pkg/i18n"
	"github.com/pletka-io/pletka/pkg/session"
	weavetemplates "github.com/pletka-io/pletka/pkg/weave/templates"
)

// Pages renders the public browse experience for ontologies under
// /ontologies. Public browse pages are thin gohtml shells that mount
// Svelte islands fed by ontology-specific schemas.
//
// Reads are open to any authenticated user; the schemas served at
// /ontologies/list-schema endpoints have edit caps stripped so the
// island renders read-only.
type Pages struct {
	logger    *slog.Logger
	renderer  *weavetemplates.Renderer
	svc       *Service
	i18n      i18n.Manager
	session   *session.Manager
	languages []i18n.Language
}

// NewPages wires a Pages handler.
func NewPages(
	logger *slog.Logger,
	renderer *weavetemplates.Renderer,
	svc *Service,
	i18nManager i18n.Manager,
	sessionManager *session.Manager,
) *Pages {
	if logger == nil {
		logger = slog.Default()
	}
	return &Pages{
		logger:    logger,
		renderer:  renderer,
		svc:       svc,
		i18n:      i18nManager,
		session:   sessionManager,
		languages: i18nManager.Languages(),
	}
}

// Mount registers the public browse routes.
//
// Routes:
//
//	GET /ontologies                                     family landing page
//	GET /ontologies/all                                 flat ontology browse
//	GET /ontologies/families/{slug}                     family detail
//	GET /ontologies/{prefix}                            ontology detail with versions list (island)
//	GET /ontologies/{prefix}/{version}                  version detail with classes list (island)
//	GET /ontologies/{prefix}/{version}/properties       version's property list (island)
//	GET /ontologies/{prefix}/{version}/classes/{classID}    class detail schema island
//	GET /ontologies/{prefix}/{version}/properties/{propertyID}
//	                                                    property detail schema island
//
// Detail URLs use the entity's ULID (not qname) because the entity-list
// island's URLTemplate substitutes {id} from the row data, and the
// row's id is the ULID surrogate. Detail handlers resolve by ULID.
func (p *Pages) Mount(parent chi.Router) {
	parent.Get("/ontologies", p.familiesPage)
	parent.Get("/ontologies/page-schema", p.familiesPageSchema)
	parent.Get("/ontologies/all", p.allOntologiesSchemaPage)
	parent.Get("/ontologies/all/page-schema", p.allOntologiesPageSchema)
	parent.Get("/ontologies/families/{slug}/page-schema", p.familyDetailPageSchema)
	parent.Get("/ontologies/families/{slug}", p.familyDetailSchemaPage)
	parent.Get("/ontologies/{prefix}/page-schema", p.ontologyDetailPageSchema)
	parent.Get("/ontologies/{prefix}", p.ontologyDetailPage)
	parent.Get("/ontologies/{prefix}/{version}/page-schema", p.versionDetailPageSchema)
	parent.Get("/ontologies/{prefix}/{version}", p.versionClassesPage)
	parent.Get("/ontologies/{prefix}/{version}/properties", p.versionPropertiesPage)
	parent.Get("/ontologies/{prefix}/{version}/classes/{classID}/page-schema", p.classDetailPageSchema)
	parent.Get("/ontologies/{prefix}/{version}/classes/{classID}", p.classDetailPage)
	parent.Get("/ontologies/{prefix}/{version}/properties/{propertyID}/page-schema", p.propertyDetailPageSchema)
	parent.Get("/ontologies/{prefix}/{version}/properties/{propertyID}", p.propertyDetailPage)

	// Public list-schema endpoints (called by the entity-list island
	// the browse pages mount). These reuse the slice's existing
	// Build*ListSchema builders but strip the edit/create/delete caps
	// so the island renders read-only.
	parent.Get("/ontologies/list-schema", p.familiesListSchema)
	parent.Get("/ontologies/{prefix}/list-schema", p.versionsListSchema)
	parent.Get("/ontologies/{prefix}/{version}/classes/data", p.publicClassesData)
	parent.Get("/ontologies/{prefix}/{version}/classes/list-schema", p.classesListSchema)
	parent.Get("/ontologies/{prefix}/{version}/properties/data", p.publicPropertiesData)
	parent.Get("/ontologies/{prefix}/{version}/properties/list-schema", p.propertiesListSchema)
}

// ----------------------------------------------------------------------------
// List pages — mount entity-list with the public schema URL.
// ----------------------------------------------------------------------------

func (p *Pages) familiesPage(w http.ResponseWriter, r *http.Request) {
	p.renderOntologySchemaShell(w, r, p.i18n.T("nav.ontologies", p.currentLang(r)), "Browse ontology families first, then drill into base ontologies, extensions, and versions.", "/ontologies/page-schema", []weavetemplates.Breadcrumb{
		{Label: p.i18n.T("nav.ontologies", p.currentLang(r))},
	})
}

func (p *Pages) allOntologiesSchemaPage(w http.ResponseWriter, r *http.Request) {
	lang := p.currentLang(r)
	p.renderOntologySchemaShell(w, r, "All Ontologies", "Flat browse view across ontology families.", "/ontologies/all/page-schema", []weavetemplates.Breadcrumb{
		{Label: p.i18n.T("nav.ontologies", lang), Href: "/ontologies"},
		{Label: "All Ontologies"},
	})
}

func (p *Pages) familyDetailSchemaPage(w http.ResponseWriter, r *http.Request) {
	lang := p.currentLang(r)
	slug := chi.URLParam(r, "slug")
	family, err := p.svc.GetFamilyBySlug(r.Context(), slug)
	if err != nil {
		p.renderer.RespondNotFound(w, r, p.renderer.ErrorContext(r, lang))
		return
	}
	p.renderOntologySchemaShell(w, r, family.Name, "Family overview for related base ontologies and extensions.", "/ontologies/families/"+family.Slug+"/page-schema", []weavetemplates.Breadcrumb{
		{Label: p.i18n.T("nav.ontologies", lang), Href: "/ontologies"},
		{Label: family.Name},
	})
}

func (p *Pages) ontologyDetailPage(w http.ResponseWriter, r *http.Request) {
	prefix := chi.URLParam(r, "prefix")
	o, err := p.svc.GetOntologyByPrefix(r.Context(), prefix)
	if err != nil {
		p.renderer.RespondNotFound(w, r, p.renderer.ErrorContext(r, p.currentLang(r)))
		return
	}
	lang := p.currentLang(r)
	p.renderOntologySchemaShell(w, r, o.Name, o.Namespace, "/ontologies/"+o.Prefix+"/page-schema", []weavetemplates.Breadcrumb{
		{Label: p.i18n.T("nav.ontologies", lang), Href: "/ontologies"},
		{Label: o.Prefix},
	})
}

func (p *Pages) versionClassesPage(w http.ResponseWriter, r *http.Request) {
	prefix, version := chi.URLParam(r, "prefix"), chi.URLParam(r, "version")
	o, err := p.svc.GetOntologyByPrefix(r.Context(), prefix)
	if err != nil {
		p.renderer.RespondNotFound(w, r, p.renderer.ErrorContext(r, p.currentLang(r)))
		return
	}
	v, err := p.resolveVersion(r, prefix, version)
	if err != nil {
		p.renderer.RespondNotFound(w, r, p.renderer.ErrorContext(r, p.currentLang(r)))
		return
	}
	versionRow, err := p.svc.GetVersion(r.Context(), v.ID)
	if err != nil {
		p.renderer.RespondNotFound(w, r, p.renderer.ErrorContext(r, p.currentLang(r)))
		return
	}
	lang := p.currentLang(r)
	schemaURL := "/ontologies/" + o.Prefix + "/" + versionRow.VersionString + "/page-schema"
	if tab := r.URL.Query().Get("tab"); tab == "classes" || tab == "properties" {
		schemaURL += "?tab=" + tab
	}
	p.renderOntologySchemaShell(w, r, o.Prefix+" "+versionRow.VersionString, "Version overview", schemaURL, []weavetemplates.Breadcrumb{
		{Label: p.i18n.T("nav.ontologies", lang), Href: "/ontologies"},
		{Label: o.Prefix, Href: "/ontologies/" + o.Prefix},
		{Label: versionRow.VersionString},
	})
}

func (p *Pages) versionPropertiesPage(w http.ResponseWriter, r *http.Request) {
	prefix, version := chi.URLParam(r, "prefix"), chi.URLParam(r, "version")
	o, err := p.svc.GetOntologyByPrefix(r.Context(), prefix)
	if err != nil {
		p.renderer.RespondNotFound(w, r, p.renderer.ErrorContext(r, p.currentLang(r)))
		return
	}
	v, err := p.resolveVersion(r, prefix, version)
	if err != nil {
		p.renderer.RespondNotFound(w, r, p.renderer.ErrorContext(r, p.currentLang(r)))
		return
	}
	versionRow, err := p.svc.GetVersion(r.Context(), v.ID)
	if err != nil {
		p.renderer.RespondNotFound(w, r, p.renderer.ErrorContext(r, p.currentLang(r)))
		return
	}
	lang := p.currentLang(r)
	p.renderOntologySchemaShell(w, r, o.Prefix+" "+versionRow.VersionString, "Properties", "/ontologies/"+o.Prefix+"/"+versionRow.VersionString+"/page-schema?tab=properties", []weavetemplates.Breadcrumb{
		{Label: p.i18n.T("nav.ontologies", lang), Href: "/ontologies"},
		{Label: o.Prefix, Href: "/ontologies/" + o.Prefix},
		{Label: versionRow.VersionString},
	})
}

func (p *Pages) renderOntologySchemaShell(w http.ResponseWriter, r *http.Request, title, subheading, schemaURL string, breadcrumbs []weavetemplates.Breadcrumb) {
	lang := p.currentLang(r)
	p.renderList(w, r, weavetemplates.IslandPage{
		Title:       title,
		Lang:        lang,
		Heading:     "",
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
	})
}

func (p *Pages) familiesPageSchema(w http.ResponseWriter, r *http.Request) {
	lang := p.currentLang(r)
	model, err := p.svc.FamilyLandingPageModel(r.Context(), lang)
	if err != nil {
		p.renderer.RespondInternalError(w, r, p.renderer.ErrorContext(r, lang))
		return
	}
	writeJSON(w, http.StatusOK, BuildFamilyLandingPageSchema(model, lang, p.schemaLanguages()))
}

func (p *Pages) allOntologiesPageSchema(w http.ResponseWriter, r *http.Request) {
	lang := p.currentLang(r)
	model, err := p.svc.AllOntologiesPageModel(r.Context(), lang)
	if err != nil {
		p.renderer.RespondInternalError(w, r, p.renderer.ErrorContext(r, lang))
		return
	}
	writeJSON(w, http.StatusOK, BuildAllOntologiesPageSchema(model, lang, p.schemaLanguages()))
}

func (p *Pages) familyDetailPageSchema(w http.ResponseWriter, r *http.Request) {
	lang := p.currentLang(r)
	model, err := p.svc.FamilyDetailPageModel(r.Context(), chi.URLParam(r, "slug"), r.URL.Query().Get("tab"), lang)
	if err != nil {
		p.renderer.RespondNotFound(w, r, p.renderer.ErrorContext(r, lang))
		return
	}
	writeJSON(w, http.StatusOK, BuildFamilyDetailPageSchema(model, lang, p.schemaLanguages()))
}

// viewerIsAdmin reports whether the request comes from a super-admin, so the
// shared ontology page schemas can surface admin actions (Edit / Import /
// Set-active) on the public prefix-keyed pages — one URL space instead of a
// duplicate /admin/ontologies/{ULID} tree follow-up.
func (p *Pages) viewerIsAdmin(r *http.Request) bool {
	snap := auth.FromContext(r.Context())
	return snap != nil && snap.IsSuperAdmin
}

func (p *Pages) ontologyDetailPageSchema(w http.ResponseWriter, r *http.Request) {
	lang := p.currentLang(r)
	model, err := p.svc.OntologyDetailPageModel(r.Context(), chi.URLParam(r, "prefix"), lang)
	if err != nil {
		p.renderer.RespondNotFound(w, r, p.renderer.ErrorContext(r, lang))
		return
	}
	writeJSON(w, http.StatusOK, BuildOntologyDetailPageSchema(model, lang, p.schemaLanguages(), p.viewerIsAdmin(r)))
}

func (p *Pages) versionDetailPageSchema(w http.ResponseWriter, r *http.Request) {
	lang := p.currentLang(r)
	model, err := p.svc.VersionDetailPageModel(r.Context(), chi.URLParam(r, "prefix"), chi.URLParam(r, "version"), r.URL.Query().Get("tab"), lang)
	if err != nil {
		p.renderer.RespondNotFound(w, r, p.renderer.ErrorContext(r, lang))
		return
	}
	writeJSON(w, http.StatusOK, BuildVersionDetailPageSchema(model, lang, p.schemaLanguages(), p.viewerIsAdmin(r)))
}

func (p *Pages) classDetailPageSchema(w http.ResponseWriter, r *http.Request) {
	lang := p.currentLang(r)
	model, err := p.svc.ClassDetailPageModel(r.Context(), chi.URLParam(r, "prefix"), chi.URLParam(r, "version"), chi.URLParam(r, "classID"), lang)
	if err != nil {
		p.renderer.RespondNotFound(w, r, p.renderer.ErrorContext(r, lang))
		return
	}
	writeJSON(w, http.StatusOK, BuildClassDetailPageSchema(model, lang, p.schemaLanguages()))
}

func (p *Pages) propertyDetailPageSchema(w http.ResponseWriter, r *http.Request) {
	lang := p.currentLang(r)
	model, err := p.svc.PropertyDetailPageModel(r.Context(), chi.URLParam(r, "prefix"), chi.URLParam(r, "version"), chi.URLParam(r, "propertyID"), lang)
	if err != nil {
		p.renderer.RespondNotFound(w, r, p.renderer.ErrorContext(r, lang))
		return
	}
	writeJSON(w, http.StatusOK, BuildPropertyDetailPageSchema(model, lang, p.schemaLanguages()))
}

func (p *Pages) schemaLanguages() []formschema.LanguageInfo {
	out := make([]formschema.LanguageInfo, 0, len(p.languages))
	for _, lang := range p.languages {
		flag, _ := lang.Metadata["flag"].(string)
		out = append(out, formschema.LanguageInfo{
			Code: lang.Code,
			Name: lang.Name,
			Flag: flag,
		})
	}
	return out
}

func ontologyPagePlaceholder() template.HTML {
	return template.HTML(`
<div class="space-y-4 animate-pulse">
  <div class="h-24 rounded-lg bg-white ring-1 ring-gray-200"></div>
  <div class="grid gap-4 md:grid-cols-3">
    <div class="h-20 rounded-lg bg-white ring-1 ring-gray-200"></div>
    <div class="h-20 rounded-lg bg-white ring-1 ring-gray-200"></div>
    <div class="h-20 rounded-lg bg-white ring-1 ring-gray-200"></div>
  </div>
  <div class="h-40 rounded-lg bg-white ring-1 ring-gray-200"></div>
</div>`)
}

func (p *Pages) renderList(w http.ResponseWriter, r *http.Request, page weavetemplates.IslandPage) {
	page.Path = r.URL.Path
	page.Languages = p.languages
	page.Principal = auth.PrincipalFromContext(r.Context())
	page.IsAnonymous = page.Principal == nil
	page.Labels = p.renderer.ShellLabels(page.Lang)
	if err := p.renderer.RenderIslandPage(w, page); err != nil {
		p.logger.Error("render ontology page", "path", r.URL.Path, "err", err)
		p.renderer.RespondInternalError(w, r, p.renderer.ErrorContext(r, page.Lang))
	}
}

// ----------------------------------------------------------------------------
// Public list-schema endpoints — strip edit caps for read-only render.
// ----------------------------------------------------------------------------

func (p *Pages) familiesListSchema(w http.ResponseWriter, r *http.Request) {
	schema := BuildFamilyListSchema(p.currentLang(r), nil)
	schema.DataURL = "/admin/ontologies/families"
	stripWriteCaps(schema)
	writeJSON(w, http.StatusOK, schema)
}

func (p *Pages) versionsListSchema(w http.ResponseWriter, r *http.Request) {
	prefix := chi.URLParam(r, "prefix")
	o, err := p.svc.GetOntologyByPrefix(r.Context(), prefix)
	if err != nil || o == nil {
		p.renderer.RespondNotFound(w, r, p.renderer.ErrorContext(r, p.currentLang(r)))
		return
	}
	schema := BuildVersionListSchema(o.ID, p.currentLang(r), nil)
	stripWriteCaps(schema)
	writeJSON(w, http.StatusOK, schema)
}

func (p *Pages) classesListSchema(w http.ResponseWriter, r *http.Request) {
	prefix, version := chi.URLParam(r, "prefix"), chi.URLParam(r, "version")
	v, err := p.resolveVersion(r, prefix, version)
	if err != nil {
		p.renderer.RespondNotFound(w, r, p.renderer.ErrorContext(r, p.currentLang(r)))
		return
	}
	schema := buildVersionItemListSchema(v.ID, "ontology-class", "/ontologies/"+prefix+"/"+version+"/classes/data",
		"/ontologies/"+prefix+"/"+version+"/classes/{id}",
		"Classes")
	writeJSON(w, http.StatusOK, schema)
}

func (p *Pages) propertiesListSchema(w http.ResponseWriter, r *http.Request) {
	prefix, version := chi.URLParam(r, "prefix"), chi.URLParam(r, "version")
	v, err := p.resolveVersion(r, prefix, version)
	if err != nil {
		p.renderer.RespondNotFound(w, r, p.renderer.ErrorContext(r, p.currentLang(r)))
		return
	}
	schema := buildVersionItemListSchema(v.ID, "ontology-property", "/ontologies/"+prefix+"/"+version+"/properties/data",
		"/ontologies/"+prefix+"/"+version+"/properties/{id}",
		"Properties")
	writeJSON(w, http.StatusOK, schema)
}

func (p *Pages) publicClassesData(w http.ResponseWriter, r *http.Request) {
	prefix, version := chi.URLParam(r, "prefix"), chi.URLParam(r, "version")
	v, err := p.resolveVersion(r, prefix, version)
	if err != nil {
		p.renderer.RespondNotFound(w, r, p.renderer.ErrorContext(r, p.currentLang(r)))
		return
	}
	items, err := p.svc.ListClasses(r.Context(), v.ID)
	if err != nil {
		p.renderer.RespondInternalError(w, r, p.renderer.ErrorContext(r, p.currentLang(r)))
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (p *Pages) publicPropertiesData(w http.ResponseWriter, r *http.Request) {
	prefix, version := chi.URLParam(r, "prefix"), chi.URLParam(r, "version")
	v, err := p.resolveVersion(r, prefix, version)
	if err != nil {
		p.renderer.RespondNotFound(w, r, p.renderer.ErrorContext(r, p.currentLang(r)))
		return
	}
	items, err := p.svc.ListProperties(r.Context(), v.ID)
	if err != nil {
		p.renderer.RespondInternalError(w, r, p.renderer.ErrorContext(r, p.currentLang(r)))
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (p *Pages) classDetailPage(w http.ResponseWriter, r *http.Request) {
	prefix, version, classID := chi.URLParam(r, "prefix"), chi.URLParam(r, "version"), chi.URLParam(r, "classID")
	lang := p.currentLang(r)
	model, err := p.svc.ClassDetailPageModel(r.Context(), prefix, version, classID, lang)
	if err != nil {
		p.renderer.RespondNotFound(w, r, p.renderer.ErrorContext(r, lang))
		return
	}
	p.renderOntologySchemaShell(w, r, model.Class.Qname, model.Label, "/ontologies/"+prefix+"/"+version+"/classes/"+classID+"/page-schema", []weavetemplates.Breadcrumb{
		{Label: p.i18n.T("nav.ontologies", lang), Href: "/ontologies"},
		{Label: model.Prefix, Href: "/ontologies/" + model.Prefix},
		{Label: model.VersionString, Href: "/ontologies/" + model.Prefix + "/" + model.VersionString},
		{Label: "Classes", Href: "/ontologies/" + model.Prefix + "/" + model.VersionString + "?tab=classes"},
		{Label: model.Class.Qname},
	})
}

func (p *Pages) propertyDetailPage(w http.ResponseWriter, r *http.Request) {
	prefix, version, propertyID := chi.URLParam(r, "prefix"), chi.URLParam(r, "version"), chi.URLParam(r, "propertyID")
	lang := p.currentLang(r)
	model, err := p.svc.PropertyDetailPageModel(r.Context(), prefix, version, propertyID, lang)
	if err != nil {
		p.renderer.RespondNotFound(w, r, p.renderer.ErrorContext(r, lang))
		return
	}
	p.renderOntologySchemaShell(w, r, model.Property.Qname, model.Label, "/ontologies/"+prefix+"/"+version+"/properties/"+propertyID+"/page-schema", []weavetemplates.Breadcrumb{
		{Label: p.i18n.T("nav.ontologies", lang), Href: "/ontologies"},
		{Label: model.Prefix, Href: "/ontologies/" + model.Prefix},
		{Label: model.VersionString, Href: "/ontologies/" + model.Prefix + "/" + model.VersionString},
		{Label: "Properties", Href: "/ontologies/" + model.Prefix + "/" + model.VersionString + "/properties"},
		{Label: model.Property.Qname},
	})
}

// ----------------------------------------------------------------------------
// Helpers
// ----------------------------------------------------------------------------

func (p *Pages) currentLang(r *http.Request) string {
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

func placeholderHTML() template.HTML {
	return template.HTML(`<div class="bg-white shadow-sm rounded-lg p-6 animate-pulse">
<div class="h-8 bg-gray-200 rounded w-1/3 mb-4"></div>
<div class="h-4 bg-gray-200 rounded w-2/3 mb-6"></div>
<div class="space-y-3"><div class="h-10 bg-gray-100 rounded"></div><div class="h-10 bg-gray-100 rounded"></div><div class="h-10 bg-gray-100 rounded"></div></div></div>`)
}

func (p *Pages) resolveVersion(r *http.Request, prefix, version string) (resolvedVersion, error) {
	o, err := p.svc.GetOntologyByPrefix(r.Context(), prefix)
	if err != nil {
		return resolvedVersion{}, &pageError{msg: "ontology not found"}
	}
	v, err := p.svc.store.GetVersionByOntologyAndString(r.Context(), o.ID, version)
	if err != nil || v == nil {
		return resolvedVersion{}, &pageError{msg: "version not found"}
	}
	return resolvedVersion{ID: v.ID, VersionString: v.VersionString}, nil
}

type resolvedVersion struct {
	ID            string
	VersionString string
}

type pageError struct{ msg string }

func (e *pageError) Error() string { return e.msg }

// stripWriteCaps removes Create / Edit / Delete capabilities from a
// list schema so the entity-list island renders read-only.
func stripWriteCaps(schema *formschema.ListSchema) {
	if schema == nil {
		return
	}
	schema.Caps.Create = nil
	schema.Caps.Edit = nil
	schema.Caps.Delete = nil
}

// buildVersionItemListSchema is a lightweight list schema for class /
// property browse pages. Detail navigation is offered via a Stats cap
// pointing at the per-detail URL template; ListManager wires that
// into the per-row action menu. Direct row-click navigation is a
// follow-up — would need a generic "navigate" action type in
// ListManager.
func buildVersionItemListSchema(versionID, entityType, dataURL, detailURLTpl, headerLabel string) *formschema.ListSchema {
	return &formschema.ListSchema{
		EntityType: entityType,
		EmptyState: &formschema.EmptyState{
			Icon:    "academic-cap",
			Title:   i18n.LF("ontology_admin.version_browse.empty_title", "No {label}", map[string]string{"label": headerLabel}),
			Message: i18n.L("ontology_admin.version_browse.empty_message", "Empty for this version."),
		},
		DataURL: dataURL,
		Caps: formschema.Capabilities{
			Stats: &formschema.StatsCap{
				URLTemplate: detailURLTpl,
			},
		},
		Columns: []formschema.Column{
			{Key: "qname", Label: i18n.L("ontology_admin.version_browse.qname", "Qname"), Type: "text", Primary: true},
			{Key: "local_name", Label: i18n.L("ontology_admin.version_browse.local_name", "Local name"), Type: "text"},
			{Key: "label", Label: i18n.L("ontology_admin.version.label", "Label"), Type: "translations"},
		},
	}
}
