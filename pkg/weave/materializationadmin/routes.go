package materializationadmin

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	weaveauth "github.com/pletka-io/pletka/pkg/auth"
)

// Host is the explicit contract required by the materialization viewer routes.
type Host struct {
	Pool   *pgxpool.Pool
	Logger *slog.Logger
}

// Mount registers the viewer:
//
//   - GET /admin/materialization — JSON (admin only)
func Mount(parent chi.Router, h Host) {
	handler := NewHandler(NewStore(h.Pool), h.Logger)
	parent.Group(func(r chi.Router) {
		r.Use(requireAdmin)
		r.Get("/admin/materialization", handler.List)
	})
}

// requireAdmin gates the viewer to super-admins, matching the rest of the
// weave admin surface (mirrors errortracking.requireAdmin).
func requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		snap := weaveauth.FromContext(r.Context())
		if snap == nil || snap.IsAnonymous || !snap.IsSuperAdmin {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
