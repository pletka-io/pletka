package errortracking

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	weaveauth "github.com/pletka-io/pletka/pkg/auth"
)

// Host is the explicit contract required by the error tracking routes.
type Host struct {
	Pool   *pgxpool.Pool
	Logger *slog.Logger
}

// Mount registers the slice routes:
//
//   - POST /errors/client    — browser error reporter (any authenticated user)
//   - GET  /admin/errors     — viewer JSON (admin only)
//
// The capture middleware is wired separately in pkg/app's global middleware
// stack so it sees every request, not just the slice's own routes.
func Mount(parent chi.Router, h Host) {
	store := NewStore(h.Pool)
	handler := NewHandler(store, h.Logger)

	parent.Post("/errors/client", handler.PostClient)

	parent.Group(func(r chi.Router) {
		r.Use(requireAdmin)
		r.Get("/admin/errors", handler.List)
	})
}

// requireAdmin gates the viewer endpoint to the admin role. Sticks
// to the existing AuthSnapshot for consistency with the rest of the
// weave admin surface.
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
