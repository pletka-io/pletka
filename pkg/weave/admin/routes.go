package admin

import (
	"log/slog"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/pkg/formschema"
	"github.com/pletka-io/pletka/pkg/i18n"
	"github.com/pletka-io/pletka/pkg/session"
	weavetemplates "github.com/pletka-io/pletka/pkg/weave/templates"
)

// Host is the explicit contract needed by the global admin shell.
type Host struct {
	Logger    *slog.Logger
	Templates *weavetemplates.Renderer
	I18n      i18n.Manager
	Session   *session.Manager
	Sections  []formschema.AdminSection
}

// Mount wires the global admin schema and shell page.
func Mount(parent chi.Router, h Host) {
	NewHandler(h.Sections...).Mount(parent)
	if h.Templates != nil && h.I18n != nil {
		NewPages(h.Logger, h.Templates, h.I18n, h.Session).Mount(parent)
	}
}

func (h *Handler) Mount(r chi.Router) {
	r.Get("/admin/schema", h.Schema)
}
