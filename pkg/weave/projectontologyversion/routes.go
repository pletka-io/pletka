package projectontologyversion

import (
	"fmt"
	"log/slog"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/pkg/formschema"
)

// Mount registers the slice's routes on r. The caller is responsible for
// the URL prefix — typically the slice is mounted at
// /projects/{projectID}/project-ontology-versions via pkg/weave/router.
//
// Routes registered (relative to mount point):
//
//	GET    /                            → List (own + inherited groups)
//	GET    /pane                        → Rich PaneView for the Svelte settings ontology pane
//	POST   /                            → Create (base + extensions)
//	GET    /list-schema                 → list view schema
//	GET    /form-schema                 → form schema (?mode=create|edit)
//	GET    /options/versions            → dependent select: versions for a base
//	GET    /options/extensions          → dependent select: extensions for a base version
//	PATCH  /{versionID}                 → Update (is_primary, usage_notes)
//	DELETE /{versionID}                 → Delete (cascade-aware, in-use guard)
//	GET    /{versionID}/stats           → Usage stats for one version
//
// PATCH (not PUT) is used because the update payload is partial. Auth and
// CSRF middleware should be attached on the parent router.
func (h *Handler) Mount(r chi.Router) {
	r.Get("/", h.List)
	r.Get("/pane", h.Pane)
	r.Post("/", h.Create)
	r.Get("/list-schema", h.ListSchema)
	r.Get("/form-schema", h.FormSchema)

	r.Route("/options", func(r chi.Router) {
		r.Get("/versions", h.OptionsVersions)
		r.Get("/extensions", h.OptionsExtensions)
	})

	r.Patch("/{versionID}", h.Patch)
	r.Patch("/{versionID}/", h.Patch)
	r.Delete("/{versionID}", h.Delete)
	r.Delete("/{versionID}/", h.Delete)
	r.Get("/{versionID}/stats", h.Stats)
}

type Host struct {
	Service      *Service
	Logger       *slog.Logger
	Languages    []formschema.LanguageInfo
	LangResolver LangResolver
}

func (h Host) Validate() error {
	if h.Service == nil {
		return fmt.Errorf("project ontology version host missing required dependencies: Service")
	}
	return nil
}

func Mount(r chi.Router, host Host) {
	if err := host.Validate(); err != nil {
		panic(err)
	}
	h := NewHandler(host.Service, host.Logger, host.Languages, host.LangResolver)
	h.Mount(r)
}
