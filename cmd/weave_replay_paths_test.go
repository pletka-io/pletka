package cmd

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/pletka-io/pletka/pkg/weave/ontology/autocomplete"
)

// TestReplaySteps pins the questions a stored path turns into: the scope in
// the root picker, then each element with the scope and the earlier elements
// as current_path, exactly as the path builder sends them.
func TestReplaySteps(t *testing.T) {
	got := replaySteps("crm:E21_Person", []string{"crm:P1_is_identified_by", "crm:E41_Appellation"})
	want := []replayStep{
		{Position: -1, Want: "crm:E21_Person", CurrentPath: nil},
		{Position: 0, Want: "crm:P1_is_identified_by", CurrentPath: []string{"crm:E21_Person"}},
		{Position: 1, Want: "crm:E41_Appellation", CurrentPath: []string{"crm:E21_Person", "crm:P1_is_identified_by"}},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("steps mismatch (-want +got):\n%s", diff)
	}
	if qnameLocal("xsd:string") != "string" || qnameLocal("plain") != "plain" {
		t.Errorf("qnameLocal")
	}
}

// TestReplayOffered pins that a suggestion counts when its qname matches, or
// when its IRI matches a stored element written under another prefix for
// the same namespace (the builder shows one prefix per namespace; the
// stored path may carry an older one).
func TestReplayOffered(t *testing.T) {
	sugs := []autocomplete.Suggestion{{Qname: "sdo:CoordinateBearer", URI: "https://nfdi4culture.de/ontology/schema-extension#CoordinateBearer", Type: "class"}}
	if !replayOffered(sugs, "sdoe:CoordinateBearer", "https://nfdi4culture.de/ontology/schema-extension#CoordinateBearer") {
		t.Fatalf("same IRI under another prefix must count as offered")
	}
	if !replayOffered(sugs, "sdo:CoordinateBearer", "") {
		t.Fatalf("qname match must count")
	}
	if replayOffered(sugs, "sdo:Other", "https://example.org/Other") {
		t.Fatalf("unrelated term must not count")
	}
}
