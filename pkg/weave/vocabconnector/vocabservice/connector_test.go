package vocabservice

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/vocabconnector"
)

// TestSearchMapsAV2Hit covers the v2 suggest shape end to end: an item plus
// broader/parents, decoded straight off the contract's own example.
func TestSearchMapsAV2Hit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"vocab":"aat","resolvedLang":"en","q":"ethnic","offset":0,"limit":2,"total":12,
			"results":[
			 {"uri":"http://vocab.getty.edu/aat/300250435","id":"300250435",
			  "kind":"concept","class":"Concept",
			  "prefLabel":{"en":"ethnicity"},"lang":"en",
			  "matched":"prefLabel","matchedLabel":"ethnicity",
			  "broader":"http://vocab.getty.edu/aat/300162135",
			  "parents":[{"uri":"http://vocab.getty.edu/aat/300162135","id":"300162135",
			              "kind":"concept","class":"Concept",
			              "prefLabel":{"en":"culture-related concepts"},"lang":"en"}],
			  "score":9.8095}]}`))
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, Vocab: "aat"}, srv.Client())
	entries, err := c.Search(context.Background(), "ethnic", vocabconnector.SearchOpts{Lang: "en", Limit: 2})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	entry := entries[0]
	if entry.URI != "http://vocab.getty.edu/aat/300250435" {
		t.Errorf("URI = %q, want the URI verbatim (not rewritten)", entry.URI)
	}
	if entry.ExternalID != "300250435" {
		t.Errorf("ExternalID = %q, want 300250435", entry.ExternalID)
	}
	if diff := cmp.Diff(map[string]string{"en": "ethnicity"}, map[string]string(entry.Label)); diff != "" {
		t.Errorf("Label mismatch (-want +got):\n%s", diff)
	}
	if entry.BroaderURI != "http://vocab.getty.edu/aat/300162135" {
		t.Errorf("BroaderURI = %q", entry.BroaderURI)
	}
	if diff := cmp.Diff([]string{"culture-related concepts"}, entry.BroaderPath); diff != "" {
		t.Errorf("BroaderPath mismatch (-want +got):\n%s", diff)
	}
	if len(entry.BroaderPathItems) != 1 {
		t.Fatalf("got %d BroaderPathItems, want 1", len(entry.BroaderPathItems))
	}
	item0 := entry.BroaderPathItems[0]
	if item0.URI != "http://vocab.getty.edu/aat/300162135" {
		t.Errorf("BroaderPathItems[0].URI = %q", item0.URI)
	}
	if diff := cmp.Diff(map[string]string{"en": "culture-related concepts"}, map[string]string(item0.Label)); diff != "" {
		t.Errorf("BroaderPathItems[0].Label mismatch (-want +got):\n%s", diff)
	}
	if item0.ExternalID != "300162135" {
		t.Errorf("BroaderPathItems[0].ExternalID = %q, want 300162135", item0.ExternalID)
	}
}

// TestSearchKeepsAnAncestorsOwnLanguage is v2's whole point for parents: each
// ancestor resolves its own language rather than being stamped with the
// requested one, so a Dutch/English request can still surface a Spanish-only
// ancestor honestly.
func TestSearchKeepsAnAncestorsOwnLanguage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"results":[
			{"uri":"http://vocab.getty.edu/aat/300010957","id":"300010957",
			 "kind":"concept","class":"Concept","prefLabel":{"en":"bronze (metal)"},"lang":"en",
			 "broader":"http://vocab.getty.edu/aat/300010942",
			 "parents":[
			  {"uri":"http://vocab.getty.edu/aat/300010942","id":"300010942",
			   "kind":"concept","class":"Concept","prefLabel":{"en":"copper alloy"},"lang":"en"},
			  {"uri":"http://vocab.getty.edu/aat/300241441","id":"300241441",
			   "kind":"guideTerm","class":"GuideTerm","prefLabel":{"en":"<copper and copper alloy>"},"lang":"en"},
			  {"uri":"http://vocab.getty.edu/aat/300011014","id":"300011014",
			   "kind":"concept","class":"Concept","prefLabel":{"es":"metal no ferroso"},"lang":"es"}
			 ]}]}`))
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, Vocab: "aat"}, srv.Client())
	entries, err := c.Search(context.Background(), "bronze", vocabconnector.SearchOpts{Lang: "en"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(entries[0].BroaderPathItems) != 3 {
		t.Fatalf("got %d BroaderPathItems, want 3", len(entries[0].BroaderPathItems))
	}
	third := entries[0].BroaderPathItems[2]
	if _, ok := third.Label["es"]; !ok {
		t.Fatalf("third ancestor Label = %v, want it keyed \"es\"", third.Label)
	}
	if _, ok := third.Label["en"]; ok {
		t.Errorf("third ancestor Label = %v, must not be stamped with the requested language \"en\"", third.Label)
	}
	if diff := cmp.Diff([]string{"copper alloy", "<copper and copper alloy>", "metal no ferroso"}, entries[0].BroaderPath); diff != "" {
		t.Errorf("BroaderPath mismatch (-want +got):\n%s", diff)
	}
}

// TestSearchHandlesARootHit covers broader: null, parents: [] — a facet root
// or any concept with no ancestor.
func TestSearchHandlesARootHit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"results":[
			{"uri":"http://vocab.getty.edu/aat/300264091","id":"300264091",
			 "kind":"facet","class":"Facet","prefLabel":{"en":"Materials Facet"},"lang":"en",
			 "broader":null,"parents":[]}]}`))
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, Vocab: "aat"}, srv.Client())
	entries, err := c.Search(context.Background(), "Materials Facet", vocabconnector.SearchOpts{Lang: "en"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if entries[0].BroaderURI != "" {
		t.Errorf("BroaderURI = %q, want empty at a root", entries[0].BroaderURI)
	}
	if entries[0].BroaderPath != nil {
		t.Errorf("BroaderPath = %v, want empty at a root", entries[0].BroaderPath)
	}
	if entries[0].BroaderPathItems != nil {
		t.Errorf("BroaderPathItems = %v, want empty at a root", entries[0].BroaderPathItems)
	}
}

// TestSearchHandlesAnItemWithNoPrefLabel covers the one legitimate {}/null
// prefLabel case: an ancestor with no skos:prefLabel at all must still
// contribute a ref with its URI and id, not be silently dropped — that would
// shorten a breadcrumb without saying so.
func TestSearchHandlesAnItemWithNoPrefLabel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"results":[
			{"uri":"http://vocab.getty.edu/aat/300010957","id":"300010957",
			 "kind":"concept","class":"Concept","prefLabel":{"en":"bronze (metal)"},"lang":"en",
			 "broader":"http://vocab.getty.edu/aat/300099999",
			 "parents":[
			  {"uri":"http://vocab.getty.edu/aat/300099999","id":"300099999",
			   "kind":"concept","class":"Concept","prefLabel":{},"lang":null}
			 ]}]}`))
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, Vocab: "aat"}, srv.Client())
	entries, err := c.Search(context.Background(), "bronze", vocabconnector.SearchOpts{Lang: "en"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(entries[0].BroaderPathItems) != 1 {
		t.Fatalf("got %d BroaderPathItems, want 1 (the unlabeled ancestor must not be dropped)", len(entries[0].BroaderPathItems))
	}
	unlabeled := entries[0].BroaderPathItems[0]
	if unlabeled.URI != "http://vocab.getty.edu/aat/300099999" || unlabeled.ExternalID != "300099999" {
		t.Errorf("unlabeled ancestor = %+v, want URI/ExternalID set", unlabeled)
	}
	if len(unlabeled.Label) != 0 {
		t.Errorf("unlabeled ancestor Label = %v, want nil/empty", unlabeled.Label)
	}
	if len(entries[0].BroaderPath) != 1 {
		t.Fatalf("got %d BroaderPath entries, want 1 (dropping would shorten the breadcrumb)", len(entries[0].BroaderPath))
	}
}

// TestSearchSendsTheRequestV2Expects covers the request side: q, lang, limit
// always; under= only when ParentURI was passed, and as a bare id.
func TestSearchSendsTheRequestV2Expects(t *testing.T) {
	var gotPath, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery = r.URL.Path, r.URL.RawQuery
		_, _ = w.Write([]byte(`{"results":[]}`))
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, Vocab: "aat"}, srv.Client())
	if _, err := c.Search(context.Background(), "brons", vocabconnector.SearchOpts{
		Lang: "nl", Limit: 20, ParentURI: "https://vocab.getty.edu/aat/300264091",
	}); err != nil {
		t.Fatalf("Search: %v", err)
	}
	if gotPath != "/vocab/aat/suggest" {
		t.Errorf("path = %q, want /vocab/aat/suggest", gotPath)
	}
	for _, want := range []string{"q=brons", "lang=nl", "limit=20", "under=300264091"} {
		if !contains(gotQuery, want) {
			t.Errorf("query %q is missing %q", gotQuery, want)
		}
	}

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

