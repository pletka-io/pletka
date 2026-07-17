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

func TestRegister_CreatesActorAndAuth(t *testing.T) {
	r, pool := weaverouter.NewForTests(t)
	ctx := context.Background()
	email := "newuser@test.local"
	username := "newuser"

	// Pre-cleanup in case a prior run left rows behind.
	_, _ = pool.Exec(ctx, `DELETE FROM weave_auth WHERE actor_id IN (SELECT id FROM weave_actors WHERE email = $1 OR slug = $2)`, email, username)
	_, _ = pool.Exec(ctx, `DELETE FROM weave_actors WHERE email = $1 OR slug = $2`, email, username)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM weave_auth WHERE actor_id IN (SELECT id FROM weave_actors WHERE email = $1 OR slug = $2)`, email, username)
		_, _ = pool.Exec(context.Background(), `DELETE FROM weave_actors WHERE email = $1 OR slug = $2`, email, username)
	})

	body := map[string]any{
		"name":     "New User",
		"email":    email,
		"username": username,
		"password": "hunter2_password",
	}
	buf, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/api/v1/auth/register", bytes.NewReader(buf))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status: got %d want 201; body=%s", rr.Code, rr.Body.String())
	}

	var count int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM weave_auth wa JOIN weave_actors a ON a.id = wa.actor_id WHERE a.email = $1`,
		email,
	).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("expected 1 joined row, got %d", count)
	}
}
