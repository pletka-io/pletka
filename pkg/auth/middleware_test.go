//go:build integration

package auth_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/session"
	"github.com/pletka-io/pletka/pkg/weave"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ensureSessionsTable creates the sessions table if it does not already exist.
// This is needed for integration tests that use the pgx-backed session store.
func ensureSessionsTable(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		CREATE TABLE IF NOT EXISTS sessions (
			token TEXT PRIMARY KEY,
			data BYTEA NOT NULL,
			expiry TIMESTAMPTZ NOT NULL
		)
	`)
	if err != nil {
		t.Fatalf("create sessions table: %v", err)
	}
}

func TestMiddleware_AnonymousRequest(t *testing.T) {
	pool := testPool(t)
	ensureSessionsTable(t, pool)
	ws := weave.NewPostgresStore(pool)
	sm := session.NewManager(nil)
	_ = sm.SetupPgxStore(pool)

	mw := auth.NewMiddleware(sm, ws)

	var captured *auth.AuthSnapshot
	var principal *auth.Principal
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = auth.FromContext(r.Context())
		principal = auth.PrincipalFromContext(r.Context())
	}))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	sm.LoadAndSave(h).ServeHTTP(rr, req)

	if captured == nil || !captured.IsAnonymous {
		t.Errorf("anon request should produce anonymous snapshot, got %+v", captured)
	}
	if principal != nil {
		t.Errorf("anon request should not set principal, got %+v", principal)
	}
	// No session cookie present → not a lapsed session, no stale-session header.
	if got := rr.Header().Get("X-Session-Expired"); got != "" {
		t.Errorf("X-Session-Expired = %q, want empty for a request with no session cookie", got)
	}
}

// A request carrying a session cookie that resolves to no user (expired or
// deleted server-side session) must get the X-Session-Expired header so the
// frontend can prompt re-login regardless of the downstream status code.
func TestMiddleware_StaleSessionCookieSetsHeader(t *testing.T) {
	pool := testPool(t)
	ensureSessionsTable(t, pool)
	ws := weave.NewPostgresStore(pool)
	sm := session.NewManager(nil)
	_ = sm.SetupPgxStore(pool)

	mw := auth.NewMiddleware(sm, ws)
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound) // downstream may 404 (private read gate)
	}))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/projects/TES/models/TESM.3/overrides", nil)
	// A leftover cookie whose token is not in the store → scs loads an empty
	// (anonymous) session, so userID stays "" while the cookie is present.
	req.AddCookie(&http.Cookie{Name: sm.Cookie.Name, Value: "stale-token-not-in-store"})
	sm.LoadAndSave(h).ServeHTTP(rr, req)

	if got := rr.Header().Get("X-Session-Expired"); got != "1" {
		t.Errorf("X-Session-Expired = %q, want \"1\" for a stale session cookie", got)
	}
}

func TestMiddleware_AuthenticatedRequest(t *testing.T) {
	pool := testPool(t)
	ensureSessionsTable(t, pool)
	ws := weave.NewPostgresStore(pool)
	sm := session.NewManager(nil)
	_ = sm.SetupPgxStore(pool)
	ctx := context.Background()

	actorID := seedAuthTestActor(t, pool, "mw_test_1")
	// Middleware's principal-population step joins weave_auth with
	// weave_actors. Seed an auth row so GetProfileByActorID returns a row.
	if _, err := pool.Exec(ctx, `
		INSERT INTO weave_auth (actor_id, password_hash)
		VALUES ($1, $2)
	`, actorID, "test-hash"); err != nil {
		t.Fatalf("seed auth row: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM weave_auth WHERE actor_id = $1`, actorID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_memberships WHERE actor_id = $1`, actorID)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_actors WHERE id = $1`, actorID)
	})

	mw := auth.NewMiddleware(sm, ws)
	var captured *auth.AuthSnapshot
	var principal *auth.Principal
	final := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = auth.FromContext(r.Context())
		principal = auth.PrincipalFromContext(r.Context())
	}))

	// Step 1: preliminary request to set session cookie + user_id.
	rr1 := httptest.NewRecorder()
	req1 := httptest.NewRequest("GET", "/", nil)
	sm.LoadAndSave(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sm.Put(r.Context(), session.KeyUserID, actorID)
	})).ServeHTTP(rr1, req1)

	// Step 2: replay cookie, verify middleware loads snapshot.
	req2 := httptest.NewRequest("GET", "/", nil)
	for _, c := range rr1.Result().Cookies() {
		req2.AddCookie(c)
	}
	rr2 := httptest.NewRecorder()
	sm.LoadAndSave(final).ServeHTTP(rr2, req2)

	if captured == nil || captured.IsAnonymous {
		t.Fatalf("authenticated request should NOT be anonymous: %+v", captured)
	}
	if captured.ActorID != actorID {
		t.Errorf("ActorID: got %q want %q", captured.ActorID, actorID)
	}
	if principal == nil || principal.ActorID != actorID {
		t.Fatalf("principal actor_id: got %+v want %q", principal, actorID)
	}
}
