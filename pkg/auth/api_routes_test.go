package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

// stubAuthHandler is a no-op AuthAPIHandler implementation for route-mount tests.
type stubAuthHandler struct{}

func (stubAuthHandler) Login(w http.ResponseWriter, r *http.Request)    { w.WriteHeader(http.StatusOK) }
func (stubAuthHandler) Register(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }
func (stubAuthHandler) Logout(w http.ResponseWriter, r *http.Request)   { w.WriteHeader(http.StatusOK) }
func (stubAuthHandler) Me(w http.ResponseWriter, r *http.Request)       { w.WriteHeader(http.StatusOK) }

func TestRegisterRouteAbsentWhenDisabled(t *testing.T) {
	r := chi.NewRouter()
	MountAPIRoutes(r, stubAuthHandler{}, false)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("register with registration disabled: got %d, want 404", rec.Code)
	}

	loginReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	loginRec := httptest.NewRecorder()
	r.ServeHTTP(loginRec, loginReq)
	if loginRec.Code == http.StatusNotFound {
		t.Fatal("login should be mounted regardless of registration flag")
	}
}

func TestRegisterRoutePresentWhenEnabled(t *testing.T) {
	r := chi.NewRouter()
	MountAPIRoutes(r, stubAuthHandler{}, true)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code == http.StatusNotFound {
		t.Fatal("register should be mounted when enabled")
	}

	loginReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	loginRec := httptest.NewRecorder()
	r.ServeHTTP(loginRec, loginReq)
	if loginRec.Code == http.StatusNotFound {
		t.Fatal("login should be mounted regardless of registration flag")
	}
}
