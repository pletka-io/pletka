package autocomplete

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestReplaySteps(t *testing.T) {
	got := ReplaySteps("crm:E21_Person", "u:E21", []string{"crm:P1_is_identified_by", "crm:E41_Appellation"}, []string{"u:P1", "u:E41"})
	want := []ReplayStep{
		{Position: -1, Want: "crm:E21_Person", WantURI: "u:E21"},
		{Position: 0, Want: "crm:P1_is_identified_by", WantURI: "u:P1", CurrentPath: []string{"crm:E21_Person"}},
		{Position: 1, Want: "crm:E41_Appellation", WantURI: "u:E41", CurrentPath: []string{"crm:E21_Person", "crm:P1_is_identified_by"}},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("(-want +got):\n%s", diff)
	}
}

func TestOffered_ByQnameOrIRI(t *testing.T) {
	sugs := []Suggestion{{Qname: "sdo:X", URI: "https://e.org/X"}}
	if !Offered(sugs, "sdoe:X", "https://e.org/X") || !Offered(sugs, "sdo:X", "") || Offered(sugs, "sdo:Y", "https://e.org/Y") {
		t.Fatal("Offered")
	}
	if QnameLocal("xsd:string") != "string" || QnameLocal("plain") != "plain" {
		t.Fatal("QnameLocal")
	}
}

func TestReplayPath_ReportsMisses(t *testing.T) {
	suggest := func(_ context.Context, req Request) ([]Suggestion, error) {
		switch len(req.CurrentPath) {
		case 0:
			return []Suggestion{{Qname: "base:Thing"}}, nil
		case 1:
			return []Suggestion{{Qname: "base:name"}}, nil
		default:
			return nil, nil // nothing offered after base:name
		}
	}
	misses, steps, err := ReplayPath(context.Background(), suggest, "P", "base:Thing", "", []string{"base:name", "xsd:string"}, []string{"", ""})
	if err != nil || steps != 3 || len(misses) != 1 || misses[0].Position != 1 || misses[0].Want != "xsd:string" {
		t.Fatalf("misses=%v steps=%d err=%v", misses, steps, err)
	}
}
