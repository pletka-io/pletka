package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/pkg/session"
	"github.com/pletka-io/pletka/pkg/weave/errresp"
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

// TestSetupGlobalMiddlewareStashesErrResponderAfterHolderSet is the boot/
// wiring proof for the global error responder (a full app.New() boot test
// needs a live DB pool plus ~20 slice host fakes, which is impractical here —
// see the app.go/root.go wiring instead). It exercises the real
// setupGlobalMiddleware chain the same way csrf_wiring_test.go does, and
// proves the two-step dance app.go performs actually works end to end:
// errresp.StashMiddleware is installed on the mux before any route exists
// (buildRootMux calls setupGlobalMiddleware before route registration, so
// this never panics chi's "middleware after routes" guard), and once
// Holder.Set is called — mirroring the Set app.go issues right after
// weaverouter.Mount builds the concrete responder from the error-page host —
// the same already-built router carries that exact responder on the request
// context for every subsequent request.
func TestSetupGlobalMiddlewareStashesErrResponderAfterHolderSet(t *testing.T) {
	r := chi.NewRouter()
	holder := &errresp.Holder{}
	setupGlobalMiddleware(r, rootDependencies{
		Session:      session.New(session.DefaultConfig()),
		ErrResponder: holder,
	})

	var gotOK bool
	var gotResp errresp.Responder
	r.Get("/probe", func(w http.ResponseWriter, req *http.Request) {
		gotResp, gotOK = errresp.FromContext(req.Context())
		w.WriteHeader(http.StatusOK)
	})

	probe := func() {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/probe", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
	}

	probe()
	if gotOK {
		t.Fatal("errresp.FromContext ok = true before Holder.Set, want false")
	}

	var responderCalled bool
	holder.Set(func(http.ResponseWriter, *http.Request, int, string, string) { responderCalled = true })

	probe()
	if !gotOK {
		t.Fatal("errresp.FromContext ok = false after Holder.Set, want true")
	}
	gotResp(httptest.NewRecorder(), httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/probe", nil), 404, "not_found", "x")
	if !responderCalled {
		t.Fatal("stashed responder is not the one Holder.Set installed")
	}
}
