package app

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRecoverMiddleware_BrandedHTML(t *testing.T) {
	rendered := false
	mw := recoverMiddleware(func(w http.ResponseWriter, r *http.Request) {
		rendered = true
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("<html>branded 500</html>"))
	}, slog.New(slog.DiscardHandler))
	h := mw(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") }))

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/x", nil))
	if w.Code != http.StatusInternalServerError || !rendered {
		t.Fatalf("want branded 500 rendered; code=%d rendered=%v", w.Code, rendered)
	}
}

func TestRecoverMiddleware_NilRendererFallsBackToPlain500(t *testing.T) {
	mw := recoverMiddleware(nil, slog.New(slog.DiscardHandler))
	h := mw(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") }))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/x", nil))
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("nil renderer should still 500, got %d", w.Code)
	}
}
