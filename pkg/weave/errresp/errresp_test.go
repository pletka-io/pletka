package errresp_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pletka-io/pletka/pkg/weave/errresp"
)

func TestResponder_RoundTrip(t *testing.T) {
	called := false
	var resp errresp.Responder = func(w http.ResponseWriter, r *http.Request, status int, code, msg string) {
		called = true
		w.WriteHeader(status)
	}
	ctx := errresp.WithResponder(context.Background(), resp)
	got, ok := errresp.FromContext(ctx)
	if !ok {
		t.Fatal("responder not found in context")
	}
	w := httptest.NewRecorder()
	got(w, httptest.NewRequestWithContext(context.Background(), "GET", "/", nil).WithContext(ctx), 404, "not_found", "nope")
	if !called || w.Code != 404 {
		t.Fatalf("responder not invoked; called=%v code=%d", called, w.Code)
	}
}

func TestFromContext_Absent(t *testing.T) {
	if _, ok := errresp.FromContext(context.Background()); ok {
		t.Error("expected no responder in a bare context")
	}
}

func TestError_UsesStashedResponder(t *testing.T) {
	var gotStatus int
	var gotCode string
	resp := errresp.Responder(func(w http.ResponseWriter, r *http.Request, status int, code, msg string) {
		gotStatus, gotCode = status, code
		w.WriteHeader(status)
	})
	ctx := errresp.WithResponder(context.Background(), resp)
	w := httptest.NewRecorder()
	errresp.Error(w, httptest.NewRequestWithContext(ctx, "GET", "/x", nil), 404, "not_found", "nope")
	if gotStatus != 404 || gotCode != "not_found" || w.Code != 404 {
		t.Fatalf("responder not invoked correctly: status=%d code=%q w=%d", gotStatus, gotCode, w.Code)
	}
}

func TestError_FallsBackWhenNoResponder(t *testing.T) {
	w := httptest.NewRecorder()
	errresp.Error(w, httptest.NewRequestWithContext(context.Background(), "GET", "/x", nil), 404, "not_found", "nope")
	if w.Code != 404 {
		t.Fatalf("fallback should still 404, got %d", w.Code)
	}
}

func TestHolder_StashMiddlewareInjectsWhenSet(t *testing.T) {
	h := &errresp.Holder{}
	mw := errresp.StashMiddleware(h)
	var got errresp.Responder
	var ok bool
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { got, ok = errresp.FromContext(r.Context()) })

	// Before Set: no responder on context.
	mw(next).ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(context.Background(), "GET", "/", nil))
	if ok {
		t.Fatal("responder present before Holder.Set")
	}

	// After Set: present.
	var called bool
	h.Set(func(http.ResponseWriter, *http.Request, int, string, string) { called = true })
	mw(next).ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(context.Background(), "GET", "/", nil))
	if !ok || got == nil {
		t.Fatal("responder absent after Holder.Set")
	}
	got(httptest.NewRecorder(), httptest.NewRequestWithContext(context.Background(), "GET", "/", nil), 404, "not_found", "x")
	if !called {
		t.Fatal("stashed responder not the one Set")
	}
}
