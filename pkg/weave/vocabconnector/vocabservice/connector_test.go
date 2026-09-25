package vocabservice

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/pletka-io/pletka/pkg/weave/vocabconnector"
)

func TestSearchBuildsTheRequestAndMapsAHit(t *testing.T) {
	var gotPath, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery = r.URL.Path, r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[
			{"uri":"http://vocab.getty.edu/aat/300010957","id":"300010957","prefLabel":"brons","lang":"nl",
			 "parentString":"koperlegering, metaal","broader":"http://vocab.getty.edu/aat/300010942"},
			{"uri":"http://vocab.getty.edu/aat/300311355","id":"300311355","prefLabel":"brons (kleur)","lang":"nl",
			 "parentString":null,"broader":null}]}`))
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, Vocab: "aat"}, srv.Client())
	entries, err := c.Search(context.Background(), "brons", vocabconnector.SearchOpts{
		Lang: "nl", Limit: 20, ParentURI: "https://vocab.getty.edu/aat/300264091",
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if gotPath != "/vocab/aat/suggest" {
		t.Errorf("path = %q, want /vocab/aat/suggest", gotPath)
	}
	// under= carries only the identifier, not the whole URI.
	for _, want := range []string{"q=brons", "lang=nl", "limit=20", "under=300264091"} {
		if !contains(gotQuery, want) {
			t.Errorf("query %q is missing %q", gotQuery, want)
		}
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}
	if entries[0].URI != "https://vocab.getty.edu/aat/300010957" {
		t.Errorf("URI = %q, want the https form", entries[0].URI)
	}
	if entries[0].Label["nl"] != "brons" {
		t.Errorf("Label[nl] = %q, want brons", entries[0].Label["nl"])
	}
	if entries[0].BroaderURI != "https://vocab.getty.edu/aat/300010942" {
		t.Errorf("BroaderURI = %q", entries[0].BroaderURI)
	}
	if diff := cmp.Diff([]string{"koperlegering", "metaal"}, entries[0].BroaderPath); diff != "" {
		t.Errorf("BroaderPath mismatch (-want +got):\n%s", diff)
	}
	// The nearest ancestor is the broader concept, so item 0 gets its identity.
	if entries[0].BroaderPathItems[0].URI != entries[0].BroaderURI {
		t.Errorf("BroaderPathItems[0].URI = %q, want %q", entries[0].BroaderPathItems[0].URI, entries[0].BroaderURI)
	}
	if entries[0].BroaderPathItems[0].ID != "300010942" {
		t.Errorf("BroaderPathItems[0].ID = %q, want 300010942", entries[0].BroaderPathItems[0].ID)
	}
	if entries[0].ExternalID != "300010957" {
		t.Errorf("ExternalID = %q", entries[0].ExternalID)
	}
	if entries[1].BroaderPath != nil || entries[1].BroaderURI != "" {
		t.Error("a hit with null parentString and null broader must map to no path and no broader URI")
	}
}

func TestSearchOmitsUnderWhenNoParentGiven(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_, _ = w.Write([]byte(`{"results":[]}`))
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, Vocab: "aat"}, srv.Client())
	if _, err := c.Search(context.Background(), "brons", vocabconnector.SearchOpts{Lang: "nl"}); err != nil {
		t.Fatalf("Search: %v", err)
	}
	if contains(gotQuery, "under=") {
		t.Errorf("query %q carries under= with no parent URI", gotQuery)
	}
}

func TestSearchReturnsErrDegradedOnServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, Vocab: "aat"}, srv.Client())
	entries, err := c.Search(context.Background(), "brons", vocabconnector.SearchOpts{})
	if len(entries) != 0 {
		t.Errorf("got %d entries, want none", len(entries))
	}
	if !errors.Is(err, vocabconnector.ErrDegraded) {
		t.Fatalf("err = %v, want ErrDegraded", err)
	}
}

func TestSearchReturnsErrDegradedOnTimeoutAndReturnsPromptly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(400 * time.Millisecond)
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, Vocab: "aat", Timeout: 50 * time.Millisecond}, srv.Client())
	start := time.Now()
	entries, err := c.Search(context.Background(), "brons", vocabconnector.SearchOpts{})
	elapsed := time.Since(start)
	if len(entries) != 0 {
		t.Errorf("got %d entries, want none", len(entries))
	}
	if !errors.Is(err, vocabconnector.ErrDegraded) {
		t.Fatalf("err = %v, want ErrDegraded", err)
	}
	if elapsed > 300*time.Millisecond {
		t.Errorf("Search took %s; the per-row timeout did not bound it", elapsed)
	}
}

func TestSearchOnBlankQueryAsksNothing(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		_, _ = w.Write([]byte(`{"results":[]}`))
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, Vocab: "aat"}, srv.Client())
	entries, err := c.Search(context.Background(), "   ", vocabconnector.SearchOpts{})
	if err != nil || entries != nil {
		t.Fatalf("Search(blank) = %v, %v; want nil, nil", entries, err)
	}
	if called {
		t.Error("a blank query must not reach the service")
	}
}

func TestFetchReadsOneConcept(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`{"uri":"http://vocab.getty.edu/aat/300010957","id":"300010957",
			"prefLabel":"bronze (metal)","lang":"en","parentString":"copper alloy, metal",
			"broader":"http://vocab.getty.edu/aat/300010942"}`))
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, Vocab: "aat"}, srv.Client())
	entry, err := c.Fetch(context.Background(), "https://vocab.getty.edu/aat/300010957", vocabconnector.SearchOpts{Lang: "en"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if gotPath != "/vocab/aat/concept/300010957" {
		t.Errorf("path = %q, want /vocab/aat/concept/300010957", gotPath)
	}
	if entry.Label["en"] != "bronze (metal)" {
		t.Errorf("Label[en] = %q", entry.Label["en"])
	}
}

// Fetch is an explicit lookup of a known URI, not a keystroke, so its failure
// is an error rather than a degraded empty.
func TestFetchPropagatesTheServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"boom"}`))
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, Vocab: "aat"}, srv.Client())
	_, err := c.Fetch(context.Background(), "https://vocab.getty.edu/aat/300010957", vocabconnector.SearchOpts{})
	if err == nil {
		t.Fatal("Fetch returned no error on a 500")
	}
	if errors.Is(err, vocabconnector.ErrDegraded) {
		t.Fatal("Fetch must not report ErrDegraded; that is Search's contract")
	}
}

