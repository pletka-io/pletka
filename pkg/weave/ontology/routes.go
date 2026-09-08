package ontology

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/formschema"
	"github.com/pletka-io/pletka/pkg/i18n"
	"github.com/pletka-io/pletka/pkg/session"
	weavetemplates "github.com/pletka-io/pletka/pkg/weave/templates"
)

// Mount registers the slice's routes on r. The caller is responsible
// for the URL prefix — currently /admin/ontologies/ via
// pkg/weave/router. The legacy ontology_admin / ontology_family_admin /
// ontology_management handlers no longer register, so the slice owns
// the /admin/ontologies/ URL space.
//
// Routes registered (relative to mount point):
//
//	GET    /                                                ListOntologies (?q=, ?family_id=)
//	POST   /                                                CreateOntology         (super_admin)
//	GET    /families                                        ListFamilies
//	POST   /families                                        CreateFamily           (super_admin)
//	GET    /families/{familyID}                             GetFamily
//	PUT    /families/{familyID}                             UpdateFamily           (super_admin)
//	DELETE /families/{familyID}                             DeleteFamily           (super_admin)
//	GET    /versions/{versionID}                            GetVersion
//	PUT    /versions/{versionID}                            UpdateVersion          (super_admin)
//	DELETE /versions/{versionID}                            DeleteVersion          (super_admin)
//	GET    /versions/{versionID}/classes     (?q=, ?limit=) ListClasses
//	GET    /versions/{versionID}/properties  (?q=, ?limit=) ListProperties
//	GET    /{ontologyID}                                    GetOntology
//	PUT    /{ontologyID}                                    UpdateOntology         (super_admin)
//	DELETE /{ontologyID}                                    DeleteOntology         (super_admin)
//	GET    /{ontologyID}/extensions                         ListExtensions
//	GET    /{ontologyID}/versions                           ListVersions (with usage)
//	POST   /{ontologyID}/versions/import/probe              ProbeImportVersion     (super_admin)
//	POST   /{ontologyID}/versions/import                    ImportVersionUpload    (super_admin)
//	POST   /{ontologyID}/versions/{versionID}/set-active    SetActiveVersion       (super_admin)
//
// More specific paths are registered before the {ontologyID} catch-all
// so chi matches them first.
//
// Auth: writes are super_admin-gated at the handler level
// (h.requireSuperAdmin). Reads are permitted to any authenticated user
// since autocomplete needs them. The parent router supplies the auth +
// session middleware that populates the AuthSnapshot on the context.
//
// RDF version-import reuses the same parser/converter/persistence path
// as `ontology-import import-v2`.
func (h *Handler) Mount(r chi.Router) {
	r.Get("/", h.ListOntologies)
	r.Get("/data", h.ListOntologiesData)
	r.Get("/options", h.OptionsOntologies)
	r.Post("/", h.CreateOntology)
	r.Get("/list-schema", h.ListSchema)
	r.Get("/entity-list-schema", h.EntityListSchema)
	r.Get("/form-schema", h.FormSchema)
	r.Get("/page-schema", h.AdminLandingPageSchema)

	r.Route("/autocomplete-cache", func(r chi.Router) {
		r.Post("/drop", h.DropAutocompleteCache)
		r.Get("/stats", h.AutocompleteCacheStats)
	})

	r.Route("/families", func(r chi.Router) {
		r.Get("/", h.ListFamilies)
		r.Get("/data", h.ListFamiliesData)
		r.Get("/options", h.OptionsFamilies)
		r.Post("/", h.CreateFamily)
		r.Get("/list-schema", h.FamilyListSchema)
		r.Get("/entity-list-schema", h.FamilyEntityListSchema)
		r.Get("/form-schema", h.FamilyFormSchema)
		r.Route("/{familyID}", func(r chi.Router) {
			r.Get("/page-schema", h.AdminFamilyPageSchema)
			r.Get("/", h.GetFamily)
			r.Put("/", h.UpdateFamily)
			r.Delete("/", h.DeleteFamily)
		})
	})

	r.Route("/versions", func(r chi.Router) {
		r.Get("/form-schema", h.VersionFormSchema)
		r.Route("/{versionID}", func(r chi.Router) {
			r.Get("/page-schema", h.AdminVersionPageSchema)
			r.Get("/", h.GetVersion)
			r.Put("/", h.UpdateVersion)
			r.Delete("/", h.DeleteVersion)
			r.Get("/classes", h.ListClasses)
			r.Get("/properties", h.ListProperties)
		})
	})

	r.Route("/{ontologyID}", func(r chi.Router) {
		r.Get("/page-schema", h.AdminOntologyPageSchema)
		r.Get("/", h.GetOntology)
		r.Put("/", h.UpdateOntology)
		r.Delete("/", h.DeleteOntology)
		r.Get("/extensions", h.ListExtensions)
		r.Get("/versions", h.ListVersions)
		r.Get("/versions/list-schema", h.VersionListSchema)
		r.Get("/versions/import/page-schema", h.AdminImportVersionPageSchema)
		r.Post("/versions/import/probe", h.ProbeImportVersion)
		r.Post("/versions/import", h.ImportVersionUpload)
		r.Post("/versions/{versionID}/set-active", h.SetActiveVersion)
	})
}

