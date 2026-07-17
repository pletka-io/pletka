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
