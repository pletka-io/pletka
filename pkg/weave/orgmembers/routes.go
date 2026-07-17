package orgmembers

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/formschema"
)

type OrganizationReader interface {
	GetBySlug(ctx context.Context, slug string) (*domain.Organization, error)
}

type Host struct {
	Service      *Service
	OrgReader    OrganizationReader
	Logger       *slog.Logger
	Languages    []formschema.LanguageInfo
	LangResolver LangResolver
}

func (h Host) Validate() error {
	var missing []string
	if h.Service == nil {
		missing = append(missing, "Service")
	}
	if h.OrgReader == nil {
		missing = append(missing, "OrgReader")
	}
	if len(missing) > 0 {
		return fmt.Errorf("orgmembers host missing required dependencies: %s", strings.Join(missing, ", "))
	}
	return nil
}

func Mount(parent chi.Router, host Host) {
	if err := host.Validate(); err != nil {
		panic(err)
	}
	h := NewHandler(host.Service, host.Logger, host.Languages, host.LangResolver)

	sub := chi.NewMux()
	h.Mount(sub)
	parent.With(auth.WithOrgResource(host.OrgReader), auth.RequireOrgEdit).Mount("/orgs/{slug}/members", sub)
}

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