type AdminHost struct {
	Service      *Service
	Logger       *slog.Logger
	Templates    *weavetemplates.Renderer
	I18n         i18n.Manager
	Session      *session.Manager
	Languages    []formschema.LanguageInfo
	LangResolver func(*http.Request) string
}

func (h AdminHost) Validate() error {
	if h.Service == nil {
		return fmt.Errorf("ontology admin host missing required dependencies: Service")
	}
	return nil
}

// MountAdmin registers the admin ontology management surface. The supplied
// service must be the same shared Service used by the autocomplete API and
// public ontology pages.
func MountAdmin(r chi.Router, host AdminHost) {
	if err := host.Validate(); err != nil {
		panic(err)
	}
	h := NewHandler(host.Service, host.Logger, host.Languages, host.LangResolver)
	if host.Templates != nil && host.I18n != nil {
		NewAdminPages(host.Logger, host.Templates, host.Service, host.I18n, host.Session).Mount(r)
	}
	h.Mount(r)
}

// APIHost is the explicit dependency surface for the active ontology
// autocomplete and label endpoints.
type APIHost struct {
	Service      *Service
	Logger       *slog.Logger
	Languages    []formschema.LanguageInfo
	LangResolver func(*http.Request) string
	// Projects gates the project-scoped ontology-labels route with
	// RequireProjectRead. Required — the labels endpoint leaks which
	// ontology classes/properties a private project uses. The global
	// autocomplete/edge-provenance endpoints are deliberately unscoped and
	// do not consult it.
	Projects auth.ProjectReader
}

func (h APIHost) Validate() error {
	if h.Service == nil {
		return fmt.Errorf("ontology api host missing required dependencies: Service")
	}
	if h.Projects == nil {
		return fmt.Errorf("ontology api host missing required dependencies: Projects")
	}
	return nil
}

// MountAPI registers the path-builder autocomplete + ontology-labels endpoints
// under the stable URLs used by the active Svelte path-builder clients.
func MountAPI(parent chi.Router, host APIHost) {
	if err := host.Validate(); err != nil {
		panic(err)
	}
	h := NewHandler(host.Service, host.Logger, host.Languages, host.LangResolver)

	parent.Post("/api/v1/ontology/autocomplete", h.Autocomplete)
	parent.Get("/api/v1/ontology/edge-provenance", h.EdgeProvenance)
	parent.With(auth.RequireProjectRead(host.Projects)).
		Get("/api/projects/{projectID}/ontology-labels", h.OntologyLabels)
}

type PagesHost struct {
	Service   *Service
	Logger    *slog.Logger
	Templates *weavetemplates.Renderer
	I18n      i18n.Manager
	Session   *session.Manager
}

func (h PagesHost) Validate() error {
	if h.Service == nil {
		return fmt.Errorf("ontology pages host missing required dependencies: Service")
	}
	return nil
}

// MountPages registers the public ontology browse pages (/ontologies/...) on
// parent. Skipped when renderer or i18n is not wired, keeping the slice
// boot-safe in minimal test setups.
func MountPages(parent chi.Router, host PagesHost) {
	if err := host.Validate(); err != nil {
		panic(err)
	}
	if host.Templates == nil || host.I18n == nil {
		return
	}
	pages := NewPages(host.Logger, host.Templates, host.Service, host.I18n, host.Session)
	pages.Mount(parent)
}
