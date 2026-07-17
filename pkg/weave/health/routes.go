package health

import (
	"log/slog"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Host is the explicit contract required by the health routes.
type Host struct {
	Pool   *pgxpool.Pool
	Logger *slog.Logger
}

// Mount registers the liveness paths on parent. Same handler under
// each path; the multiple URLs exist so load balancers
// (typically /healthz), operator dashboards (/health), and legacy
// monitoring (/api/v1/health) all answer.
func Mount(parent chi.Router, h Host) {
	handler := NewHandler(h.Pool, h.Logger)

	parent.Get("/healthz", handler.Check)
	parent.Get("/readyz", handler.Check)
	parent.Get("/health", handler.Check)
	parent.Get("/api/v1/health", handler.Check)
}
