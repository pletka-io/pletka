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

func registerAndLoginUser(t *testing.T, r http.Handler, email, username, password string) []*http.Cookie {
	t.Helper()
	regBody, _ := json.Marshal(map[string]any{
		"name":     username + " user",
		"email":    email,
		"username": username,
		"password": password,
	})
	regReq := httptest.NewRequest("POST", "/api/v1/auth/register", bytes.NewReader(regBody))
	regReq.Header.Set("Content-Type", "application/json")
	regRR := httptest.NewRecorder()
	r.ServeHTTP(regRR, regReq)
	if regRR.Code != http.StatusCreated {
		t.Fatalf("registerAndLogin: register %d body=%s", regRR.Code, regRR.Body.String())
	}

	loginBody, _ := json.Marshal(map[string]any{"login": email, "password": password})
	loginReq := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewReader(loginBody))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRR := httptest.NewRecorder()
	r.ServeHTTP(loginRR, loginReq)
	if loginRR.Code != http.StatusOK {
		t.Fatalf("registerAndLogin: login %d body=%s", loginRR.Code, loginRR.Body.String())
	}
	return loginRR.Result().Cookies()
}

func TestLogout_ClearsSession(t *testing.T) {
	r, pool := weaverouter.NewForTests(t)
	email, username, pw := "logout_test@test.local", "logout_test", "password_secret_1"
	ctx := context.Background()
	_, _ = pool.Exec(ctx, `DELETE FROM weave_auth WHERE actor_id IN (SELECT id FROM weave_actors WHERE email = $1 OR slug = $2)`, email, username)
	_, _ = pool.Exec(ctx, `DELETE FROM weave_actors WHERE email = $1 OR slug = $2`, email, username)
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM weave_auth WHERE actor_id IN (SELECT id FROM weave_actors WHERE email = $1 OR slug = $2)`, email, username)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_actors WHERE email = $1 OR slug = $2`, email, username)
	})

	cookies := registerAndLoginUser(t, r, email, username, pw)

	req := httptest.NewRequest("POST", "/api/v1/auth/logout", nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("logout status: %d want 204", rr.Code)
	}
}

func TestMe_ReturnsProfileForAuthenticatedUser(t *testing.T) {
	r, pool := weaverouter.NewForTests(t)
	email, username, pw := "me_test@test.local", "me_test", "password_secret_1"
	ctx := context.Background()
	_, _ = pool.Exec(ctx, `DELETE FROM weave_auth WHERE actor_id IN (SELECT id FROM weave_actors WHERE email = $1 OR slug = $2)`, email, username)
	_, _ = pool.Exec(ctx, `DELETE FROM weave_actors WHERE email = $1 OR slug = $2`, email, username)
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM weave_auth WHERE actor_id IN (SELECT id FROM weave_actors WHERE email = $1 OR slug = $2)`, email, username)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_actors WHERE email = $1 OR slug = $2`, email, username)
	})

	cookies := registerAndLoginUser(t, r, email, username, pw)

	req := httptest.NewRequest("GET", "/api/v1/auth/me", nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("me: %d body=%s", rr.Code, rr.Body.String())
	}
	var got map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &got)
	if got["email"] != email {
		t.Errorf("email: got %v want %s", got["email"], email)
	}
	if got["slug"] != username {
		t.Errorf("slug: got %v want %s", got["slug"], username)
	}
}

func TestMe_AnonymousReturns401(t *testing.T) {
	r, _ := weaverouter.NewForTests(t)
	req := httptest.NewRequest("GET", "/api/v1/auth/me", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("anon me: %d want 401", rr.Code)
	}
}
