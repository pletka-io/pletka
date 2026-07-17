package drafts

import (
	"fmt"
	"log/slog"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/pkg/domain"
)

type Host struct {
	Weave  domain.WeaveStore
	Logger *slog.Logger
}

func (h Host) Validate() error {
	if h.Weave == nil {
		return fmt.Errorf("drafts host missing required dependencies: Weave")
	}
	return nil
}

// Mount registers POST /api/v1/drafts directly on parent. The path embeds
// project_id in the body, not the URL, so there's no per-project sub-mux.
func Mount(parent chi.Router, host Host) {
	if err := host.Validate(); err != nil {
		panic(err)
	}
	h := NewHandler(host.Weave, host.Logger)
	parent.Post("/api/v1/drafts", h.Create)
}
