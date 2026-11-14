package canonical

import (
	"strings"
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
)

// TestConceptList_CanonicalKeys pins the keys and order of a concept-list
// file in the project tree. Entries and broader edges are not part of it.
func TestConceptList_CanonicalKeys(t *testing.T) {
	l := &domain.ConceptList{IsClosed: true}
	l.SemanticID = "TPC.CL.3"
	l.SystemName = "license"
	l.UIName = domain.Translations{"en": "License"}
	l.Status = "draft"
	got, err := ConceptList(l, "")
	if err != nil {
		t.Fatal(err)
	}
	want := "description: {}\nis_closed: true\nsemantic_id: TPC.CL.3\nstatus: draft\nsystem_name: license\nui_name:\n  en: License\n"
	if string(got) != want {
		t.Errorf("canonical concept list mismatch:\n got %q\nwant %q", got, want)
	}
}

func TestConceptList_Vocabulary(t *testing.T) {
	l := &domain.ConceptList{}
	l.SemanticID, l.SystemName, l.Status = "SI.CL.1", "x", "draft"
	got, err := ConceptList(l, "SI.VOC.1")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "vocabulary: SI.VOC.1\n") {
		t.Errorf("missing vocabulary key:\n%s", got)
	}
}
