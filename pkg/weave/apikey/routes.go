package apikey

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/pkg/formschema"
)

// Host is the dependency surface for the actor-scoped /me/api-keys HTTP
// routes.
type Host struct {
	Service      *Service
	Logger       *slog.Logger
	Languages    []formschema.LanguageInfo
	LangResolver func(*http.Request) string
}

// Validate reports missing required dependencies.
func (h Host) Validate() error {
	var missing []string
	if h.Service == nil {
		missing = append(missing, "Service")
	}
	if h.LangResolver == nil {
		missing = append(missing, "LangResolver")
	}
	if len(missing) > 0 {
		return fmt.Errorf("apikey host missing required dependencies: %s", strings.Join(missing, ", "))
	}
	return nil
}

// Mount registers the self-service API-key routes. Auth is handler-level
// (the actoradmin MountSelf model): anonymous callers get a 401 envelope.
func Mount(parent chi.Router, host Host) {
	if err := host.Validate(); err != nil {
		panic(err) // wiring-time fail-fast
	}
	handler := NewHandler(host)
	parent.Get("/me/api-keys", handler.List)
	parent.Get("/me/api-keys/list-schema", handler.ListSchema)
	parent.Get("/me/api-keys/form-schema", handler.FormSchema)
	parent.Post("/me/api-keys", handler.Create)
	parent.Post("/me/api-keys/{id}/revoke", handler.Revoke)
}
