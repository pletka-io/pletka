package version

import (
	"log/slog"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Host is the explicit contract required by the version routes.
type Host struct {
	Pool     *pgxpool.Pool
	Logger   *slog.Logger
	Instance string
}

// Mount registers GET /version. Public, no auth — same as the health probes.
// ?refresh=1 forces a re-read of the migration version + frontend manifest
// fingerprint.
func Mount(parent chi.Router, h Host) {
	handler := NewHandler(h.Pool, h.Logger, h.Instance)
	parent.Get("/version", handler.Check)
	parent.Get("/api/v1/version", handler.Check)
}
