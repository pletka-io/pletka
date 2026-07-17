package ontology

import (
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestNaturalSortKey(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"plain word", "Birth", "Birth"},
		{"single digit", "E1", "E0001"},
		{"two digits", "E10", "E0010"},
		{"three digits", "P195", "P0195"},
		{"four digits stays as-is", "X1234", "X1234"},
		{"crm class with suffix", "E1_CRM_Entity", "E0001_CRM_Entity"},
		{"crm class higher number", "E89_Propositional_Object", "E0089_Propositional_Object"},
		{"crm class double-id", "E33_E41_Linguistic_Appellation", "E0033_E0041_Linguistic_Appellation"},
		{"property", "P1_is_identified_by", "P0001_is_identified_by"},
		{"property triple digit", "P100_was_death_of", "P0100_was_death_of"},
		{"non-numeric ontology", "schema:Person", "schema:Person"},
		{"only digits", "42", "0042"},
		{"leading digits", "1st_thing", "0001st_thing"},
		{"trailing digits", "thing_42", "thing_0042"},
		{"mixed runs", "a1b2c3", "a0001b0002c0003"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := naturalSortKey(tc.in)
			if got != tc.want {
				t.Errorf("naturalSortKey(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestNaturalSortKey_OrderingMatchesIntuition(t *testing.T) {
	input := []string{
		"E89_Propositional_Object",
		"E2_Temporal_Entity",
		"E10_Transfer_of_Custody",
		"E1_CRM_Entity",
		"E11_Modification",
		"E33_E41_Linguistic_Appellation",
		"E33_E1_Made_From",
	}
	want := []string{
		"E1_CRM_Entity",
		"E2_Temporal_Entity",
		"E10_Transfer_of_Custody",
		"E11_Modification",
		"E33_E1_Made_From",
		"E33_E41_Linguistic_Appellation",
		"E89_Propositional_Object",
	}

	got := append([]string(nil), input...)
	sort.Slice(got, func(i, j int) bool {
		return naturalSortKey(got[i]) < naturalSortKey(got[j])
	})

	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("natural sort order mismatch (-want +got):\n%s", diff)
	}
}
