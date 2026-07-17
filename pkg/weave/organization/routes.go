package organization

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/formschema"
)

type Host struct {
	Service      *Service
	Logger       *slog.Logger
	Languages    []formschema.LanguageInfo
	LangResolver LangResolver
}

func (h Host) Validate() error {
	var missing []string
	if h.Service == nil {
		missing = append(missing, "Service")
	}
	if len(missing) > 0 {
		return fmt.Errorf("organization host missing required dependencies: %s", strings.Join(missing, ", "))
	}
	return nil
}

func Mount(parent chi.Router, host Host) {
	if err := host.Validate(); err != nil {
		panic(err)
	}
	h := NewHandler(host.Service, host.Logger, host.Languages, host.LangResolver)

	parent.Get("/profile/orgs/form-schema", h.OrgCreateFormSchema)
	parent.Post("/profile/orgs", h.CreateSelf)

	parent.Get("/orgs/entity-list-schema", h.EntityListSchema)
	parent.Get("/orgs/data", h.Data)
	// Lazy-loaded options for the country filter on /orgs.
	// Returns {options: [{value, label}]} per FilterConfig contract.
	parent.Get("/orgs/filters/countries", h.CountriesFilter)
	parent.With(auth.WithOrgResource(host.Service), auth.RequireOrgEdit).Get("/orgs/{slug}/settings/schema", h.SettingsSchema)
	parent.With(auth.WithOrgResource(host.Service), auth.RequireOrgEdit).Get("/orgs/{slug}/settings/form-schema/general", h.GeneralFormSchema)
	parent.With(auth.WithOrgResource(host.Service), auth.RequireOrgEdit).Put("/orgs/{slug}/settings/general", h.UpdateGeneral)
}
