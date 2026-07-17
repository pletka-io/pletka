package collection

import (
	"fmt"
	"log/slog"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/formschema"
	"github.com/pletka-io/pletka/pkg/i18n"
	"github.com/pletka-io/pletka/pkg/weave/publication"
)

// Mount registers the slice's routes on r. Caller mounts at
// /projects/{projectID}/collections via pkg/weave/router.
//
// Routes registered (relative to mount point):
//
//	GET    /                              → List
//	POST   /                              → Create (metadata only)
//	GET    /api/{collectionID}            → Detail
//	PUT    /{collectionID}                → Update
//	DELETE /{collectionID}                → Delete (in-use preflight)
//	GET    /{collectionID}/stats          → Stats
//	POST   /{collectionID}/deprecate      → soft-retire
//	POST   /{collectionID}/activate       → reverse soft-retire
//	POST   /{collectionID}/fork           → fork adopted collection into local editable copy
//	GET    /{collectionID}/overrides      → flat list of collection-context overrides
//	PUT    /{collectionID}/overrides      → bulk replace via override.Service
func (h *Handler) Mount(r chi.Router) {
	r.Get("/", h.List)
	r.Post("/", h.Create)
	r.Get("/filters/scope-classes", h.ScopeClassesOptions)
	r.Get("/filters/categories", h.CategoriesOptions)
	r.Route("/api", func(r chi.Router) {
		r.Get("/{collectionID}", h.Detail)
	})
	r.Route("/{collectionID}", func(r chi.Router) {
		r.Put("/", h.Update)
		r.Delete("/", h.Delete)
		r.Get("/stats", h.Stats)
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
		return fmt.Errorf("collection host missing required dependencies: Service")
	}
	if h.Projects == nil {
		return fmt.Errorf("collection host missing required dependencies: Projects")
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
