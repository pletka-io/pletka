package admin

import (
	"net/http"

	weaveauth "github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/weave/errresp"
)

// RequireSuperAdmin hides admin routes from non-superadmins with the same
// 404 behavior used by the admin shell pages.
func RequireSuperAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		snap := weaveauth.FromContext(r.Context())
		if snap == nil || snap.IsAnonymous || !snap.IsSuperAdmin {
			errresp.Error(w, r, http.StatusNotFound, "not_found", "not found")
			return
		}
		next.ServeHTTP(w, r)
	})
}
