package attribution

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/pkg/formschema"
	"github.com/pletka-io/pletka/pkg/i18n"
	schemaregistry "github.com/pletka-io/pletka/pkg/schemaui/registry"
)

const SliceID = "attribution"

// Host is the narrow dependency surface needed to wire the attribution slice.
type Host struct {
	Store     Store
	Projects  ProjectReader
	Logger    *slog.Logger
	Languages []formschema.LanguageInfo
}

// Validate checks that the host supplies the dependencies required by the
// attribution route contribution.
func (h Host) Validate() error {
	var missing []string
	if h.Store == nil {
		missing = append(missing, "Store")
	}
	if h.Projects == nil {
		missing = append(missing, "Projects")
	}
	if len(missing) > 0 {
		return fmt.Errorf("attribution host missing required dependencies: %s", strings.Join(missing, ", "))
	}
	return nil
}

// Slice is a configured attribution slice that declares registry
// contributions from an explicit host dependency contract.
type Slice struct {
	Host Host
}

var _ schemaregistry.DescriptorProvider = Slice{}

// Descriptor declares the attribution slice contributions.
func (s Slice) Descriptor() (schemaregistry.SliceDescriptor, error) {
	host := s.Host
	if err := host.Validate(); err != nil {
		return schemaregistry.SliceDescriptor{}, err
	}
	return schemaregistry.SliceDescriptor{
		ID:           SliceID,
		Label:        i18n.L("attribution.slice.label", "Attributions"),
		Description:  i18n.L("attribution.slice.description", "Manage project credits and contributors."),
		Capabilities: []string{"project.read", "project.edit"},
		Contributions: schemaregistry.Contributions{
			Mounts: []schemaregistry.MountSpec{
				{
					ID:           "attribution.routes",
					Surface:      schemaregistry.SurfaceSettings,
					Pattern:      "/projects/{projectID}/settings/attributions",
					Capabilities: []string{"project.read"},
					Mount: func(r chi.Router) {
						NewHandlerFromHost(host).Mount(r)
					},
				},
			},
		},
	}, nil
}

// NewHandlerFromHost builds the runtime chain from attribution host
// dependencies.
func NewHandlerFromHost(host Host) *Handler {
	svc := NewService(host.Store, host.Projects, host.Logger)
	return NewHandler(svc, host.Logger, host.Languages)
}

// Mount registers the attribution slice's routes on r. The caller is
// responsible for the URL prefix — typically the slice is mounted at
// /projects/{projectID}/settings/attributions.
//
// Routes registered (relative to the mount point):
//
//	GET    /                                       → List
//	POST   /                                       → Create
//	GET    /list-schema                            → settings list schema
//	GET    /form-schema                            → create-form schema
//	PATCH  /reorder                                → Reorder within a kind
//	PATCH  /{actorID}/{kind}/{position}            → UpdateNote
//	DELETE /{actorID}/{kind}/{position}            → Delete
func (h *Handler) Mount(r chi.Router) {
	r.Get("/", h.List)
	r.Post("/", h.Create)
	r.Get("/list-schema", h.ListSchema)
	r.Get("/form-schema", h.FormSchema)
	r.Get("/options/actors", h.OptionsActors)
	r.Patch("/reorder", h.Reorder)
	r.Route("/{actorID}/{kind}/{position}", func(r chi.Router) {
		r.Patch("/", h.UpdateNote)
		r.Delete("/", h.Delete)
	})
}
