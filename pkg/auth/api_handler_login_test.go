package auth_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	weaveauth "github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/session"
	"github.com/pletka-io/pletka/pkg/weave"
	weaverouter "github.com/pletka-io/pletka/pkg/weave/router"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// newSSORouterForTests builds a router through the real MountAPIRoutes
// wiring (unlike weaverouter.NewForTests, which mounts routes by hand) so
// ssoEnabled flows through exactly as it does in production at app.go's
// MountAPIRoutes call site. registrationEnabled is always true here so the
// tests can still self-register fixture users; only ssoEnabled varies.
func newSSORouterForTests(t *testing.T, ssoEnabled bool) (http.Handler, *pgxpool.Pool) {
	t.Helper()
	pool := weaverouter.TestPool(t)
	// scs's pgxstore expects the sessions table to already exist.
	if _, err := pool.Exec(context.Background(), `
		CREATE TABLE IF NOT EXISTS sessions (
			token  TEXT PRIMARY KEY,
			data   BYTEA NOT NULL,
			expiry TIMESTAMPTZ NOT NULL
		);
		CREATE INDEX IF NOT EXISTS sessions_expiry_idx ON sessions (expiry);
	`); err != nil {
		t.Fatalf("ensure sessions table: %v", err)
	}
	ws := weave.NewPostgresStore(pool)
	sm := session.NewManager(nil)
	if err := sm.SetupPgxStore(pool); err != nil {
		t.Fatalf("SetupPgxStore: %v", err)
	}

	r := chi.NewRouter()
	r.Use(sm.LoadAndSave)
	r.Use(weaveauth.NewMiddleware(sm, ws))

	ah := weaveauth.NewAuthHandlerForTests(ws, sm)
	weaveauth.MountAPIRoutes(r, ah, true, ssoEnabled)
	return r, pool
}

func registerForLogin(t *testing.T, r http.Handler, name, email, username, password string) {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"name": name, "email": email, "username": username, "password": password,
	})
	req := httptest.NewRequest("POST", "/api/v1/auth/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("register setup: %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestLogin_SetsSessionForValidCredentials(t *testing.T) {
	r, pool := weaverouter.NewForTests(t)
	email := "login_ok@test.local"
	username := "login_ok"
	pw := "password_secret_1"

	// Pre/post cleanup.
	_, _ = pool.Exec(context.Background(), `DELETE FROM weave_auth WHERE actor_id IN (SELECT id FROM weave_actors WHERE email = $1 OR slug = $2)`, email, username)
	_, _ = pool.Exec(context.Background(), `DELETE FROM weave_actors WHERE email = $1 OR slug = $2`, email, username)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM weave_auth WHERE actor_id IN (SELECT id FROM weave_actors WHERE email = $1 OR slug = $2)`, email, username)
		_, _ = pool.Exec(context.Background(), `DELETE FROM weave_actors WHERE email = $1 OR slug = $2`, email, username)
	})

	registerForLogin(t, r, "Login Ok", email, username, pw)

	// Login by email.
	body, _ := json.Marshal(map[string]any{"login": email, "password": pw})
	req := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != 200 {
		t.Fatalf("login by email: %d body=%s", rr.Code, rr.Body.String())
	}
	if len(rr.Result().Cookies()) == 0 {
		t.Error("expected session cookie")
	}
}

func TestLogin_AcceptsSlugAsLoginIdentifier(t *testing.T) {
	r, pool := weaverouter.NewForTests(t)
	email := "slug_login@test.local"
	username := "slug_login"
	pw := "password_secret_1"

	_, _ = pool.Exec(context.Background(), `DELETE FROM weave_auth WHERE actor_id IN (SELECT id FROM weave_actors WHERE email = $1 OR slug = $2)`, email, username)
	_, _ = pool.Exec(context.Background(), `DELETE FROM weave_actors WHERE email = $1 OR slug = $2`, email, username)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM weave_auth WHERE actor_id IN (SELECT id FROM weave_actors WHERE email = $1 OR slug = $2)`, email, username)
		_, _ = pool.Exec(context.Background(), `DELETE FROM weave_actors WHERE email = $1 OR slug = $2`, email, username)
	})

	registerForLogin(t, r, "Slug User", email, username, pw)

	body, _ := json.Marshal(map[string]any{"login": username, "password": pw})
	req := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != 200 {
		t.Errorf("login by slug: %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestLogin_RejectsWrongPassword(t *testing.T) {
	r, pool := weaverouter.NewForTests(t)
	email := "wrongpw@test.local"
	username := "wrongpw_user"
	pw := "password_secret_1"

	_, _ = pool.Exec(context.Background(), `DELETE FROM weave_auth WHERE actor_id IN (SELECT id FROM weave_actors WHERE email = $1 OR slug = $2)`, email, username)
	_, _ = pool.Exec(context.Background(), `DELETE FROM weave_actors WHERE email = $1 OR slug = $2`, email, username)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM weave_auth WHERE actor_id IN (SELECT id FROM weave_actors WHERE email = $1 OR slug = $2)`, email, username)
		_, _ = pool.Exec(context.Background(), `DELETE FROM weave_actors WHERE email = $1 OR slug = $2`, email, username)
	})

	registerForLogin(t, r, "Wrong PW", email, username, pw)

	body, _ := json.Marshal(map[string]any{"login": email, "password": "wrong_password_here"})
	req := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != 401 {
		t.Errorf("wrong pw: %d want 401", rr.Code)
	}
}

func TestLogin_SSOEnabled_NonSuperAdminRejected(t *testing.T) {
	r, pool := newSSORouterForTests(t, true)
	email := "sso_contributor@test.local"
	username := "sso_contributor"
	pw := "password_secret_1"

	_, _ = pool.Exec(context.Background(), `DELETE FROM weave_auth WHERE actor_id IN (SELECT id FROM weave_actors WHERE email = $1 OR slug = $2)`, email, username)
	_, _ = pool.Exec(context.Background(), `DELETE FROM weave_actors WHERE email = $1 OR slug = $2`, email, username)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM weave_auth WHERE actor_id IN (SELECT id FROM weave_actors WHERE email = $1 OR slug = $2)`, email, username)
		_, _ = pool.Exec(context.Background(), `DELETE FROM weave_actors WHERE email = $1 OR slug = $2`, email, username)
	})

	// Fresh registration defaults to role "contributor" (weave_actors.role
	// default) — no explicit role assignment needed for the non-super-admin case.
	registerForLogin(t, r, "SSO Contributor", email, username, pw)

	// Capture a real wrong-password response on this same handler to compare
	// against, instead of duplicating the literal message.
	wrongBody, _ := json.Marshal(map[string]any{"login": email, "password": "definitely_not_the_password"})
	wrongReq := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewReader(wrongBody))
	wrongReq.Header.Set("Content-Type", "application/json")
	wrongRR := httptest.NewRecorder()
	r.ServeHTTP(wrongRR, wrongReq)
	if wrongRR.Code != http.StatusUnauthorized {
		t.Fatalf("setup: wrong-password capture: got %d, want 401", wrongRR.Code)
	}

	// Valid credentials, but SSO is enabled and this actor is not super_admin.
	body, _ := json.Marshal(map[string]any{"login": email, "password": pw})
	req := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != wrongRR.Code {
		t.Errorf("sso non-super-admin login status: got %d, want %d (same as wrong password)", rr.Code, wrongRR.Code)
	}
	if !bytes.Equal(rr.Body.Bytes(), wrongRR.Body.Bytes()) {
		t.Errorf("sso non-super-admin login body: got %q, want %q (same as wrong password)", rr.Body.String(), wrongRR.Body.String())
	}
	if len(rr.Result().Cookies()) != 0 {
		t.Error("non-super-admin login under sso must not receive a session cookie")
	}
}

