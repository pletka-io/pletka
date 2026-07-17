package app

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/go-chi/chi/v5"
)

func TestMountStaticAssetsMergesContributedFilesystems(t *testing.T) {
	r := chi.NewRouter()
	mountStaticAssets(r, rootDependencies{
		StaticAssets: []StaticAssetSet{
			{
				ID: "platform",
				FS: fstest.MapFS{
					"app.css":      {Data: []byte("platform")},
					"platform.css": {Data: []byte("platform-only")},
				},
			},
			{
				ID: "core-extension",
				FS: fstest.MapFS{
					"app.css":  {Data: []byte("base")},
					"extra.js": {Data: []byte("extra")},
				},
			},
		},
	})

	tests := []struct {
		path string
		want string
	}{
		{path: "/static/app.css", want: "platform"},
		{path: "/static/platform.css", want: "platform-only"},
		{path: "/static/extra.js", want: "extra"},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rr := httptest.NewRecorder()

			r.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200; body=%q", rr.Code, rr.Body.String())
			}
			if rr.Body.String() != tt.want {
				t.Fatalf("body = %q, want %q", rr.Body.String(), tt.want)
			}
		})
	}
}

func TestMountStaticAssetsFallsBackToCoreEmbeddedAssets(t *testing.T) {
	r := chi.NewRouter()
	mountStaticAssets(r, rootDependencies{})

	req := httptest.NewRequest(http.MethodGet, "/static/img/pletka-logo.svg", nil)
	rr := httptest.NewRecorder()

	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if rr.Body.Len() == 0 {
		t.Fatal("embedded asset body is empty")
	}
}

func TestCorsHeadersUsesExplicitOrigins(t *testing.T) {
	handler := corsHeaders([]string{"https://example.test"}, false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "https://example.test")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if got, want := rr.Header().Get("Access-Control-Allow-Origin"), "https://example.test"; got != want {
		t.Fatalf("Access-Control-Allow-Origin = %q, want %q", got, want)
	}
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusNoContent)
	}
}

func TestCorsHeadersDefaultsByDevMode(t *testing.T) {
	tests := []struct {
		name   string
		dev    bool
		origin string
		want   string
	}{
		{name: "development localhost", dev: true, origin: "http://localhost:5173", want: "http://localhost:5173"},
		{name: "production pletka", dev: false, origin: "https://pletka.io", want: "https://pletka.io"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := corsHeaders(nil, tt.dev)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			}))
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set("Origin", tt.origin)
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			if got := rr.Header().Get("Access-Control-Allow-Origin"); got != tt.want {
				t.Fatalf("Access-Control-Allow-Origin = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDevModeMiddlewareStoresExplicitMode(t *testing.T) {
	handler := devModeMiddleware(true)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isDevMode(r.Context()) {
			t.Fatal("isDevMode(ctx) = false, want true")
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusNoContent)
	}
}
