package auth_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	weaverouter "github.com/pletka-io/pletka/pkg/weave/router"
)

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
