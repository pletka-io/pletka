package example

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/browserhttp"
	"github.com/pletka-io/pletka/pkg/formschema"
)

// BrowserSource binds inventories and recorded-value views to one boundary.
type BrowserSource interface {
	browserhttp.Source
	browserhttp.DetailSource
}

type Host struct {
	Browser      BrowserSource
	Service      *Service
	Projects     auth.ProjectReader
	Logger       *slog.Logger
	Languages    []formschema.LanguageInfo
	LangResolver LangResolver
}

func (h Host) Validate() error {
	var missing []string
	if h.Browser == nil {
		missing = append(missing, "Browser")
	}
	if h.Service == nil {
		missing = append(missing, "Service")
	}
	if h.Projects == nil {
		missing = append(missing, "Projects")
	}
	if len(missing) > 0 {
		return fmt.Errorf("example host missing required dependencies: %s", strings.Join(missing, ", "))
	}
	return nil
}

func Mount(r chi.Router, host Host) {
	if err := host.Validate(); err != nil {
		panic(err)
	}
	h := NewHandler(host.Service, host.Logger, host.Languages, host.LangResolver)
	h.reads = readHandler{source: host.Browser, maxDepth: host.Service.maxDepth, lang: h.lang, languages: host.Languages}
	h.inventory = browserhttp.Handler{Source: host.Browser, Lang: host.LangResolver}
	r.With(auth.RequireProjectRead(host.Projects)).Get("/", h.List)
	r.With(auth.RequireProjectEdit(host.Projects)).Post("/", h.Create)
	r.With(auth.RequireProjectRead(host.Projects)).Get("/form-schema", h.FormSchema)
	r.Route("/{exampleID}", func(r chi.Router) {
		r.With(auth.RequireProjectRead(host.Projects)).Get("/", h.Detail)
		r.With(auth.RequireProjectEdit(host.Projects)).Put("/", h.Update)
		r.With(auth.RequireProjectEdit(host.Projects)).Delete("/", h.Delete)
	})
}
