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
// overrides any snapshot set by earlier session middleware.
func RequireAPIKey(keys APIKeyVerifier, ws domain.WeaveStore, log *slog.Logger) func(http.Handler) http.Handler {
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
			if profile, err := ws.Auth().GetProfileByActorID(ctx, key.ActorID); err == nil && profile != nil {
				ctx = WithPrincipal(ctx, &Principal{
					ActorID:     profile.ActorID,
					Slug:        profile.Slug,
					Email:       profile.Email,
					DisplayName: profile.DisplayName,
					Role:        profile.Role,
					IsActive:    true,
				})
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
