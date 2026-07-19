package auth

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// AuthAPIHandler is the narrow contract this slice mounts. The URL contract
// is the part that needs to stay stable for the frontend.
type AuthAPIHandler interface {
	Login(w http.ResponseWriter, r *http.Request)
	Register(w http.ResponseWriter, r *http.Request)
	Logout(w http.ResponseWriter, r *http.Request)
	Me(w http.ResponseWriter, r *http.Request)
}

// ssoEnabledSetter is implemented by *AuthHandler. It stays separate from
// AuthAPIHandler because that interface is scoped to the URL contract;
// ssoEnabled is request-time handler state, not a route.
type ssoEnabledSetter interface {
	SetSSOEnabled(bool)
}

// MountAPIRoutes registers /api/v1/auth/* on parent. The slice owns
// the URL shape and method mapping; the caller (cmd/serve.go) owns
// handler construction. Decoupling this from root router construction keeps
// login/register route ownership in the weave auth slice. When
// registrationEnabled is false, /register is not mounted at all (404),
// so the JSON API can't create accounts on an instance where the
// features.registration flag is off. When ssoEnabled is true, password
// login is restricted to super-admin break-glass (see AuthHandler.Login) —
// the flag is passed through to any handler implementing ssoEnabledSetter.
func MountAPIRoutes(parent chi.Router, authH AuthAPIHandler, registrationEnabled, ssoEnabled bool) {
	if s, ok := authH.(ssoEnabledSetter); ok {
		s.SetSSOEnabled(ssoEnabled)
	}
	parent.Route("/api/v1/auth", func(r chi.Router) {
		// Public endpoints
		r.Post("/login", authH.Login)
		if registrationEnabled {
			r.Post("/register", authH.Register)
		}
		r.Post("/logout", authH.Logout)

		// Profile read endpoint (handler does its own auth gating).
		// PUT for self-edit lives at /me in pkg/weave/actoradmin;
		// schema-driven via FormRenderer, weave-native end-to-end.
		r.Get("/me", authH.Me)
	})
}
