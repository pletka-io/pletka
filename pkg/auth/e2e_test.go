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

func TestEndToEnd_RegisterLoginMeLogout(t *testing.T) {
	r, pool := weaverouter.NewForTests(t)
	email, username, pw := "e2e@test.local", "e2e_user", "e2e_password_secret"

	// Pre + post cleanup.
	ctx := context.Background()
	_, _ = pool.Exec(ctx, `DELETE FROM weave_auth WHERE actor_id IN (SELECT id FROM weave_actors WHERE email = $1 OR slug = $2)`, email, username)
	_, _ = pool.Exec(ctx, `DELETE FROM weave_actors WHERE email = $1 OR slug = $2`, email, username)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM weave_auth WHERE actor_id IN (SELECT id FROM weave_actors WHERE email = $1 OR slug = $2)`, email, username)
		_, _ = pool.Exec(context.Background(), `DELETE FROM weave_actors WHERE email = $1 OR slug = $2`, email, username)
	})

	// Register
	buf, _ := json.Marshal(map[string]any{
		"name": "End To End", "email": email, "username": username, "password": pw,
	})
	regReq := httptest.NewRequest("POST", "/api/v1/auth/register", bytes.NewReader(buf))
	regReq.Header.Set("Content-Type", "application/json")
	regRR := httptest.NewRecorder()
	r.ServeHTTP(regRR, regReq)
	if regRR.Code != http.StatusCreated {
		t.Fatalf("register: %d body=%s", regRR.Code, regRR.Body.String())
	}
	cookies := regRR.Result().Cookies()

	// Me
	meReq := httptest.NewRequest("GET", "/api/v1/auth/me", nil)
	for _, c := range cookies {
		meReq.AddCookie(c)
	}
	meRR := httptest.NewRecorder()
	r.ServeHTTP(meRR, meReq)
	if meRR.Code != http.StatusOK {
		t.Fatalf("me: %d body=%s", meRR.Code, meRR.Body.String())
	}

	// Logout
	outReq := httptest.NewRequest("POST", "/api/v1/auth/logout", nil)
	for _, c := range cookies {
		outReq.AddCookie(c)
	}
	outRR := httptest.NewRecorder()
	r.ServeHTTP(outRR, outReq)
	if outRR.Code != http.StatusNoContent {
		t.Fatalf("logout: %d", outRR.Code)
	}

	// Me after logout → 401
	meReq2 := httptest.NewRequest("GET", "/api/v1/auth/me", nil)
	for _, c := range outRR.Result().Cookies() {
		meReq2.AddCookie(c)
	}
	meRR2 := httptest.NewRecorder()
	r.ServeHTTP(meRR2, meReq2)
	if meRR2.Code != http.StatusUnauthorized {
		t.Errorf("me after logout: %d want 401", meRR2.Code)
	}
}
