package model

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
// /projects/{projectID}/models via pkg/weave/router.
//
// Routes registered (relative to mount point):
//
//	GET    /                          → List
//	POST   /                          → Create (metadata only)
//	GET    /api/{modelID}             → Detail (JSON)
//	PUT    /{modelID}                 → Update
//	DELETE /{modelID}                 → Delete (in-use preflight)
//	GET    /{modelID}/stats           → Stats
//	POST   /{modelID}/deprecate       → soft-retire
//	POST   /{modelID}/activate        → reverse soft-retire
//	GET    /{modelID}/overrides       → list of model-context override rows
//	PUT    /{modelID}/overrides       → bulk replace via override.Service
//
// gohtml entity-view at /{modelID} (no /api/ prefix) stays in
// pkg/weave/detailview.
func (h *Handler) Mount(r chi.Router) {
	r.Get("/", h.List)
	r.Post("/", h.Create)
	r.Get("/filters/scope-classes", h.ScopeClassesOptions)
	r.Route("/api", func(r chi.Router) {
		r.Get("/{modelID}", h.Detail)
	})
	r.Route("/{modelID}", func(r chi.Router) {
		r.Put("/", h.Update)
		r.Delete("/", h.Delete)
		r.Get("/stats", h.Stats)
		r.Post("/adopt", h.Adopt)
		r.Post("/fork", h.Fork)
		r.Post("/deprecate", h.Deprecate)
		r.Post("/activate", h.Activate)
		r.Get("/overrides", h.ListOverrides)
		r.Put("/overrides", h.SaveOverrides)
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
		return fmt.Errorf("model host missing required dependencies: Service")
	}
	if h.Projects == nil {
		return fmt.Errorf("model host missing required dependencies: Projects")
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
