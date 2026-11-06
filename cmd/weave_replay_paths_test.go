package cmd

import (
	"testing"

	"github.com/google/go-cmp/cmp"
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
