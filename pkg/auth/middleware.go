package auth

import (
	"log/slog"
	"net/http"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/session"
)

// NewMiddleware returns an http middleware that reads the user's id from the
// SCS session, builds an AuthSnapshot from the weave store, and stores it on
// the request context. Downstream handlers retrieve it via auth.FromContext.
//
// When a user_id is present, the middleware also loads the profile and sets
// an auth.Principal on the request context for identity/display data.
//
// Failures to load the snapshot are logged and a SAFE anonymous snapshot is
// used instead. This keeps pages working when the DB is briefly unavailable,
// and the schema builders simply emit read-only UI.
func NewMiddleware(sm *session.Manager, ws domain.WeaveStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			userID := sm.UserID(ctx)

			// Stale-session signal for the frontend. A request that carries a
			// session cookie but resolves to no user has an expired/invalidated
			// session (the browser still holds the old cookie). Flagging it here
			// — before any downstream handler writes its status — lets the
			// client's fetch interceptor show the "session expired" prompt on
			// whatever the request returns (401/403, or a 404 from the
			// existence-hiding project-read gate), without changing any status
			// code or leaking project existence. Header set on every matching
			// response; the interceptor only acts on non-ok ones.
			if userID == "" {
				if c, cerr := r.Cookie(sm.Cookie.Name); cerr == nil && c.Value != "" {
					w.Header().Set("X-Session-Expired", "1")
				}
			}

			snap, err := BuildSnapshot(ctx, ws, userID)
			if err != nil {
				slog.Error("build auth snapshot", "err", err, "user_id", userID, "path", r.URL.Path)
				snap = &AuthSnapshot{
					IsAnonymous:     true,
					Roles:           map[string]string{},
					OwnedProjectIDs: map[string]struct{}{},
				}
			}
			ctx = WithSnapshot(ctx, snap)

			principalPopulated := false
			if userID != "" {
				profile, perr := ws.Auth().GetProfileByActorID(ctx, userID)
				switch {
				case perr != nil:
					slog.Warn("auth middleware: profile lookup failed",
						"user_id", userID, "path", r.URL.Path, "err", perr)
				case profile == nil:
					slog.Warn("auth middleware: profile nil for known user_id",
						"user_id", userID, "path", r.URL.Path)
				default:
					ctx = WithPrincipal(ctx, &Principal{
						ActorID:     profile.ActorID,
						Slug:        profile.Slug,
						Email:       profile.Email,
						DisplayName: profile.DisplayName,
						Role:        profile.Role,
						IsActive:    true,
					})
					principalPopulated = true
				}
			}

			// Debug trace so we can see why downstream handlers may treat the
			// request as anonymous despite a session being present. Drop once
			// the auth race-condition hunt is closed.
			slog.Debug("auth middleware",
				"path", r.URL.Path,
				"method", r.Method,
				"session_user_id", userID,
				"snap_anonymous", snap.IsAnonymous,
				"snap_actor_id", snap.ActorID,
				"snap_super_admin", snap.IsSuperAdmin,
				"principal_set", principalPopulated,
			)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
