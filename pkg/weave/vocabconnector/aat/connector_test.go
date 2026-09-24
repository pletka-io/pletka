package aat

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/pletka-io/pletka/pkg/weave/vocabconnector"
)

func TestConnectorSearchParsesSPARQLResults(t *testing.T) {
	var requestedQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedQuery = r.URL.Query().Get("query")
		w.Header().Set("Content-Type", "application/sparql-results+json")
		_, _ = w.Write([]byte(`{
			"head": {"vars": ["subject", "label", "scopeNote", "broader", "broaderLabel", "parentString"]},
			"results": {"bindings": [{
				"subject": {"type": "uri", "value": "http://vocab.getty.edu/page/aat/300010358"},
				"label": {"type": "literal", "xml:lang": "en", "value": "bronze"},
				"scopeNote": {"type": "literal", "xml:lang": "en", "value": "Copper alloy."},
				"broader": {"type": "uri", "value": "http://vocab.getty.edu/aat/300010357"},
				"broaderLabel": {"type": "literal", "xml:lang": "en", "value": "copper alloy"},
				"parentString": {"type": "literal", "xml:lang": "en", "value": "copper alloy, metal, materials"}
			}]}
		}`))
	}))
	defer server.Close()

	connector := New(Config{EndpointURL: server.URL}, server.Client())
	entries, err := connector.Search(context.Background(), "bronze", vocabconnector.SearchOpts{
		Lang:      "en",
		Limit:     10,
		ParentURI: "http://vocab.getty.edu/aat/300010357",
	})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].URI != "https://vocab.getty.edu/aat/300010358" {
		t.Fatalf("unexpected normalized URI: %s", entries[0].URI)
	}
	if entries[0].Label["en"] != "bronze" {
		t.Fatalf("unexpected label: %#v", entries[0].Label)
	}
	if entries[0].ScopeNote["en"] != "Copper alloy." {
		t.Fatalf("unexpected scope note: %#v", entries[0].ScopeNote)
	}
	if entries[0].ExternalID != "300010358" {
		t.Fatalf("unexpected external ID: %s", entries[0].ExternalID)
	}
	if strings.Join(entries[0].BroaderPath, " > ") != "copper alloy > metal > materials" {
		t.Fatalf("unexpected broader path: %#v", entries[0].BroaderPath)
	}
	if len(entries[0].BroaderPathItems) != 3 {
		t.Fatalf("unexpected broader path refs: %#v", entries[0].BroaderPathItems)
	}
	if entries[0].BroaderPathItems[0].URI != "https://vocab.getty.edu/aat/300010357" {
		t.Fatalf("expected first broader path ref to be hydrated with URI, got %#v", entries[0].BroaderPathItems[0])
	}
	if entries[0].BroaderPathItems[0].Label["en"] != "copper alloy" {
		t.Fatalf("unexpected first broader path label: %#v", entries[0].BroaderPathItems[0].Label)
	}
	if !strings.Contains(requestedQuery, "bronze") {
		t.Fatalf("expected query to contain search term, got %s", requestedQuery)
	}
	if !strings.Contains(requestedQuery, "gvp:parentString") {
		t.Fatalf("expected query to request parentString, got %s", requestedQuery)
	}
	if !strings.Contains(requestedQuery, "gvp:broaderPreferredExtended <http://vocab.getty.edu/aat/300010357>") {
		t.Fatalf("expected query to filter by parent URI, got %s", requestedQuery)
	}
}

func TestConnectorFetchParsesSingleResult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/sparql-results+json")
		_, _ = w.Write([]byte(`{
			"results": {"bindings": [{
				"subject": {"type": "uri", "value": "https://vocab.getty.edu/aat/300010358"},
				"label": {"type": "literal", "xml:lang": "en", "value": "bronze"}
			}]}
		}`))
	}))
	defer server.Close()

	connector := New(Config{EndpointURL: server.URL}, server.Client())
	entry, err := connector.Fetch(context.Background(), "http://vocab.getty.edu/page/aat/300010358", vocabconnector.SearchOpts{Lang: "en"})
	if err != nil {
		t.Fatalf("Fetch returned error: %v", err)
	}
	if entry == nil {
		t.Fatal("expected entry")
	}
	if entry.URI != "https://vocab.getty.edu/aat/300010358" {
		t.Fatalf("unexpected URI: %s", entry.URI)
	}
}

// TestConnectorSearchDegradesOnSlowEndpoint proves Search never blocks or
// errors the caller when the remote authority hangs: a 5s-sleeping endpoint
// against a 100ms timeout returns empty results and a nil error well within
// the sleep window (#3599).
func TestConnectorSearchDegradesOnSlowEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(5 * time.Second):
		case <-r.Context().Done(): // client gave up; unblock server.Close()
		}
	}))
	defer server.Close()

	connector := New(Config{EndpointURL: server.URL, Timeout: 100 * time.Millisecond}, server.Client())

	done := make(chan struct{})
	var entries []vocabconnector.Entry
	var err error
	go func() {
		entries, err = connector.Search(context.Background(), "bronze", vocabconnector.SearchOpts{Lang: "en"})
		close(done)
	}()

	select {
	case <-done:
		if err != nil {
			t.Fatalf("Search should degrade to nil error, got: %v", err)
		}
		if len(entries) != 0 {
			t.Fatalf("Search should return no entries on timeout, got %d", len(entries))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Search blocked past its timeout")
	}
}

// TestConnectorFetchErrorsOnSlowEndpoint proves Fetch (explicit by-URI resolve)
// still surfaces the timeout as an error, unlike Search.
func TestConnectorFetchErrorsOnSlowEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(5 * time.Second):
		case <-r.Context().Done(): // client gave up; unblock server.Close()
		}
	}))
	defer server.Close()

	connector := New(Config{EndpointURL: server.URL, Timeout: 100 * time.Millisecond}, server.Client())
	if _, err := connector.Fetch(context.Background(), "http://vocab.getty.edu/aat/300010358", vocabconnector.SearchOpts{Lang: "en"}); err == nil {
		t.Fatal("Fetch should return an error on timeout")
	}
}
