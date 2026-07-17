package field

import (
	"fmt"
	"log/slog"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/formschema"
	"github.com/pletka-io/pletka/pkg/i18n"
	"github.com/pletka-io/pletka/pkg/weave/publication"
)

// Mount registers the slice's routes on r. Caller is responsible for
// the URL prefix — typically the slice mounts at
// /projects/{projectID}/fields via pkg/weave/router.
//
// Routes registered (relative to mount point):
//
//	GET    /                          → List
//	POST   /                          → Create (with co-located base override)
//	GET    /api/{fieldID}             → Detail (JSON)
//	PUT    /{fieldID}                 → Update
//	DELETE /{fieldID}                 → Delete (in-use preflight; 409 with usage)
//	GET    /{fieldID}/models          → ModelRefs
//	GET    /{fieldID}/collections     → CollectionRefs
//	GET    /{fieldID}/stats           → Stats (Field + UsageReport)
//	POST   /{fieldID}/deprecate       → soft-retire
//	POST   /{fieldID}/activate        → reverse soft-retire
//
// The gohtml entity-view page at /{fieldID} (no /api/ prefix) stays
// owned by pkg/weave/detailview (handler-only module per ADR-0001).
func (h *Handler) Mount(r chi.Router) {
	r.Get("/", h.List)
	r.Post("/", h.Create)
	// Lazy-loaded options for the ontology_scope filter (entity list
	// schema → FilterConfig.OptionsURL).
	r.Get("/filters/ontology-scopes", h.OntologyScopesFilter)
	r.Get("/filters/categories", h.CategoriesFilter)
	r.Route("/api", func(r chi.Router) {
		r.Get("/{fieldID}", h.Detail)
	})
	r.Route("/{fieldID}", func(r chi.Router) {
		r.Put("/", h.Update)
		r.Delete("/", h.Delete)
		r.Get("/models", h.ModelRefs)
		r.Get("/collections", h.CollectionRefs)
		r.Get("/stats", h.Stats)
		r.Post("/deprecate", h.Deprecate)
		r.Post("/activate", h.Activate)
	})
}

type Host struct {
	Service      *Service
	Projects     auth.ProjectReader
	Logger       *slog.Logger
	Languages    []formschema.LanguageInfo
	LangResolver LangResolver
	I18n         i18n.Manager
	Publication  *publication.Reader
}

func (h Host) Validate() error {
	if h.Service == nil {
		return fmt.Errorf("field host missing required dependencies: Service")
	}
	if h.Projects == nil {
		return fmt.Errorf("field host missing required dependencies: Projects")
	}
	return nil
}

func Mount(r chi.Router, host Host) {
	if err := host.Validate(); err != nil {
		panic(err)
	}
	r.Use(auth.RequireProjectRead(host.Projects))
	h := NewHandler(host.Service, host.Logger, host.Languages, host.LangResolver, host.I18n)
	h.pub = host.Publication
	h.Mount(r)
}
