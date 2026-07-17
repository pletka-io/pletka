package namespacebinding

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/formschema"
	"github.com/pletka-io/pletka/pkg/i18n"
	schemaregistry "github.com/pletka-io/pletka/pkg/schemaui/registry"
)

const SliceID = "namespacebinding"

// Host is the narrow dependency surface needed to wire the namespace-binding
// slice.
type Host struct {
	Store        Store
	Logger       *slog.Logger
	ChangeLog    domain.ChangeLogRunner
	Languages    []formschema.LanguageInfo
	LangResolver LangResolver
}

// Validate checks that the host supplies the dependencies required by the
// namespace-binding route contributions.
func (h Host) Validate() error {
	var missing []string
	if h.Store == nil {
		missing = append(missing, "Store")
	}
	if len(missing) > 0 {
		return fmt.Errorf("namespacebinding host missing required dependencies: %s", strings.Join(missing, ", "))
	}
	return nil
}

// Slice is a configured namespace-binding slice that declares registry
// contributions from an explicit host dependency contract.
type Slice struct {
	Host Host
}

var _ schemaregistry.DescriptorProvider = Slice{}

// Descriptor declares the namespace-binding slice contributions.
func (s Slice) Descriptor() (schemaregistry.SliceDescriptor, error) {
	host := s.Host
	if err := host.Validate(); err != nil {
		return schemaregistry.SliceDescriptor{}, err
	}
	return schemaregistry.SliceDescriptor{
		ID:           SliceID,
		Label:        i18n.L("namespacebinding.slice.label", "Namespace bindings"),
		Description:  i18n.L("namespacebinding.slice.description", "Manage namespace prefixes used by project data."),
		Capabilities: []string{"project.read", "project.edit"},
		Contributions: schemaregistry.Contributions{
			Mounts: []schemaregistry.MountSpec{
				{
					ID:           "namespacebinding.routes",
					Surface:      schemaregistry.SurfacePublic,
					Pattern:      "/projects/{projectID}/namespace-bindings",
					Capabilities: []string{"project.read"},
					Mount: func(r chi.Router) {
						NewHandlerFromHost(host).Mount(r)
					},
				},
				{
					ID:           "namespacebinding.admin.routes",
					Surface:      schemaregistry.SurfaceAdmin,
					Pattern:      "/admin/namespaces",
					Capabilities: []string{"admin.namespaces"},
					Mount: func(r chi.Router) {
						NewHandlerFromHost(host).MountAdmin(r)
					},
				},
			},
		},
	}, nil
}

// NewHandlerFromHost builds the runtime chain from namespace-binding host
// dependencies.
func NewHandlerFromHost(host Host) *Handler {
	svc := NewService(host.Store, host.Logger, host.ChangeLog)
	return NewHandler(svc, host.Logger, host.Languages, host.LangResolver)
}

// Mount registers the namespace-binding slice's routes on r. Caller is
// responsible for the URL prefix — typically the slice is mounted at
// /projects/{projectID}/namespace-bindings via pkg/weave/router.
//
// Routes registered (relative to mount point):
//
//	GET    /                       → List (returns []bindingItem)
//	POST   /                       → Create
//	GET    /list-schema            → list view schema
//	GET    /form-schema            → create-form schema
//	GET    /{id}                   → Get
//	PATCH  /{id}                   → Update (partial)
//	DELETE /{id}                   → Delete
//	GET    /{id}/form-schema       → edit-form schema
//
// PATCH (not PUT) is used for the update verb because bindings update
// arrives partial (only fields the user changed). Mirrors the legacy
// frontend's expectation.
func (h *Handler) Mount(r chi.Router) {
	r.Get("/", h.List)
	r.Post("/", h.Create)
	r.Get("/list-schema", h.ListSchema)
	r.Get("/form-schema", h.FormSchemaCreate)

	r.Route("/{id}", func(r chi.Router) {
		r.Get("/", h.Get)
		r.Patch("/", h.Update)
		r.Delete("/", h.Delete)
		r.Get("/form-schema", h.FormSchemaEdit)
	})
}

func (h *Handler) MountAdmin(r chi.Router) {
	r.Get("/", h.ListGlobal)
	r.Get("/data", h.ListGlobalData)
	r.Post("/", h.CreateGlobal)
	r.Get("/list-schema", h.ListSchemaGlobal)
	r.Get("/entity-list-schema", h.EntityListSchemaGlobal)
	r.Get("/form-schema", h.FormSchemaCreateGlobal)

	r.Route("/{id}", func(r chi.Router) {
		r.Get("/", h.GetGlobal)
		r.Get("/stats", h.StatsGlobal)
		r.Patch("/", h.UpdateGlobal)
		r.Delete("/", h.DeleteGlobal)
		r.Get("/form-schema", h.FormSchemaEditGlobal)
	})
}
