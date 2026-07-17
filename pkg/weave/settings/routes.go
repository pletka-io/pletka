package settings

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/go-chi/chi/v5"

	weaveauth "github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/formschema"
)

type Host struct {
	Weave     domain.WeaveStore
	Store     Store
	Logger    *slog.Logger
	Languages []formschema.LanguageInfo
}

func (h Host) Validate() error {
	var missing []string
	if h.Weave == nil {
		missing = append(missing, "Weave")
	}
	if h.Store == nil {
		missing = append(missing, "Store")
	}
	if len(missing) > 0 {
		return fmt.Errorf("settings host missing required dependencies: %s", strings.Join(missing, ", "))
	}
	return nil
}

// Mount registers each settings path on parent via chi.Mount with a fresh
// sub-mux. Mounting per-path (rather than one mount on
// /projects/{projectID}/settings) keeps each settings sub-surface isolated.
func Mount(parent chi.Router, host Host) {
	if err := host.Validate(); err != nil {
		panic(err)
	}
	h := NewHandler(host.Weave, host.Store, host.Languages, host.Logger)

	mountAt(parent, "/projects/{projectID}/settings/schema", host, func(r chi.Router) {
		r.Get("/", h.PageSchema)
	})
	mountAt(parent, "/projects/{projectID}/settings/form-schema/{section}", host, func(r chi.Router) {
		r.Get("/", h.FormSchema)
	})
	mountAt(parent, "/projects/{projectID}/settings/pane-schema/{section}", host, func(r chi.Router) {
		r.Get("/", h.PaneSchema)
	})
	mountAt(parent, "/projects/{projectID}/settings/list-schema/{section}", host, func(r chi.Router) {
		r.Get("/", h.ListSchema)
	})
	mountAt(parent, "/projects/{projectID}/settings/general", host, func(r chi.Router) {
		r.Put("/", h.UpdateGeneral)
	})
	mountAt(parent, "/projects/{projectID}/settings/about", host, func(r chi.Router) {
		r.Put("/", h.UpdateAbout)
	})
	mountAt(parent, "/projects/{projectID}/settings/vocabularies", host, func(r chi.Router) {
		r.Put("/", h.UpdateVocabularies)
	})
	mountAt(parent, "/projects/{projectID}/settings/ontology", host, func(r chi.Router) {
		r.Put("/", h.UpdateOntology)
	})
	mountAt(parent, "/projects/{projectID}/settings/ontology/child-weave-options", host, func(r chi.Router) {
		r.Get("/", h.ChildWeaveOptions)
	})
	mountAt(parent, "/projects/{projectID}/settings/inheritance", host, func(r chi.Router) {
		r.Get("/", h.ListInheritance)
		r.Post("/", h.CreateInheritance)
	})
	mountAt(parent, "/projects/{projectID}/settings/inheritance/release-options", host, func(r chi.Router) {
		r.Get("/", h.InheritanceReleaseOptions)
	})
	mountAt(parent, "/projects/{projectID}/settings/inheritance/reorder", host, func(r chi.Router) {
		r.Patch("/", h.ReorderInheritance)
	})
	mountAt(parent, "/projects/{projectID}/settings/inheritance/{parentID}", host, func(r chi.Router) {
		r.Delete("/", h.DeleteInheritance)
	})
	mountAt(parent, "/projects/{projectID}/settings/inheritance/{parentID}/source", host, func(r chi.Router) {
		r.Put("/", h.UpdateInheritanceSource)
	})
	mountAt(parent, "/projects/{projectID}/settings/inheritance/{parentID}/primary", host, func(r chi.Router) {
		r.Post("/", h.SetPrimaryInheritance)
	})
	// Attributions routes are owned by pkg/weave/attribution and
	// mounted via pkg/weave/router — settings doesn't dispatch them.
}

// mountAt mounts a slice sub-mux at pattern, installing the project
// resource middleware for routes that carry a {projectID}. Mirrors the
// helper in pkg/weave/project + pkg/weave/router so each settings
// sub-path is isolated from the legacy /projects route group during
// the strangler-fig period.
func mountAt(parent chi.Router, pattern string, host Host, attach func(chi.Router)) {
	sub := chi.NewMux()
	sub.Use(weaveauth.WithProjectVersionContext)
	sub.Use(weaveauth.WithProjectResource(host.Weave))
	attach(sub)
	parent.Mount(pattern, sub)
}
