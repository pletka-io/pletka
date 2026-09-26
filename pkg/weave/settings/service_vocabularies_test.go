package settings

import (
	"context"
	"errors"
	"testing"
)

type stubLister struct {
	out []ServiceVocabulary
	err error
}

func (s stubLister) ServiceVocabularies(context.Context) ([]ServiceVocabulary, error) {
	return s.out, s.err
}

func TestServiceVocabulariesPassesThroughTheListing(t *testing.T) {
	want := []ServiceVocabulary{{Name: "aat", Label: "Art & Architecture Thesaurus", Concepts: 58996, Languages: []string{"en", "nl"}}}
	got, err := stubLister{out: want}.ServiceVocabularies(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].Name != "aat" || len(got[0].Languages) != 2 {
		t.Errorf("listing = %+v, want one aat entry with two languages", got)
	}
}

func TestServiceVocabulariesSurfacesAnError(t *testing.T) {
	sentinel := errors.New("service unreachable")
	if _, err := (stubLister{err: sentinel}).ServiceVocabularies(context.Background()); !errors.Is(err, sentinel) {
		t.Errorf("error = %v, want it to wrap the sentinel — configuration must not swallow failures", err)
	}
}
