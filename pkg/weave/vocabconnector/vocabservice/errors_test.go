package vocabservice

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pletka-io/pletka/pkg/weave/vocabconnector"
)

// TestServiceErrorCarriesItsToken covers the v2 error envelope end to end: a
// 404 unknown_vocabulary body, decoded off the connector's own get, must
// surface as a *ServiceError an errors.As can reach, carrying the token and
// the vocab name the service sent.
func TestServiceErrorCarriesItsToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"unknown_vocabulary","message":"No vocabulary is mounted under that name on this server.","vocab":"not-a-mount"}`))
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, Vocab: "not-a-mount"}, srv.Client())
	_, err := c.Fetch(context.Background(), "https://vocab.getty.edu/not-a-mount/1", vocabconnector.SearchOpts{})
	if err == nil {
		t.Fatal("Fetch returned no error for a 404 unknown_vocabulary body")
	}
	var svcErr *ServiceError
	if !errors.As(err, &svcErr) {
		t.Fatalf("errors.As found no *ServiceError in %v", err)
	}
	if svcErr.Token != errCodeUnknownVocabulary {
		t.Errorf("Token = %q, want %s", svcErr.Token, errCodeUnknownVocabulary)
	}
	if svcErr.Vocab != "not-a-mount" {
		t.Errorf("Vocab = %q, want not-a-mount", svcErr.Vocab)
	}
	if svcErr.Status != http.StatusNotFound {
		t.Errorf("Status = %d, want 404", svcErr.Status)
	}
}

// TestServiceErrorStatusIsFromTransportNotBody pins the fix-round-1 bug: a
// body that happens to carry its own "status" key must not overwrite
// ServiceError.Status, which is authoritative precisely because it comes
// from resp.StatusCode, not from the peer. No documented v2 token's body
// carries a "status" key today, but Status has no json tag protecting it,
// and encoding/json matches an untagged exported field by name
// case-insensitively — so a future token, or a misbehaving proxy, could
// otherwise silently relabel a 404 as whatever the body claims.
func TestServiceErrorStatusIsFromTransportNotBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"unknown_vocabulary","message":"m","vocab":"not-a-mount","status":999}`))
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, Vocab: "not-a-mount"}, srv.Client())
	_, err := c.Fetch(context.Background(), "https://vocab.getty.edu/not-a-mount/1", vocabconnector.SearchOpts{})
	if err == nil {
		t.Fatal("Fetch returned no error")
	}
	var svcErr *ServiceError
	if !errors.As(err, &svcErr) {
		t.Fatalf("errors.As found no *ServiceError in %v", err)
	}
	if svcErr.Status != http.StatusNotFound {
		t.Errorf("Status = %d, want %d (the real HTTP status, not the body's own \"status\" key)", svcErr.Status, http.StatusNotFound)
	}
}

// TestUnknownVocabularyIsPermanent asserts the ErrMisconfigured classification
// itself: unknown_vocabulary and bad_lang (a malformed language tag, since
// the service now falls back on a merely unindexed one) are permanent rows
// that will not fix themselves. vocab_unavailable is deliberately NOT
// misconfiguration — the mount exists but could not be read, which is the
// service's problem and may well fix itself.
func TestUnknownVocabularyIsPermanent(t *testing.T) {
	for _, tc := range []struct {
		name          string
		status        int
		body          string
		wantPermanent bool
	}{
		{errCodeUnknownVocabulary, http.StatusNotFound, `{"error":"unknown_vocabulary","message":"No vocabulary is mounted under that name on this server.","vocab":"not-a-mount"}`, true},
		{"bad_lang", http.StatusBadRequest, `{"error":"bad_lang","message":"lang is not a usable language tag (letters, digits, '-' and '_' only)."}`, true},
		{"vocab_unavailable", http.StatusInternalServerError, `{"error":"vocab_unavailable","message":"the mount could not be read."}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			c := New(Config{BaseURL: srv.URL, Vocab: "aat"}, srv.Client())
			_, err := c.Fetch(context.Background(), "https://vocab.getty.edu/aat/1", vocabconnector.SearchOpts{})
			if err == nil {
				t.Fatal("Fetch returned no error")
			}
			if got := errors.Is(err, ErrMisconfigured); got != tc.wantPermanent {
				t.Errorf("errors.Is(err, ErrMisconfigured) = %v, want %v (err: %v)", got, tc.wantPermanent, err)
			}
		})
	}
}

// TestSearchStillDegradesOnAPermanentError is the policy this task must not
// change: Search always returns ErrDegraded, keystroke autocomplete never
// errors. What's new is that the SAME error also satisfies
// errors.Is(err, ErrMisconfigured) for a permanent fault, so a throttled
// server log can say the row is misconfigured while the curator still sees
// an ordinary empty result.
func TestSearchStillDegradesOnAPermanentError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"unknown_vocabulary","message":"No vocabulary is mounted under that name on this server.","vocab":"not-a-mount"}`))
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, Vocab: "not-a-mount"}, srv.Client())
	entries, err := c.Search(context.Background(), "brons", vocabconnector.SearchOpts{})
	if len(entries) != 0 {
		t.Errorf("got %d entries, want none", len(entries))
	}
	if !errors.Is(err, vocabconnector.ErrDegraded) {
		t.Fatalf("err = %v, want ErrDegraded (the policy: autocomplete never errors on a keystroke)", err)
	}
	if !errors.Is(err, ErrMisconfigured) {
		t.Fatalf("err = %v, want it also ErrMisconfigured: one error must carry both facts", err)
	}
}

// TestANonJSONErrorBodyStillFails covers a non-2xx body that is not the v2
// envelope at all — HTML from a proxy or a captive portal, which is not
// guaranteed small either. There is no token to read, but the call must
// still fail, and the error must name the status: never a silent success.
func TestANonJSONErrorBodyStillFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("<html><body>502 Bad Gateway</body></html>"))
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, Vocab: "aat"}, srv.Client())
	_, err := c.Fetch(context.Background(), "https://vocab.getty.edu/aat/1", vocabconnector.SearchOpts{})
	if err == nil {
		t.Fatal("Fetch returned no error for a non-JSON error body")
	}
	var svcErr *ServiceError
	if errors.As(err, &svcErr) {
		t.Fatalf("errors.As found a *ServiceError %+v in a non-JSON body", svcErr)
	}
	if !strings.Contains(err.Error(), "502") {
		t.Errorf("err = %v, want it to name the status 502", err)
	}
}
