package apikey

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/domain"
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

func TestRowFromKeyStatusPrecedence(t *testing.T) {
	now := time.Now()
	past24h := now.Add(-24 * time.Hour)
	past48h := now.Add(-48 * time.Hour)

	tests := []struct {
		name          string
		key           *domain.APIKey
		wantStatus    string
		wantRevokedAt string // "empty" or "present"
	}{
		{
			name: "active key",
			key: &domain.APIKey{
				ID:        "key1",
				Name:      "test",
				KeyPrefix: "pk_abc",
				CreatedAt: now,
				RevokedAt: nil,
				ExpiresAt: nil,
			},
			wantStatus:    "active",
			wantRevokedAt: "empty",
		},
		{
			name: "expired key (past expiry, not revoked)",
			key: &domain.APIKey{
				ID:        "key2",
				Name:      "test",
				KeyPrefix: "pk_abc",
				CreatedAt: past48h,
				RevokedAt: nil,
				ExpiresAt: &past24h,
			},
			wantStatus:    "expired",
			wantRevokedAt: "empty",
		},
		{
			name: "revoked key (also past expiry, revoked wins)",
			key: &domain.APIKey{
				ID:        "key3",
				Name:      "test",
				KeyPrefix: "pk_abc",
				CreatedAt: past48h,
				RevokedAt: &now,
				ExpiresAt: &past24h,
			},
			wantStatus:    "revoked",
			wantRevokedAt: "present",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			row := rowFromKey(tt.key)
			if row.Status != tt.wantStatus {
				t.Errorf("status = %q, want %q", row.Status, tt.wantStatus)
			}
			if tt.wantRevokedAt == "empty" && row.RevokedAt != "" {
				t.Errorf("RevokedAt should be empty, got %q", row.RevokedAt)
			}
			if tt.wantRevokedAt == "present" && row.RevokedAt == "" {
				t.Error("RevokedAt should be present, got empty")
			}
		})
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

	// After revoke: owner's list should show status="revoked" and non-empty revoked_at
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, asActor(httptest.NewRequest("GET", "/me/api-keys", nil), "actorA"))
	if rec.Code != http.StatusOK {
		t.Fatalf("list after revoke = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"status":"revoked"`) {
		t.Fatalf("list after revoke missing status=revoked: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"revoked_at":"`) {
		t.Fatalf("list after revoke missing non-empty revoked_at: %s", rec.Body.String())
	}

	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, asActor(httptest.NewRequest("POST", "/me/api-keys/"+id+"/revoke", nil), "actorA"))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("double revoke = %d, want 404", rec.Code)
	}
}

func TestCreateValidation(t *testing.T) {
	r := testRouter(newFakeStore())

	// Test 1: blank name
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

	// Test 2: non-numeric expires_days
	req = asActor(httptest.NewRequest("POST", "/me/api-keys",
		strings.NewReader(`{"name":"x","expires_days":"abc"}`)), "actorA")
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expires_days abc = %d, want 422", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"expires_days"`) {
		t.Fatalf("422 must have expires_days error: %s", rec.Body.String())
	}

	// Test 3: negative expires_days
	req = asActor(httptest.NewRequest("POST", "/me/api-keys",
		strings.NewReader(`{"name":"x","expires_days":"-3"}`)), "actorA")
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expires_days -3 = %d, want 422", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"expires_days"`) {
		t.Fatalf("422 must have expires_days error: %s", rec.Body.String())
	}
}
