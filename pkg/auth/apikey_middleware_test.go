package auth

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
)

type fakeVerifier struct{ key *domain.APIKey }

func (f *fakeVerifier) Verify(_ context.Context, secret string) (*domain.APIKey, error) {
	if f.key != nil && secret == "pk_good" {
		return f.key, nil
	}
	return nil, errors.New("invalid api key")
}

func TestRequireAPIKey(t *testing.T) {
	verifier := &fakeVerifier{key: &domain.APIKey{ID: "k1", ActorID: "actor1"}}
	// ws == nil is safe in this test: the middleware must consult the
	// verifier BEFORE any store access, and 401 paths never touch ws.
	var seen *AuthSnapshot
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = FromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	cases := []struct {
		name       string
		authHeader string
		wantStatus int
	}{
		{"missing header", "", http.StatusUnauthorized},
		{"malformed", "Basic zzz", http.StatusUnauthorized},
		{"bad key", "Bearer pk_bad", http.StatusUnauthorized},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mw := RequireAPIKey(verifier, nil, slog.Default())
			req := httptest.NewRequest("POST", "/mcp", nil)
			if tc.authHeader != "" {
				req.Header.Set("Authorization", tc.authHeader)
			}
			rec := httptest.NewRecorder()
			mw(next).ServeHTTP(rec, req)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
			if ct := rec.Header().Get("Content-Type"); tc.wantStatus == 401 && ct == "" {
				t.Fatal("401 must carry the apierror JSON envelope")
			}
		})
	}
	if seen != nil {
		t.Fatal("next handler must not run on auth failure")
	}
}
