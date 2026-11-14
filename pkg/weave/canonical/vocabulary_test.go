package canonical

import (
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
)

func TestVocabulary_CanonicalKeys(t *testing.T) {
	v := &domain.Vocabulary{BaseURI: "http://localhost:8000/"}
	v.SemanticID, v.SystemName, v.Status = "SI.VOC.1", "arches", "active"
	v.UIName = domain.Translations{"en": "Arches"}
	got, err := Vocabulary(v)
	if err != nil {
		t.Fatal(err)
	}
	want := "base_uri: http://localhost:8000/\ndescription: {}\nsemantic_id: SI.VOC.1\nstatus: active\nsystem_name: arches\nui_name:\n  en: Arches\n"
	if string(got) != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
}

func TestVocabularyConcepts_SortedAndOmitsEmpty(t *testing.T) {
	got, err := VocabularyConcepts([]TreeConcept{
		{URI: "http://x/2", Label: domain.Translations{"en": "B"}, Broader: []string{"http://x/1", "http://x/3"}},
		{URI: "http://x/1", Label: domain.Translations{"en": "A"}, TopConcept: true, ExternalID: "1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := "- external_id: \"1\"\n  label:\n    en: A\n  top_concept: true\n  uri: http://x/1\n" +
		"- broader:\n    - http://x/1\n    - http://x/3\n  label:\n    en: B\n  uri: http://x/2\n"
	if string(got) != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
}

func TestVocabularyConcepts_OptionalKeys(t *testing.T) {
	got, err := VocabularyConcepts([]TreeConcept{{
		URI: "http://x/1", Label: domain.Translations{"en": "A"},
		ScopeNote: domain.Translations{"en": "note"}, ExactMatch: []string{"http://y/1"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	want := "- exact_match:\n    - http://y/1\n  label:\n    en: A\n  scope_note:\n    en: note\n  uri: http://x/1\n"
	if string(got) != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
}

func TestConceptListEntries_KeepsOrder(t *testing.T) {
	got, err := ConceptListEntries([]string{"http://x/2", "http://x/1"})
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "- http://x/2\n- http://x/1\n" {
		t.Fatalf("got %q", got)
	}
}