func TestLogin_SSOEnabled_SuperAdminSucceeds(t *testing.T) {
	r, pool := newSSORouterForTests(t, true)
	email := "sso_superadmin@test.local"
	username := "sso_superadmin"
	pw := "password_secret_1"

	_, _ = pool.Exec(context.Background(), `DELETE FROM weave_auth WHERE actor_id IN (SELECT id FROM weave_actors WHERE email = $1 OR slug = $2)`, email, username)
	_, _ = pool.Exec(context.Background(), `DELETE FROM weave_actors WHERE email = $1 OR slug = $2`, email, username)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM weave_auth WHERE actor_id IN (SELECT id FROM weave_actors WHERE email = $1 OR slug = $2)`, email, username)
		_, _ = pool.Exec(context.Background(), `DELETE FROM weave_actors WHERE email = $1 OR slug = $2`, email, username)
	})

	registerForLogin(t, r, "SSO Super Admin", email, username, pw)
	if _, err := pool.Exec(context.Background(), `UPDATE weave_actors SET role = 'super_admin' WHERE email = $1`, email); err != nil {
		t.Fatalf("setup: promote to super_admin: %v", err)
	}

	body, _ := json.Marshal(map[string]any{"login": email, "password": pw})
	req := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("super-admin login under sso: %d body=%s", rr.Code, rr.Body.String())
	}
	if len(rr.Result().Cookies()) == 0 {
		t.Error("expected session cookie for super-admin login under sso")
	}
}

func TestLogin_SSODisabled_ContributorSucceeds(t *testing.T) {
	// Guard against regression: today's behavior (weaverouter.NewForTests
	// never calls SetSSOEnabled, so it defaults to false) must still let a
	// non-super-admin contributor log in with password credentials.
	r, pool := weaverouter.NewForTests(t)
	email := "sso_disabled_contributor@test.local"
	username := "sso_disabled_contributor"
	pw := "password_secret_1"

	_, _ = pool.Exec(context.Background(), `DELETE FROM weave_auth WHERE actor_id IN (SELECT id FROM weave_actors WHERE email = $1 OR slug = $2)`, email, username)
	_, _ = pool.Exec(context.Background(), `DELETE FROM weave_actors WHERE email = $1 OR slug = $2`, email, username)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM weave_auth WHERE actor_id IN (SELECT id FROM weave_actors WHERE email = $1 OR slug = $2)`, email, username)
		_, _ = pool.Exec(context.Background(), `DELETE FROM weave_actors WHERE email = $1 OR slug = $2`, email, username)
	})

	registerForLogin(t, r, "SSO Disabled Contributor", email, username, pw)

	body, _ := json.Marshal(map[string]any{"login": email, "password": pw})
	req := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("contributor login with sso disabled: %d body=%s", rr.Code, rr.Body.String())
	}
	if len(rr.Result().Cookies()) == 0 {
		t.Error("expected session cookie")
	}
}
