package apikey

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/pkg/auth"
)

func testRouter(store Store) chi.Router {
	r := chi.NewRouter()
	Mount(r, Host{
		Service:      NewService(store, slog.Default()),
		Logger:       slog.Default(),
		LangResolver: func(*http.Request) string { return "en" },
	})
	return r
}

func asActor(req *http.Request, actorID string) *http.Request {
	snap := &auth.AuthSnapshot{ActorID: actorID}
	return req.WithContext(auth.WithSnapshot(req.Context(), snap))
}

func TestAnonymous401(t *testing.T) {
	r := testRouter(newFakeStore())
	for _, tc := range []struct{ method, path string }{
		{"GET", "/me/api-keys"},
		{"GET", "/me/api-keys/list-schema"},
		{"GET", "/me/api-keys/form-schema"},
		{"POST", "/me/api-keys"},
		{"POST", "/me/api-keys/xyz/revoke"},
	} {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader("{}"))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s = %d, want 401", tc.method, tc.path, rec.Code)
		}
	}
}

func TestCreateListRevokeFlow(t *testing.T) {
	store := newFakeStore()
	r := testRouter(store)

	// create
	req := asActor(httptest.NewRequest("POST", "/me/api-keys",
		strings.NewReader(`{"name":"laptop","expires_days":""}`)), "actorA")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d body=%s", rec.Code, rec.Body.String())
	}
	var created map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	secret, _ := created["secret"].(string)
	if !strings.HasPrefix(secret, "pk_") {
		t.Fatalf("secret missing/wrong: %v", created["secret"])
	}
	id, _ := created["id"].(string)

	// list: owner sees it, other actor does not
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, asActor(httptest.NewRequest("GET", "/me/api-keys", nil), "actorA"))
	if !strings.Contains(rec.Body.String(), `"laptop"`) {
		t.Fatalf("owner list missing key: %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "secret") {
		t.Fatal("list must never contain the secret")
	}
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, asActor(httptest.NewRequest("GET", "/me/api-keys", nil), "actorB"))
	if strings.Contains(rec.Body.String(), `"laptop"`) {
		t.Fatal("cross-actor list leak")
	}

	// revoke: foreign actor 404, owner 204, second revoke 404
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, asActor(httptest.NewRequest("POST", "/me/api-keys/"+id+"/revoke", nil), "actorB"))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("foreign revoke = %d, want 404", rec.Code)
	}
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, asActor(httptest.NewRequest("POST", "/me/api-keys/"+id+"/revoke", nil), "actorA"))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("owner revoke = %d, want 204", rec.Code)
	}
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, asActor(httptest.NewRequest("POST", "/me/api-keys/"+id+"/revoke", nil), "actorA"))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("double revoke = %d, want 404", rec.Code)
	}
}

func TestCreateValidation(t *testing.T) {
	r := testRouter(newFakeStore())
	req := asActor(httptest.NewRequest("POST", "/me/api-keys",
		strings.NewReader(`{"name":"   "}`)), "actorA")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("blank name = %d, want 422", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"name"`) {
		t.Fatalf("422 must carry per-field errors: %s", rec.Body.String())
	}
}
