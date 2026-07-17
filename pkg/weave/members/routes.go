package members

import (
	"fmt"
	"log/slog"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/pkg/formschema"
)

type Host struct {
	Service      *Service
	Logger       *slog.Logger
	Languages    []formschema.LanguageInfo
	LangResolver LangResolver
}

func (h Host) Validate() error {
	if h.Service == nil {
		return fmt.Errorf("members host missing required dependencies: Service")
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

// Mount registers Member routes on r. Caller is responsible for the
// URL prefix — typically the slice mounts at
// /projects/{projectID}/members.
//
// Routes (relative to mount point):
//
//	GET    /                       → List members
//	POST   /                       → Add member (resolves email/slug to actor)
//	GET    /list-schema            → ListSchema for ListManager
//	GET    /form-schema            → Add-member form schema
//	PATCH  /{actorID}              → Change member role
//	DELETE /{actorID}              → Remove member
func (h *Handler) Mount(r chi.Router) {
	r.Get("/", h.List)
	r.Post("/", h.Create)
	r.Get("/list-schema", h.ListSchema)
	r.Get("/form-schema", h.FormSchema)
	r.Get("/options/actors", h.OptionsActors)

	r.Route("/{actorID}", func(r chi.Router) {
		r.Get("/form-schema", h.FormSchemaEdit)
		r.Patch("/", h.Patch)
		r.Delete("/", h.Delete)
	})
}
