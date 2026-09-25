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

// TestSearchReturnsErrDegradedOnTransportFailure covers the transport-failure
// degraded path (as opposed to a non-2xx status or a timeout): the server is
// closed before the call, so the client never gets a response at all.
func TestSearchReturnsErrDegradedOnTransportFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	client := srv.Client()
	base := srv.URL
	srv.Close()

	c := New(Config{BaseURL: base, Vocab: "aat"}, client)
	entries, err := c.Search(context.Background(), "brons", vocabconnector.SearchOpts{})
	if len(entries) != 0 {
		t.Errorf("got %d entries, want none", len(entries))
	}
	if !errors.Is(err, vocabconnector.ErrDegraded) {
		t.Fatalf("err = %v, want ErrDegraded", err)
	}
}

// TestSearchReturnsErrDegradedOnUndecodableBody covers the fourth degraded
// path: a 200 whose body does not parse as JSON at all.
func TestSearchReturnsErrDegradedOnUndecodableBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[`))
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

// TestSearchWithNoVocabConfiguredReturnsErrDegradedNamingTheProblem covers
// item 9: a row with no vocabulary name configured must fail clearly rather
// than calling /vocab//suggest on every keystroke and reporting a permanent,
// misleading degradation.
func TestSearchWithNoVocabConfiguredReturnsErrDegradedNamingTheProblem(t *testing.T) {
	c := New(Config{BaseURL: "http://unused.invalid"}, http.DefaultClient)
	entries, err := c.Search(context.Background(), "brons", vocabconnector.SearchOpts{})
	if len(entries) != 0 {
		t.Errorf("got %d entries, want none", len(entries))
	}
	if !errors.Is(err, vocabconnector.ErrDegraded) {
		t.Fatalf("err = %v, want ErrDegraded", err)
	}
	if !errors.Is(err, errNoVocab) {
		t.Fatalf("err = %v, want it to name the missing vocabulary", err)
	}
}

// TestFetchWithNoVocabConfiguredReturnsAClearError is Fetch's side of item 9:
// an explicit lookup with a misconfigured row is a clear error, not
// ErrDegraded — that sentinel is Search's contract, not Fetch's.
func TestFetchWithNoVocabConfiguredReturnsAClearError(t *testing.T) {
	c := New(Config{BaseURL: "http://unused.invalid"}, http.DefaultClient)
	_, err := c.Fetch(context.Background(), "https://vocab.getty.edu/aat/300010957", vocabconnector.SearchOpts{})
	if err == nil {
		t.Fatal("Fetch returned no error with no vocabulary configured")
	}
	if errors.Is(err, vocabconnector.ErrDegraded) {
		t.Fatal("Fetch must not report ErrDegraded; that is Search's contract")
	}
	if !errors.Is(err, errNoVocab) {
		t.Fatalf("err = %v, want it to name the missing vocabulary", err)
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
	var gotPath, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery = r.URL.Path, r.URL.RawQuery
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
	// concept/{id} takes no lang parameter in the contract; sending one is a
	// request a future implementation may reject.
	if gotQuery != "" {
		t.Errorf("query = %q, want no parameters", gotQuery)
	}
	if entry.Label["en"] != "bronze (metal)" {
		t.Errorf("Label[en] = %q", entry.Label["en"])
	}
}

// TestFetchReadsOneConceptWithLanguageMappedFields is the regression guard for
// the /concept response shape: unlike /suggest, prefLabel and parentString are
// language-keyed objects there, not plain strings. Reverting hitText back to a
// bare `string` field leaves every other test in this file green (they all
// mock /suggest's shape) while silently restoring the decode failure a live
// run against the real service caught.
func TestFetchReadsOneConceptWithLanguageMappedFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"uri":"http://vocab.getty.edu/aat/300250435","id":"300250435",
			"prefLabel":{"en":"ethnicity","nl":"etniciteit"},
			"parentString":{"en":"culture-related concepts, associated concepts",
			                "nl":"cultuurgerelateerde concepten, geassocieerde concepten"},
			"broader":"http://vocab.getty.edu/aat/300264086"}`))
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, Vocab: "aat"}, srv.Client())
	entry, err := c.Fetch(context.Background(), "https://vocab.getty.edu/aat/300250435", vocabconnector.SearchOpts{Lang: "nl"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if entry.Label["nl"] != "etniciteit" {
		t.Errorf("Label[nl] = %q, want etniciteit", entry.Label["nl"])
	}
	if diff := cmp.Diff([]string{"cultuurgerelateerde concepten", "geassocieerde concepten"}, entry.BroaderPath); diff != "" {
		t.Errorf("BroaderPath mismatch (-want +got):\n%s", diff)
	}
}

// TestFetchOnPrefLabelOfNeitherShapeErrors covers the third decode outcome:
// a prefLabel that is neither a plain string nor a language object (here, a
// JSON number) must surface as a real decode error, not an empty entry.
func TestFetchOnPrefLabelOfNeitherShapeErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"uri":"http://vocab.getty.edu/aat/300250435","id":"300250435","prefLabel":42}`))
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, Vocab: "aat"}, srv.Client())
	entry, err := c.Fetch(context.Background(), "https://vocab.getty.edu/aat/300250435", vocabconnector.SearchOpts{})
	if err == nil {
		t.Fatalf("Fetch with a prefLabel of neither shape returned no error, entry=%+v", entry)
	}
}

// TestFetchFallsBackToEnglishWhenTheConceptHasNoLabelInTheRequestedLanguage is
// the concrete case from review: fetching ULAN 500115588 in Dutch must not
// drop the English ancestor chain just because the concept has no Dutch
// prefLabel. Search's own fallback and Fetch's must agree on the same
// concept, or the vocabulary service persists a permanently empty path.
func TestFetchFallsBackToEnglishWhenTheConceptHasNoLabelInTheRequestedLanguage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"uri":"http://vocab.getty.edu/ulan/500115588","id":"500115588",
			"prefLabel":{"en":"Gogh, Vincent van"},
			"parentString":{"en":"Persons, Artists"},
			"broader":"http://vocab.getty.edu/ulan/500000002"}`))
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, Vocab: "ulan"}, srv.Client())
	entry, err := c.Fetch(context.Background(), "https://vocab.getty.edu/ulan/500115588", vocabconnector.SearchOpts{Lang: "nl"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if entry.Label["en"] != "Gogh, Vincent van" {
		t.Errorf("Label[en] = %q, want the English fallback", entry.Label["en"])
	}
	if diff := cmp.Diff([]string{"Persons", "Artists"}, entry.BroaderPath); diff != "" {
		t.Errorf("BroaderPath mismatch (-want +got):\n%s", diff)
	}
}

// TestFetchNarrowsTheStoredLabelToTheResolvedLanguagePlusEnglish is the ruled
// fix for item 3: the stored entry carries the language Fetch resolved to,
// plus English when that differs, never the concept's whole language set —
// otherwise adopting one TGN place would persist up to ~200 language keys.
func TestFetchNarrowsTheStoredLabelToTheResolvedLanguagePlusEnglish(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"uri":"http://vocab.getty.edu/tgn/7006952","id":"7006952",
			"prefLabel":{"en":"Amsterdam","nl":"Amsterdam","de":"Amsterdam","fr":"Amsterdam"},
			"parentString":{"en":"North Holland, Netherlands, Europe, World",
			                "nl":"Noord-Holland, Nederland, Europa, Wereld"},
			"broader":"http://vocab.getty.edu/tgn/7006951"}`))
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, Vocab: "tgn"}, srv.Client())
	entry, err := c.Fetch(context.Background(), "https://vocab.getty.edu/tgn/7006952", vocabconnector.SearchOpts{Lang: "nl"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	want := map[string]string{"nl": "Amsterdam", "en": "Amsterdam"}
	if diff := cmp.Diff(want, map[string]string(entry.Label)); diff != "" {
		t.Errorf("Label mismatch (-want +got):\n%s", diff)
	}
}

// TestFetchResolvesLanguageCaseInsensitively covers item 4: the contract
// matches lang case-insensitively and echoes the vocabulary's own spelling,
// so a caller asking for "ZH-HANT" must still find a map keyed "zh-Hant".
func TestFetchResolvesLanguageCaseInsensitively(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"uri":"http://vocab.getty.edu/aat/300010957","id":"300010957",
			"prefLabel":{"zh-Hant":"青銅"},
			"parentString":{"zh-Hant":"合金, 金屬"},
			"broader":"http://vocab.getty.edu/aat/300010942"}`))
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, Vocab: "aat"}, srv.Client())
	entry, err := c.Fetch(context.Background(), "https://vocab.getty.edu/aat/300010957", vocabconnector.SearchOpts{Lang: "ZH-HANT"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if entry.Label["zh-Hant"] != "青銅" {
		t.Errorf("Label[zh-Hant] = %q", entry.Label["zh-Hant"])
	}
	if entry.BroaderPath == nil {
		t.Error("BroaderPath is nil; case-insensitive lang match should have found the zh-Hant chain")
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