// TestFetchReadsOneConcept covers the v2 /concept shape: an item plus
// broader/parents, same as suggest. Per the contract, concept/{id}?lang=
// takes a display-language parameter that "the concept and every nested
// item" resolve against server-side, so Fetch must send it (v1's connector
// deliberately did not; that assumption no longer holds under v2).
func TestFetchReadsOneConcept(t *testing.T) {
	var gotPath, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery = r.URL.Path, r.URL.RawQuery
		_, _ = w.Write([]byte(`{"uri":"http://vocab.getty.edu/aat/300010957","id":"300010957",
			"kind":"concept","class":"Concept",
			"prefLabel":{"en":"bronze (metal)","nl":"brons"},"lang":"en",
			"broader":"http://vocab.getty.edu/aat/300010942",
			"parents":[
			 {"uri":"http://vocab.getty.edu/aat/300010942","id":"300010942",
			  "kind":"concept","class":"Concept","prefLabel":{"en":"copper alloy"},"lang":"en"},
			 {"uri":"http://vocab.getty.edu/aat/300011014","id":"300011014",
			  "kind":"concept","class":"Concept","prefLabel":{"en":"metal"},"lang":"en"}]}`))
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
	if !contains(gotQuery, "lang=en") {
		t.Errorf("query %q is missing lang=en", gotQuery)
	}
	if entry.URI != "http://vocab.getty.edu/aat/300010957" {
		t.Errorf("URI = %q, want it verbatim (not rewritten)", entry.URI)
	}
	if entry.Label["en"] != "bronze (metal)" {
		t.Errorf("Label[en] = %q", entry.Label["en"])
	}
	if diff := cmp.Diff([]string{"copper alloy", "metal"}, entry.BroaderPath); diff != "" {
		t.Errorf("BroaderPath mismatch (-want +got):\n%s", diff)
	}
}

