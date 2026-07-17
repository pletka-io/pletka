package exports

import (
	"fmt"
	"log/slog"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/pkg/weave/generators"
)

type Host struct {
	Service  *generators.Service
	Projects ProjectGetter
	Logger   *slog.Logger
}

func (h Host) Validate() error {
	if h.Service == nil {
		return fmt.Errorf("exports host missing required dependencies: Service")
	}
	if h.Projects == nil {
		return fmt.Errorf("exports host missing required dependencies: Projects")
	}
	return nil
}

// Mount registers per-entity CSV download routes on a chi.Router that's
// already scoped under /projects/{projectID}/exports. The legacy
// 8-type project-wide CSV list + zip is mounted separately by
// pkg/weave/project/csvexport at the same path prefix.
//
// Routes registered here:
//
//	GET /{entityKind}/{entityID}        — single-entity CSV
//	GET /{entityKind}/{entityID}.csv    — same, with .csv suffix
func (h *Handler) Mount(r chi.Router) {
	r.Get("/{entityKind}/{entityID}", h.ExportCSV)
}

func Mount(r chi.Router, host Host) {
	if err := host.Validate(); err != nil {
		panic(err)
	}
	NewHandler(host.Service, host.Projects, host.Logger).Mount(r)
}
