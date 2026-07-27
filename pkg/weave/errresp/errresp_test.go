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
