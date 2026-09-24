package project

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/formschema"
	overridepkg "github.com/pletka-io/pletka/pkg/weave/override"
)

type Host struct {
	Service      *Service
	Overrides    *overridepkg.Service
	Weave        domain.WeaveStore
	Logger       *slog.Logger
	Languages    []formschema.LanguageInfo
	LangResolver LangResolver
}

func (h Host) Validate() error {
	var missing []string
	if h.Service == nil {
		missing = append(missing, "Service")
	}
	if h.Overrides == nil {
		missing = append(missing, "Overrides")
	}
	if h.Weave == nil {
		missing = append(missing, "Weave")
	}
	if len(missing) > 0 {
		return fmt.Errorf("project host missing required dependencies: %s", strings.Join(missing, ", "))
	}
	return nil
}

// Mount registers the slice's endpoints on parent. Unlike slices that mount
// under a single sub-path, the project endpoints live across several sub-paths
// under /projects, so each concrete path is mounted independently.
func Mount(parent chi.Router, host Host) {
	if err := host.Validate(); err != nil {
		panic(err)
	}
	h := NewHandler(host.Service, host.Overrides, host.Weave, host.Logger, host.Languages, host.LangResolver)

	// POST /projects → create. Coexists with the pages slice's GET /projects
	// on the same path.
	parent.Post("/projects", h.Create)

	mountAt(parent, "/projects/data", host, func(r chi.Router) {
		r.Get("/", h.Data)
	})
	mountAt(parent, "/projects/entity-list-schema", host, func(r chi.Router) {
		r.Get("/", h.EntityListSchema)
	})
	mountAt(parent, "/projects/filters/institutions", host, func(r chi.Router) {
		r.Get("/", h.FilterInstitutions)
	})
	mountAt(parent, "/projects/check-prefix", host, func(r chi.Router) {
		r.Get("/", h.CheckIDPrefix)
	})
	// /projects/form-schema/project is owned by the entityschema
	// dispatcher (per ADR — handler-only modules own dispatcher URLs
	// and delegate to slice schema builders). Slice still exposes
	// FormSchema as a Handler method for tests + direct callers, but
	// does not register the route.
	mountAt(parent, "/projects/{projectID}/inheritance-tree", host, func(r chi.Router) {
		r.Use(auth.RequireProjectRead(host.Weave.Projects()))
		r.Get("/", h.InheritanceTree)
	})
	mountAt(parent, "/projects/{projectID}/adoptions", host, func(r chi.Router) {
		r.Use(auth.RequireProjectRead(host.Weave.Projects()))
		r.Get("/", h.Adoptions)
		r.Post("/", h.CreateProjectAdoption)
	})
	mountAt(parent, "/projects/{projectID}/adoptable/{entityType}", host, func(r chi.Router) {
		r.Use(auth.RequireProjectRead(host.Weave.Projects()))
		r.Get("/", h.AdoptableEntities)
	})
	mountAt(parent, "/projects/{projectID}/models/{modelID}/overrides", host, func(r chi.Router) {
		r.Use(auth.RequireProjectRead(host.Weave.Projects()))
		r.Get("/", h.ModelOverrides)
		r.Put("/", h.SaveModelOverrides)
		r.Post("/presence", h.ModelOverridesPresence)
	})
	mountAt(parent, "/projects/{projectID}/models/{modelID}/composition/adopt-collection", host, func(r chi.Router) {
		r.Use(auth.RequireProjectRead(host.Weave.Projects()))
		r.Post("/", h.AdoptCollectionIntoModel)
	})
	mountAt(parent, "/projects/{projectID}/composition/sidebar-schema/field", host, func(r chi.Router) {
		r.Use(auth.RequireProjectRead(host.Weave.Projects()))
		r.Get("/", h.OverrideFieldSidebarSchema)
	})
	mountAt(parent, "/projects/{projectID}/composition/sidebar-schema/collection-group", host, func(r chi.Router) {
		r.Use(auth.RequireProjectRead(host.Weave.Projects()))
		r.Get("/", h.OverrideCollectionGroupSidebarSchema)
	})
	mountAt(parent, "/projects/{projectID}/collections/{collectionID}/overrides", host, func(r chi.Router) {
		r.Use(auth.RequireProjectRead(host.Weave.Projects()))
		r.Get("/", h.CollectionOverrides)
		r.Put("/", h.SaveCollectionOverrides)
		r.Post("/presence", h.CollectionOverridesPresence)
	})
}

// mountAt mounts a slice sub-mux at pattern, installing the project
// resource middleware on routes that carry a {projectID} so the
// per-request project + populated auth.Resource is in context.
// Mirrors the mountSlice helper in pkg/weave/router.
func mountAt(parent chi.Router, pattern string, host Host, attach func(chi.Router)) {
	sub := chi.NewMux()
	sub.Use(auth.WithProjectVersionContext)
	sub.Use(auth.WithProjectResource(host.Weave))
	attach(sub)
	parent.Mount(pattern, sub)
}
