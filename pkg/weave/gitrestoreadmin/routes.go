package gitrestoreadmin

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/pkg/weave/admin"
)

type Host struct {
	Store     Store
	Previewer Previewer
	Runner    Runner
	Logger    *slog.Logger
}

func (h Host) Validate() error {
	var missing []string
	if h.Store == nil {
		missing = append(missing, "Store")
	}
	if h.Runner == nil {
		missing = append(missing, "Runner")
	}
	if len(missing) > 0 {
		return fmt.Errorf("gitrestoreadmin host missing required dependencies: %s", strings.Join(missing, ", "))
	}
	return nil
}

func Mount(parent chi.Router, h Host) {
	if err := h.Validate(); err != nil {
		panic(err)
	}
	svc := NewService(h.Store, h.Previewer, h.Runner)
	handler := NewHandler(h.Logger, svc)

	parent.Group(func(r chi.Router) {
		r.Use(admin.RequireSuperAdmin)
		r.Post("/admin/git-restore/preview", handler.Preview)
		r.Get("/admin/git-restore/jobs", handler.ListJobs)
		r.Post("/admin/git-restore/jobs", handler.CreateJob)
		r.Get("/admin/git-restore/jobs/{jobID}", handler.GetJob)
		r.Post("/admin/git-restore/jobs/{jobID}/run", handler.RunJob)
	})
}
