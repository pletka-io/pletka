package observability

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	dto "github.com/prometheus/client_model/go"
)

func TestMiddleware_RecordsRouteAndStatus(t *testing.T) {
	rt := New(Config{}, nil, nil) // no listener, no pool

	r := chi.NewRouter()
	r.Use(rt.Middleware)
	r.Get("/projects/{id}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	srv := httptest.NewServer(r)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/projects/AME")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()

	// The counter must use the chi route PATTERN, not the raw /projects/AME.
	got := counterValue(t, rt, map[string]string{
		"method": "GET", "route": "/projects/{id}", "status": "204", "status_class": "2xx",
	})
	if got != 1 {
		t.Fatalf("pletka_http_requests_total = %v, want 1 (route pattern + 204/2xx)", got)
	}
}

func TestMiddleware_UnknownRouteBounded(t *testing.T) {
	rt := New(Config{}, nil, nil)

	r := chi.NewRouter()
	r.Use(rt.Middleware)
	// A real router always has routes; register one so chi builds the
	// middleware chain, then hit an unmatched path -> 404, pattern "unknown".
	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {})

	srv := httptest.NewServer(r)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/nope/" + "deadbeef")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()

	got := counterValue(t, rt, map[string]string{
		"method": "GET", "route": "unknown", "status": "404", "status_class": "4xx",
	})
	if got != 1 {
		t.Fatalf("unknown-route counter = %v, want 1 (bounded label, not raw URL)", got)
	}
}

// counterValue gathers the registry and returns pletka_http_requests_total for
// the exact label set, or 0 if absent.
func counterValue(t *testing.T, rt *Runtime, labels map[string]string) float64 {
	t.Helper()
	mfs, err := rt.reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	for _, mf := range mfs {
		if mf.GetName() != "pletka_http_requests_total" {
			continue
		}
		for _, m := range mf.GetMetric() {
			if labelsMatch(m.GetLabel(), labels) {
				return m.GetCounter().GetValue()
			}
		}
	}
	return 0
}

func labelsMatch(pairs []*dto.LabelPair, want map[string]string) bool {
	if len(pairs) != len(want) {
		return false
	}
	for _, p := range pairs {
		if want[p.GetName()] != p.GetValue() {
			return false
		}
	}
	return true
}
