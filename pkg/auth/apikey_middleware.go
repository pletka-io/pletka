package auth

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/apierror"
)

// APIKeyVerifier resolves a presented API key secret to its active key.
// Implemented by pkg/weave/apikey.Service.
type APIKeyVerifier interface {
	Verify(ctx context.Context, secret string) (*domain.APIKey, error)
}

// RequireAPIKey authenticates the request via an Authorization bearer API
// key and attaches the actor's AuthSnapshot and Principal to the context.
// There is no session fallback: a missing or invalid key is a 401. It
// overrides both the snapshot and the principal set by earlier session
// middleware, so a stale session Principal never survives onto a bearer
// key request.
func RequireAPIKey(keys APIKeyVerifier, ws domain.WeaveStore, log *slog.Logger) func(http.Handler) http.Handler {
	if log == nil {
		log = slog.Default()
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			secret, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
			if !ok || secret == "" {
				apierror.Write(w, apierror.Unauthorized())
				return
			}
			key, err := keys.Verify(r.Context(), secret)
			if err != nil {
				apierror.Write(w, apierror.Unauthorized())
				return
			}

			ctx := r.Context()
			snap, err := BuildSnapshot(ctx, ws, key.ActorID)
			if err != nil {
				log.Error("api key snapshot build failed", "actor_id", key.ActorID, "err", err)
				apierror.Write(w, apierror.Unauthorized())
				return
			}
			ctx = WithSnapshot(ctx, snap)

			principal := &Principal{ActorID: key.ActorID, IsActive: true}
			switch profile, perr := ws.Auth().GetProfileByActorID(ctx, key.ActorID); {
			case perr != nil:
				log.Warn("api key principal degraded", "actor_id", key.ActorID, "err", perr)
			case profile == nil:
				log.Warn("api key principal degraded", "actor_id", key.ActorID, "err", "profile not found")
			default:
				principal = &Principal{
					ActorID:     profile.ActorID,
					Slug:        profile.Slug,
					Email:       profile.Email,
					DisplayName: profile.DisplayName,
					Role:        profile.Role,
					IsActive:    true,
				}
			}
			ctx = WithPrincipal(ctx, principal)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
