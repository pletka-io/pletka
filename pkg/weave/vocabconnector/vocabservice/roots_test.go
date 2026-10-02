package vocabservice

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRoots_decodesCuratedFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/vocab/aat/roots" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if r.URL.Query().Get("descendants") != "1" {
			t.Errorf("descendants param not sent: %q", r.URL.RawQuery)
		}
		_, _ = io.WriteString(w, `{"vocab":"aat","resolvedLang":"en","roots":[
          {"uri":"http://vocab.getty.edu/aat/300445640","id":"300445640","kind":"concept","class":"Concept","prefLabel":{"en":"gender identity"},"lang":"en","narrowerTotal":15,"descendantsTotal":15,"label":{"en":"Gender","nl":"Gender"},"browse":"descendants","note":"15 below"}]}`)
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, Vocab: "aat"}, srv.Client())
	got, err := c.Roots(context.Background(), "en")
	if err != nil {
		t.Fatalf("Roots: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d", len(got))
	}
	e := got[0]
	if e.URI != "http://vocab.getty.edu/aat/300445640" {
		t.Errorf("uri = %q", e.URI)
	}
	if e.Label.Get("en") != "Gender" { // curated label wins over prefLabel
		t.Errorf("label = %q, want curated Gender", e.Label.Get("en"))
	}
	if e.Browse != "descendants" || e.Note != "15 below" {
		t.Errorf("browse=%q note=%q", e.Browse, e.Note)
	}
	if e.NarrowerTotal == nil || *e.NarrowerTotal != 15 {
		t.Errorf("narrowerTotal = %v", e.NarrowerTotal)
	}
	if e.DescendantsTotal == nil || *e.DescendantsTotal != 15 {
		t.Errorf("descendantsTotal = %v", e.DescendantsTotal)
	}
}

func TestRoots_emptyIsNotAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"vocab":"aat","resolvedLang":"en","roots":[]}`)
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, Vocab: "aat"}, srv.Client())
	got, err := c.Roots(context.Background(), "en")
	if err != nil || len(got) != 0 {
		t.Fatalf("want empty,nil got %v,%v", got, err)
	}
}

func TestChildren_decodesHasChildrenAndCounts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/vocab/aat/children/300445640" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if r.URL.Query().Get("descendants") != "1" {
			t.Errorf("descendants param not sent")
		}
		_, _ = io.WriteString(w, `{"uri":"http://x/300445640","id":"300445640","resolvedLang":"en","total":15,"children":[
          {"uri":"http://x/300438735","id":"300438735","kind":"concept","class":"Concept","prefLabel":{"en":"agender"},"lang":"en","narrowerTotal":0,"descendantsTotal":0,"hasChildren":false},
          {"uri":"http://x/300417544","id":"300417544","kind":"concept","class":"Concept","prefLabel":{"en":"genderqueer"},"lang":"en","narrowerTotal":2,"descendantsTotal":5,"hasChildren":true}]}`)
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, Vocab: "aat"}, srv.Client())
	got, err := c.Children(context.Background(), "http://x/300445640", "en", 100, 0)
	if err != nil {
		t.Fatalf("Children: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d", len(got))
	}
	if got[0].HasChildren == nil || *got[0].HasChildren {
		t.Errorf("child0 hasChildren = %v, want false", got[0].HasChildren)
	}
	if got[1].HasChildren == nil || !*got[1].HasChildren {
		t.Errorf("child1 hasChildren = %v, want true", got[1].HasChildren)
	}
	if got[1].DescendantsTotal == nil || *got[1].DescendantsTotal != 5 {
		t.Errorf("child1 descendantsTotal = %v", got[1].DescendantsTotal)
	}
}

func TestRoots_degradesOnServiceError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, `{"error":"vocab_unavailable","message":"down"}`)
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, Vocab: "aat"}, srv.Client())
	_, err := c.Roots(context.Background(), "en")
	if err == nil {
		t.Fatal("want error on 500")
	}
}