// TestFetchSendsLang covers the request side for a non-English row: the
// contract's lang param sets the display language for the concept and every
// nested item, so it must ride on every Fetch, not just the default.
func TestFetchSendsLang(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_, _ = w.Write([]byte(`{"uri":"http://vocab.getty.edu/aat/300010957","id":"300010957",
			"kind":"concept","class":"Concept","prefLabel":{"nl":"brons"},"lang":"nl"}`))
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, Vocab: "aat"}, srv.Client())
	if _, err := c.Fetch(context.Background(), "https://vocab.getty.edu/aat/300010957", vocabconnector.SearchOpts{Lang: "nl"}); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if !contains(gotQuery, "lang=nl") {
		t.Errorf("query %q is missing lang=nl", gotQuery)
	}
}

// TestFetchNarrowsTheStoredLanguages covers concept/{id}'s one real
// difference from every other item: prefLabel there holds every language the
// concept has (up to ~200 for TGN), not two. Storing all of them would carry
// a concept's whole language set where every other path stores one, so
// labelFor's narrowing (display language + English) applies here exactly as
// it does on an item elsewhere.
func TestFetchNarrowsTheStoredLanguages(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"uri":"http://vocab.getty.edu/aat/300010957","id":"300010957",
			"kind":"concept","class":"Concept",
			"prefLabel":{"en":"bronze (metal)","nl":"brons","de":"Bronze"},"lang":"nl"}`))
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, Vocab: "aat"}, srv.Client())
	entry, err := c.Fetch(context.Background(), "https://vocab.getty.edu/aat/300010957", vocabconnector.SearchOpts{Lang: "nl"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if diff := cmp.Diff(map[string]string{"nl": "brons", "en": "bronze (metal)"}, map[string]string(entry.Label)); diff != "" {
		t.Errorf("Label mismatch (-want +got):\n%s", diff)
	}
}

// TestFetchReadsScopeNoteInTheDisplayLanguagePairedWithEnglish covers
// scopeNote's ordinary hit case: the display language names one of its
// keys, and — like labelFor's prefLabel — the result is paired with English
// too, when present and different, because the frontend's tr() falls back
// requested-language-then-English-then-whatever's-there, and a note stored
// in Dutch alone would fall all the way through to some other language for
// a curator whose next-best language is English, not whatever is left.
func TestFetchReadsScopeNoteInTheDisplayLanguagePairedWithEnglish(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"uri":"http://vocab.getty.edu/aat/300010957","id":"300010957",
			"kind":"concept","class":"Concept",
			"prefLabel":{"nl":"brons"},"lang":"nl",
			"scopeNote":{"en":"a copper alloy","nl":"een koperlegering","de":"eine Kupferlegierung"}}`))
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, Vocab: "aat"}, srv.Client())
	entry, err := c.Fetch(context.Background(), "https://vocab.getty.edu/aat/300010957", vocabconnector.SearchOpts{Lang: "nl"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if diff := cmp.Diff(map[string]string{"nl": "een koperlegering", "en": "a copper alloy"}, map[string]string(entry.ScopeNote)); diff != "" {
		t.Errorf("ScopeNote mismatch (-want +got):\n%s", diff)
	}
}

// TestFetchReadsScopeNoteInEnglishWithNoDuplicateKey covers the "and
// differs" half of the pairing rule: an English display language must not
// duplicate the "en" key, the same way labelFor's own English case does not.
func TestFetchReadsScopeNoteInEnglishWithNoDuplicateKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"uri":"http://vocab.getty.edu/aat/300010957","id":"300010957",
			"kind":"concept","class":"Concept",
			"prefLabel":{"en":"bronze (metal)"},"lang":"en",
			"scopeNote":{"en":"a copper alloy","nl":"een koperlegering"}}`))
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, Vocab: "aat"}, srv.Client())
	entry, err := c.Fetch(context.Background(), "https://vocab.getty.edu/aat/300010957", vocabconnector.SearchOpts{Lang: "en"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if diff := cmp.Diff(map[string]string{"en": "a copper alloy"}, map[string]string(entry.ScopeNote)); diff != "" {
		t.Errorf("ScopeNote mismatch (-want +got):\n%s", diff)
	}
}

// TestFetchScopeNoteFallsBackToEnglishWhenDisplayLanguageHasNone pins the
// bug this connector shipped with: prefLabel guarantees lang names one of
// its own keys, but the contract makes no such promise about scopeNote — a
// concept can have a German prefLabel and no German scopeNote at all. Using
// labelFor's exact-key rule for scopeNote too silently dropped the note in
// every language that had none of its own (seven of this concept's fourteen
// display languages, measured live), and worse: Fetch's result reaches an
// upsert with no guard against overwriting a stored scope note with
// nothing. lang here is "de" (present in prefLabel, matching the live bug
// report) with no "de" key in scopeNote at all, so this must fall back to
// English rather than return nil.
func TestFetchScopeNoteFallsBackToEnglishWhenDisplayLanguageHasNone(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"uri":"http://vocab.getty.edu/aat/300010957","id":"300010957",
			"kind":"concept","class":"Concept",
			"prefLabel":{"de":"Bronze"},"lang":"de",
			"scopeNote":{"en":"a copper alloy","nl":"een koperlegering"}}`))
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, Vocab: "aat"}, srv.Client())
	entry, err := c.Fetch(context.Background(), "https://vocab.getty.edu/aat/300010957", vocabconnector.SearchOpts{Lang: "de"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if diff := cmp.Diff(map[string]string{"en": "a copper alloy"}, map[string]string(entry.ScopeNote)); diff != "" {
		t.Errorf("ScopeNote mismatch (-want +got):\n%s", diff)
	}
}

// TestFetchScopeNoteFallsBackToUndWhenNeitherDisplayLanguageNorEnglishHaveOne
// covers scopeNote's second fallback: the contract calls scopeNote out as
// the one predicate whose values can still arrive with no language tag at
// all, so an untagged note must not be dropped just because it is not
// keyed "de" or "en".
func TestFetchScopeNoteFallsBackToUndWhenNeitherDisplayLanguageNorEnglishHaveOne(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"uri":"http://vocab.getty.edu/aat/300010957","id":"300010957",
			"kind":"concept","class":"Concept",
			"prefLabel":{"de":"Bronze"},"lang":"de",
			"scopeNote":{"und":"an untagged note"}}`))
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, Vocab: "aat"}, srv.Client())
	entry, err := c.Fetch(context.Background(), "https://vocab.getty.edu/aat/300010957", vocabconnector.SearchOpts{Lang: "de"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if diff := cmp.Diff(map[string]string{"und": "an untagged note"}, map[string]string(entry.ScopeNote)); diff != "" {
		t.Errorf("ScopeNote mismatch (-want +got):\n%s", diff)
	}
}

// TestFetchScopeNoteAbsentWhenNoneOfTheFallbacksMatch covers a concept with
// a scopeNote that has neither the display language, nor English, nor an
// untagged value: the fallback chain is exhausted, and the result must be
// nil rather than an empty map or a language the concept does not have.
func TestFetchScopeNoteAbsentWhenNoneOfTheFallbacksMatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"uri":"http://vocab.getty.edu/aat/300010957","id":"300010957",
			"kind":"concept","class":"Concept",
			"prefLabel":{"de":"Bronze"},"lang":"de",
			"scopeNote":{"fr":"une note"}}`))
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, Vocab: "aat"}, srv.Client())
	entry, err := c.Fetch(context.Background(), "https://vocab.getty.edu/aat/300010957", vocabconnector.SearchOpts{Lang: "de"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(entry.ScopeNote) != 0 {
		t.Errorf("ScopeNote = %v, want none (display language, English and und all absent)", entry.ScopeNote)
	}
}

// TestFetchKeepsAncestorLanguages is the contract's own bronze example: an en
// fetch whose third ancestor (AAT 300011014) has no English prefLabel at all,
// so it is honestly reported in Spanish rather than stamped with the
// requested language the way v1's parentString did.
func TestFetchKeepsAncestorLanguages(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"uri":"http://vocab.getty.edu/aat/300010957","id":"300010957",
			"kind":"concept","class":"Concept","prefLabel":{"en":"bronze (metal)"},"lang":"en",
			"broader":"http://vocab.getty.edu/aat/300010942",
			"parents":[
			 {"uri":"http://vocab.getty.edu/aat/300010942","id":"300010942",
			  "kind":"concept","class":"Concept","prefLabel":{"en":"copper alloy"},"lang":"en"},
			 {"uri":"http://vocab.getty.edu/aat/300241441","id":"300241441",
			  "kind":"guideTerm","class":"GuideTerm","prefLabel":{"en":"<copper and copper alloy>"},"lang":"en"},
			 {"uri":"http://vocab.getty.edu/aat/300011014","id":"300011014",
			  "kind":"concept","class":"Concept","prefLabel":{"es":"metal no ferroso"},"lang":"es"}]}`))
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, Vocab: "aat"}, srv.Client())
	entry, err := c.Fetch(context.Background(), "https://vocab.getty.edu/aat/300010957", vocabconnector.SearchOpts{Lang: "en"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(entry.BroaderPathItems) != 3 {
		t.Fatalf("got %d BroaderPathItems, want 3", len(entry.BroaderPathItems))
	}
	third := entry.BroaderPathItems[2]
	if _, ok := third.Label["es"]; !ok {
		t.Fatalf("third ancestor Label = %v, want it keyed \"es\"", third.Label)
	}
	if _, ok := third.Label["en"]; ok {
		t.Errorf("third ancestor Label = %v, must not be stamped with the requested language \"en\"", third.Label)
	}
}

// TestFetchOnPrefLabelNotAnObjectErrors covers a prefLabel that is not an
// object at all (here, a JSON number): the contract guarantees prefLabel is
// always a language-keyed object, so anything else is a real decode error,
// not an empty entry.
func TestFetchOnPrefLabelNotAnObjectErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"uri":"http://vocab.getty.edu/aat/300250435","id":"300250435","prefLabel":42}`))
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, Vocab: "aat"}, srv.Client())
	entry, err := c.Fetch(context.Background(), "https://vocab.getty.edu/aat/300250435", vocabconnector.SearchOpts{})
	if err == nil {
		t.Fatalf("Fetch with a non-object prefLabel returned no error, entry=%+v", entry)
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

// TestVocabulariesReadsTheListing covers the v2 discovery shape: a top-level
// version plus, per mount, name/label/concepts/languages. Shaped like the
// real listing, including the fields core does not read yet: the decode
// must ignore profile, dump, scheme, kinds, obsolete, languages_skipped,
// roots, built, source, license, endpoints and extras rather than fail on
// them.
func TestVocabulariesReadsTheListing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/vocab" {
			t.Errorf("path = %q, want /vocab", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"version":2,"vocabs":[
			{"name":"aat","version":2,"label":"Art & Architecture Thesaurus","profile":"gvp","dump":"aat",
			 "scheme":"http://vocab.getty.edu/aat/","concepts":58996,
			 "kinds":{"concept":57085,"guideTerm":1785,"hierarchy":118,"facet":8},
			 "obsolete":1332,"languages":["ar","en","nl"],"languages_skipped":139,"roots":8,
			 "built":"2026-09-25T14:27:57Z",
			 "source":{"url":"http://aatdownloads.getty.edu/VocabData/explicit.zip"},
			 "license":{"name":"ODC-By 1.0"},
			 "endpoints":{"suggest":"/vocab/aat/suggest","concept":"/vocab/aat/concept/{id}","children":"/vocab/aat/children/{id}"}},
			{"name":"tgn","version":2,"label":"Getty Thesaurus of Geographic Names","profile":"gvp","concepts":2991143,
			 "extras":{"coordinates":2972719,"placeTypes":2991065},"languages":["en"],
			 "endpoints":{"suggest":"/vocab/tgn/suggest"}},
			{"name":"ulan","version":2,"profile":"gvp","concepts":404637,
			 "extras":{"agentTypes":404637,"biographies":398155,"nationalities":401234},"languages":["en"],
			 "endpoints":{"suggest":"/vocab/ulan/suggest"}}]}`))
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL}, srv.Client())
	got, err := c.Vocabularies(context.Background())
	if err != nil {
		t.Fatalf("Vocabularies: %v", err)
	}
	if got.Version != 2 {
		t.Errorf("Version = %d, want 2", got.Version)
	}
	want := []VocabularyInfo{
		{Name: "aat", Label: "Art & Architecture Thesaurus", Concepts: 58996, Languages: []string{"ar", "en", "nl"}},
		{Name: "tgn", Label: "Getty Thesaurus of Geographic Names", Concepts: 2991143, Languages: []string{"en"}},
		{Name: "ulan", Concepts: 404637, Languages: []string{"en"}},
	}
	if diff := cmp.Diff(want, got.Vocabs); diff != "" {
		t.Errorf("Vocabs mismatch (-want +got):\n%s", diff)
	}
}

// TestVocabulariesWarnsOnAnUnexpectedVersion covers the owner's decision:
// warn, do not fail. A service reporting something other than the contract
// version this connector decodes still returns its listing, but logs once —
// and a service reporting the expected version must not log anything, so the
// test would fail if the warning were deleted rather than pass vacuously.
func TestVocabulariesWarnsOnAnUnexpectedVersion(t *testing.T) {
	t.Run("unexpected version logs and still returns the listing", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"version":3,"vocabs":[{"name":"aat","concepts":58996}]}`))
		}))
		defer srv.Close()

		var buf bytes.Buffer
		c := New(Config{BaseURL: srv.URL}, srv.Client())
		c.logger = slog.New(slog.NewTextHandler(&buf, nil))
		got, err := c.Vocabularies(context.Background())
		if err != nil {
			t.Fatalf("Vocabularies: %v", err)
		}
		if len(got.Vocabs) != 1 || got.Vocabs[0].Name != "aat" {
			t.Fatalf("Vocabularies = %+v, want the listing to still come back", got)
		}
		if !strings.Contains(buf.String(), "level=WARN") {
			t.Errorf("log output = %q, want a warning logged for version 3", buf.String())
		}
	})

	t.Run("version 2 logs nothing", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"version":2,"vocabs":[{"name":"aat","concepts":58996}]}`))
		}))
		defer srv.Close()

		var buf bytes.Buffer
		c := New(Config{BaseURL: srv.URL}, srv.Client())
		c.logger = slog.New(slog.NewTextHandler(&buf, nil))
		if _, err := c.Vocabularies(context.Background()); err != nil {
			t.Fatalf("Vocabularies: %v", err)
		}
		if buf.Len() != 0 {
			t.Errorf("log output = %q, want no warning for the expected version", buf.String())
		}
	})
}

// TestLiveVocabService runs only when KAKUGO_VOCAB_URL names a service, so it
// never runs in CI. Run it once by hand against https://vocab.pletka.io:
//
//	KAKUGO_VOCAB_URL=https://vocab.pletka.io go test ./pkg/weave/vocabconnector/vocabservice/ -run TestLive -count=1 -v
//
// It exercises aat, tgn and ulan with a Search and a Fetch each, and
// separately proves the contract document's bronze example against the real
// service: AAT 300010957 fetched at lang=en has a third ancestor (300011014)
// with no English prefLabel, so v2 must report it honestly in Spanish rather
// than stamp it with the requested language the way v1's parentString did.
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

	// TestLiveVocabService/aat_ancestor_keeps_its_own_language is the one
	// assertion the v1 connector could never make: fetch a concept whose
	// ancestor chain contains a link in a language other than the requested
	// one, and assert we kept that link's own language rather than silently
	// stamping it with "en". This is the contract document's bronze example.
	t.Run("aat ancestor keeps its own language", func(t *testing.T) {
		c := New(Config{BaseURL: base, Vocab: "aat"}, nil)
		entry, err := c.Fetch(context.Background(), "http://vocab.getty.edu/aat/300010957", vocabconnector.SearchOpts{Lang: "en"})
		if err != nil {
			t.Fatalf("live Fetch: %v", err)
		}
		var ancestor *domain.VocabularyEntryRef
		for i := range entry.BroaderPathItems {
			if entry.BroaderPathItems[i].ExternalID == "300011014" {
				ancestor = &entry.BroaderPathItems[i]
				break
			}
		}
		if ancestor == nil {
			t.Fatalf("live Fetch of 300010957 (lang=en) has no ancestor 300011014; got %v", entry.BroaderPath)
		}
		if _, ok := ancestor.Label["es"]; !ok {
			t.Fatalf("ancestor 300011014 Label = %v, want it keyed \"es\" (this concept has no English prefLabel)", ancestor.Label)
		}
		if _, ok := ancestor.Label["en"]; ok {
			t.Errorf("ancestor 300011014 Label = %v, must not be stamped with the requested language \"en\"", ancestor.Label)
		}
		t.Logf("aat 300010957 (lang=en): ancestor 300011014 kept its own language, Label = %v, BroaderPath = %v",
			ancestor.Label, entry.BroaderPath)
	})
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

// TestSearchWithNoQueryBrowsesUnderTheParent is the regression gate for a
// half-ported guard. The v2 connector bailed on an empty query before it ever
// looked at ParentURI, so "pick a parent term and show me what is under it"
// — which the aat connector supports, and which the service answers as
// q=&under=<id> — returned nothing at all. The picker already issues that
// request on load, so the feature was silently dead rather than absent.
func TestSearchWithNoQueryBrowsesUnderTheParent(t *testing.T) {
	var called bool
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called, gotQuery = true, r.URL.RawQuery
		_, _ = w.Write([]byte(`{"results":[{"uri":"http://vocab.getty.edu/aat/300448875","id":"300448875","prefLabel":{"en":"cèyàng"},"lang":"en"}]}`))
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, Vocab: "aat"}, srv.Client())
	got, err := c.Search(context.Background(), "", vocabconnector.SearchOpts{
		Lang: "en", Limit: 20, ParentURI: "http://vocab.getty.edu/aat/300054196",
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if !called {
		t.Fatal("no request reached the service — the empty query short-circuited before the parent was consulted")
	}
	if !contains(gotQuery, "under=300054196") {
		t.Errorf("query %q is missing under=300054196", gotQuery)
	}
	if len(got) != 1 {
		t.Fatalf("got %d entries, want the parent's one child", len(got))
	}
}

// TestSearchWithNeitherQueryNorParentStaysANoOp pins the other half of the
// same condition: with nothing to search for and nothing to scope by, the
// connector must still not call the service. Without this, widening the guard
// above would turn every idle picker into an unscoped fetch.
func TestSearchWithNeitherQueryNorParentStaysANoOp(t *testing.T) {
	var called bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		_, _ = w.Write([]byte(`{"results":[]}`))
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, Vocab: "aat"}, srv.Client())
	got, err := c.Search(context.Background(), "   ", vocabconnector.SearchOpts{Lang: "en", Limit: 20})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if called {
		t.Error("the service was called with neither a query nor a parent")
	}
	if got != nil {
		t.Errorf("got %v, want nil", got)
	}
}
