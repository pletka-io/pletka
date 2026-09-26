package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/pletka-io/pletka/pkg/weave/settings"
)

// TestNewServiceVocabularyListerNoBaseURLReturnsNilInterface locks the one
// property the whole seam exists for: an unconfigured instance must produce
// a true nil interface, not a non-nil interface wrapping a nil
// *vocabservice.Connector. A future refactor that always returns
// serviceVocabularyLister{connector: vocabservice.New(...)} — even with an
// empty base URL — would pass every other check here and still break
// settings.Host's "nil means unconfigured" contract; only this comparison
// catches it.
func TestNewServiceVocabularyListerNoBaseURLReturnsNilInterface(t *testing.T) {
	for _, baseURL := range []string{"", "   "} {
		lister := newServiceVocabularyLister(baseURL, http.DefaultClient)
		if lister != nil {
			t.Errorf("newServiceVocabularyLister(%q, ...) = %#v, want a nil interface", baseURL, lister)
		}
	}
}

// TestServiceVocabularyListerUnreachableServiceReturnsAnError covers
// configured-but-unreachable: the lister must be non-nil (a service is
// configured) but ServiceVocabularies must return an error rather than an
// empty, error-free listing — the exact confusion a curator must never see.
func TestServiceVocabularyListerUnreachableServiceReturnsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	baseURL := srv.URL
	srv.Close() // closed before use: the port is free, so the call fails fast (connection refused) instead of blocking on the connector's request timeout.

	lister := newServiceVocabularyLister(baseURL, http.DefaultClient)
	if lister == nil {
		t.Fatal("lister is nil for a configured base URL, want a non-nil lister that reports the failure")
	}
	got, err := lister.ServiceVocabularies(context.Background())
	if err == nil {
		t.Fatalf("ServiceVocabularies() = (%v, nil), want an error for an unreachable service", got)
	}
	if got != nil {
		t.Errorf("ServiceVocabularies() listing = %v, want nil alongside the error", got)
	}
}

// TestServiceVocabularyListerZeroMountsIsNotAnError covers the documented
// legal response {"version":2,"vocabs":[]}: no error, no options — and must
// read differently from the unreachable case above.
func TestServiceVocabularyListerZeroMountsIsNotAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":2,"vocabs":[]}`))
	}))
	defer srv.Close()

	lister := newServiceVocabularyLister(srv.URL, http.DefaultClient)
	if lister == nil {
		t.Fatal("lister is nil for a configured base URL")
	}
	got, err := lister.ServiceVocabularies(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("listing = %+v, want zero entries for a documented empty response", got)
	}
}

// TestServiceVocabularyListerMapsEveryField proves the field-for-field copy
// in serviceVocabularyLister.ServiceVocabularies: a rename on either side
// (e.g. dropping Languages) would pass every other test in this file but
// fail here.
func TestServiceVocabularyListerMapsEveryField(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"version": 2,
			"vocabs": []map[string]any{
				{
					"name":      "aat",
					"label":     "Art & Architecture Thesaurus",
					"concepts":  58996,
					"languages": []string{"en", "nl"},
				},
			},
		})
	}))
	defer srv.Close()

	lister := newServiceVocabularyLister(srv.URL, http.DefaultClient)
	got, err := lister.ServiceVocabularies(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("listing = %+v, want exactly one entry", got)
	}
	want := settings.ServiceVocabulary{
		Name:      "aat",
		Label:     "Art & Architecture Thesaurus",
		Concepts:  58996,
		Languages: []string{"en", "nl"},
	}
	if !reflect.DeepEqual(got[0], want) {
		t.Errorf("mapped entry = %+v, want %+v", got[0], want)
	}
}
