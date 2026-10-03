package errortracking

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"

	weaveauth "github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/weave/errresp"
)

type chanWriter chan Event

func (c chanWriter) Insert(_ context.Context, e Event) error {
	c <- e
	return nil
}

func (c chanWriter) next(t *testing.T) Event {
	t.Helper()
	select {
	case e := <-c:
		return e
	case <-time.After(2 * time.Second):
		t.Fatal("no event recorded")
		return Event{}
	}
}

// withAuth stands in for the auth middleware, which sits inside the tracker
// and puts the snapshot on a derived request the tracker never sees.
func withAuth(actorID string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := weaveauth.WithSnapshot(r.Context(), &weaveauth.AuthSnapshot{ActorID: actorID})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// TestMiddlewareRecordsCauseActorAndPlainBody wires the tracker the way the
// app root does: compression outside it, auth inside it.
func TestMiddlewareRecordsCauseActorAndPlainBody(t *testing.T) {
	events := make(chanWriter, 1)
	r := chi.NewRouter()
	r.Use(chimiddleware.Compress(5))
	r.Use(Middleware(events, nil))
	r.Use(withAuth("actor-1"))
	r.Use(CaptureActor)
	r.Post("/boom", func(w http.ResponseWriter, r *http.Request) {
		errresp.RecordCause(r.Context(), errors.New("create ontology: duplicate key"))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"code":"internal","error":"internal error"}`))
	})

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/boom", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Header().Get("Content-Encoding") != "gzip" {
		t.Fatalf("client response should still be gzipped, headers=%v", rec.Header())
	}
	if _, err := gzip.NewReader(rec.Body); err != nil {
		t.Fatalf("client body not gzip: %v", err)
	}

	e := events.next(t)
	var errPayload map[string]string
	if err := json.Unmarshal(e.Error, &errPayload); err != nil {
		t.Fatalf("decode error payload: %v (%s)", err, e.Error)
	}
	if got := errPayload["cause"]; got != "create ontology: duplicate key" {
		t.Errorf("cause=%q, want the recorded error", got)
	}
	if got := errPayload["message"]; got != `{"code":"internal","error":"internal error"}` {
		t.Errorf("message=%q, want the uncompressed body", got)
	}
	if e.ActorID == nil || *e.ActorID != "actor-1" {
		t.Errorf("actor_id=%v, want actor-1", e.ActorID)
	}
}

func TestMiddlewareOmitsCauseWhenNoneRecorded(t *testing.T) {
	events := make(chanWriter, 1)
	h := Middleware(events, nil)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/x", nil))

	var errPayload map[string]string
	if err := json.Unmarshal(events.next(t).Error, &errPayload); err != nil {
		t.Fatal(err)
	}
	if _, ok := errPayload["cause"]; ok {
		t.Errorf("cause should be absent, got %v", errPayload)
	}
}
