package category

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

const SliceID = "category"

// Host is the narrow dependency surface needed to wire the category slice.
type Host struct {
	Store          Store
	SchemaStore    SchemaStore
	Adoptions      domain.AdoptionStore
	Numberer       EntityNumberer
	OriginResolver OriginResolver
	Logger         *slog.Logger
	ChangeLog      domain.ChangeLogRunner
	Languages      []formschema.LanguageInfo
	LangResolver   LangResolver
}

// Validate checks that the host supplies the dependencies required by the
// category slice's route and schema contributions.
func (h Host) Validate() error {
	var missing []string
	if h.Store == nil {
		missing = append(missing, "Store")
	}
	if h.SchemaStore == nil {
		missing = append(missing, "SchemaStore")
	}
	if h.Adoptions == nil {
		missing = append(missing, "Adoptions")
	}
	if h.Numberer == nil {
		missing = append(missing, "Numberer")
	}
	if len(missing) > 0 {
		return fmt.Errorf("category host missing required dependencies: %s", strings.Join(missing, ", "))
	}
	return nil
}

// Slice is a configured category slice that can declare registry
// contributions from an explicit host dependency contract.
type Slice struct {
	Host Host
}

var _ schemaregistry.DescriptorProvider = Slice{}

// Descriptor declares the category slice contributions. Registration of the
// descriptor only means the host binary knows how to provide category surfaces;
// activation remains a separate host-level policy.
func (s Slice) Descriptor() (schemaregistry.SliceDescriptor, error) {
	host := s.Host
	if err := host.Validate(); err != nil {
		return schemaregistry.SliceDescriptor{}, err
	}
	contributions := schemaregistry.Contributions{
		Mounts: []schemaregistry.MountSpec{
			{
				ID:           "category.routes",
				Surface:      schemaregistry.SurfacePublic,
				Pattern:      "/projects/{projectID}/categories",
				Capabilities: []string{"project.read"},
				Mount: func(r chi.Router) {
					NewHandlerFromHost(host).Mount(r)
				},
			},
		},
	}
	contributions.Schemas = append(contributions.Schemas, schemaregistry.SchemaContribution{
		EntityType: SliceID,
		Form:       NewSchemaProvider(host.SchemaStore),
	})
	return schemaregistry.SliceDescriptor{
		ID:            SliceID,
		Label:         i18n.L("category.slice.label", "Categories"),
		Description:   i18n.L("category.slice.description", "Manage project category groups."),
		Capabilities:  []string{"project.read", "project.edit"},
		Contributions: contributions,
	}, nil
}

// NewHandlerFromHost builds the runtime chain from the category-specific host
// dependencies. Tests and host applications can construct this directly.
func NewHandlerFromHost(host Host) *Handler {
	svc := NewService(host.Store, host.Adoptions, host.Logger, host.ChangeLog, host.Numberer)
	return NewHandler(svc, host.OriginResolver, host.Logger, host.Languages, host.LangResolver)
}

// Mount registers the Category slice's routes on r. The caller is
// responsible for the URL prefix — typically the slice is mounted at
// /projects/{projectID}/categories. Auth and CSRF middleware should be
// attached on the parent router; this method does not install any.
//
// Routes registered (relative to the mount point):
//
//	GET    /                       → List (returns []WithCounts)
//	POST   /                       → Create
//	PATCH  /reorder                → Reorder (drag-end on the ListManager)
//	GET    /list-schema            → list view schema (Svelte ListManager)
//	GET    /form-schema            → create-form schema
//	GET    /{id}                   → Get
//	PUT    /{id}                   → Update
//	DELETE /{id}                   → Delete (body may include reassign_to)
//	GET    /{id}/form-schema       → edit-form schema
//	POST   /{id}/deprecate         → Deprecate
//	POST   /{id}/activate          → Activate
//
// Note: the data URL emitted by BuildListSchema points to GET /, which
// returns []WithCounts (categories with counts + in_use boolean). The
// frontend ListManager consumes that directly.
func (h *Handler) Mount(r chi.Router) {
	r.Get("/", h.List)
	r.Post("/", h.Create)
	r.Patch("/reorder", h.Reorder)
	r.Get("/list-schema", h.ListSchema)
	r.Get("/form-schema", h.FormSchemaCreate)
	r.Get("/options", h.Options)

	r.Route("/{id}", func(r chi.Router) {
		r.Get("/", h.Get)
		r.Put("/", h.Update)
		r.Delete("/", h.Delete)
		r.Get("/form-schema", h.FormSchemaEdit)
		r.Get("/stats", h.Stats)
		r.Post("/deprecate", h.Deprecate)
		r.Post("/activate", h.Activate)
	})
}