func TestVocabulariesListsWhatTheServiceServes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/vocab" {
			t.Errorf("path = %q, want /vocab", r.URL.Path)
		}
		// Shaped like the real listing, including the fields core does not
		// read: the decode must ignore profile, dump, scheme, kinds,
		// obsolete, languages, roots, built, endpoints and extras rather
		// than fail on them.
		_, _ = w.Write([]byte(`{"vocabs":[
			{"name":"aat","profile":"gvp","dump":"aat","scheme":"http://vocab.getty.edu/aat/",
			 "concepts":58996,"kinds":{"concept":57085,"guideTerm":1785,"hierarchy":118,"facet":8},
			 "obsolete":1332,"languages":["en","nl"],"languages_skipped":139,"roots":8,
			 "built":"2026-09-25T14:27:57Z",
			 "endpoints":{"suggest":"/vocab/aat/suggest","concept":"/vocab/aat/concept/{id}","children":"/vocab/aat/children/{id}"}},
			{"name":"tgn","profile":"gvp","concepts":2991143,
			 "extras":{"coordinates":2972719,"placeTypes":2991065},
			 "endpoints":{"suggest":"/vocab/tgn/suggest"}},
			{"name":"ulan","profile":"gvp","concepts":404637,
			 "extras":{"agentTypes":404637,"biographies":398155,"nationalities":401234},
			 "endpoints":{"suggest":"/vocab/ulan/suggest"}}]}`))
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL}, srv.Client())
	got, err := c.Vocabularies(context.Background())
	if err != nil {
		t.Fatalf("Vocabularies: %v", err)
	}
	want := []VocabularyInfo{
		{Name: "aat", Concepts: 58996},
		{Name: "tgn", Concepts: 2991143},
		{Name: "ulan", Concepts: 404637},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("Vocabularies mismatch (-want +got):\n%s", diff)
	}
}

// TestLiveVocabService runs only when KAKUGO_VOCAB_URL names a service, so it
// never runs in CI. Run it once by hand against https://vocab.pletka.io.
func TestLiveVocabService(t *testing.T) {
	base := os.Getenv("KAKUGO_VOCAB_URL")
	if base == "" {
		t.Skip("KAKUGO_VOCAB_URL not set")
	}

	for _, tc := range []struct{ vocab, query, lang, wantID string }{
		{"aat", "brons", "nl", "300010957"},
		{"tgn", "amsterdam", "en", "7006952"},
		{"ulan", "gogh", "en", "500115588"},
	} {
		t.Run(tc.vocab, func(t *testing.T) {
			c := New(Config{BaseURL: base, Vocab: tc.vocab}, nil)
			entries, err := c.Search(context.Background(), tc.query, vocabconnector.SearchOpts{Lang: tc.lang, Limit: 10})
			if err != nil {
				t.Fatalf("live Search: %v", err)
			}
			var found *vocabconnector.Entry
			for i := range entries {
				if entries[i].ExternalID == tc.wantID {
					found = &entries[i]
					break
				}
			}
			if found == nil {
				t.Fatalf("live Search for %q returned no entry with id %s; got %d entries", tc.query, tc.wantID, len(entries))
			}
			entry, err := c.Fetch(context.Background(), found.URI, vocabconnector.SearchOpts{Lang: tc.lang})
			if err != nil {
				t.Fatalf("live Fetch: %v", err)
			}
			if entry.Label[tc.lang] == "" {
				t.Fatalf("live Fetch returned no %s label: %+v", tc.lang, entry)
			}
			t.Logf("%s: %d suggestions, concept %s labeled %q, ancestors %v",
				tc.vocab, len(entries), entry.ExternalID, entry.Label[tc.lang], entry.BroaderPath)
		})
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (haystack == needle ||
		len(needle) > 0 && (indexOf(haystack, needle) >= 0))
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
