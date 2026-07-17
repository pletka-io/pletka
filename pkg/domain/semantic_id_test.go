package domain

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestParseSemanticID(t *testing.T) {
	tests := []struct {
		input string
		want  SemanticID
	}{
		{
			input: "LAF.309",
			want: SemanticID{
				Raw: "LAF.309", ProjectID: "LA", TypeCode: "F",
				EntityType: "field", Number: 309,
			},
		},
		{
			input: "SRDM.5",
			want: SemanticID{
				Raw: "SRDM.5", ProjectID: "SRD", TypeCode: "M",
				EntityType: "model", Number: 5,
			},
		},
		{
			input: "SRDC.8",
			want: SemanticID{
				Raw: "SRDC.8", ProjectID: "SRD", TypeCode: "C",
				EntityType: "collection", Number: 8,
			},
		},
		{
			input: "LA.CAT.5",
			want: SemanticID{
				Raw: "LA.CAT.5", ProjectID: "LA", TypeCode: "CAT",
				EntityType: "category", Number: 5,
			},
		},
		{
			input: "SEM.CL.6",
			want: SemanticID{
				Raw: "SEM.CL.6", ProjectID: "SEM", TypeCode: "CL",
				EntityType: "concept_list", Number: 6,
			},
		},
		{
			input: "AFSF.10",
			want: SemanticID{
				Raw: "AFSF.10", ProjectID: "AFS", TypeCode: "F",
				EntityType: "field", Number: 10,
			},
		},
		{
			input: "LAM.15_person",
			want: SemanticID{
				Raw: "LAM.15_person", ProjectID: "LA", TypeCode: "M",
				EntityType: "model", Number: 15, SystemName: "person",
			},
		},
		{
			input: "",
			want:  SemanticID{},
		},
		{
			input: "garbage",
			want:  SemanticID{Raw: "garbage"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got := ParseSemanticID(tc.input)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("ParseSemanticID(%q) mismatch (-want +got):\n%s", tc.input, diff)
			}
		})
	}
}

func TestSemanticID_Valid(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"LAF.309", true},
		{"SRDM.5", true},
		{"LA.CAT.5", true},
		{"SEM.CL.6", true},
		{"LAM.15_person", true},
		{"", false},
		{"garbage", false},
		{"LAF.0", false}, // number must be > 0
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got := ParseSemanticID(tc.input).Valid()
			if got != tc.want {
				t.Errorf("ParseSemanticID(%q).Valid() = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

func TestSemanticID_ID(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"LAF.309", "LAF.309"},
		{"LA.CAT.5", "LA.CAT.5"},
		{"SEM.CL.6", "SEM.CL.6"},
		{"LAM.15_person", "LAM.15"}, // strips system_name
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got := ParseSemanticID(tc.input).ID()
			if got != tc.want {
				t.Errorf("ID() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSemanticID_URL(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"LAF.309", "/projects/LA/fields/LAF.309"},
		{"SRDM.5", "/projects/SRD/models/SRDM.5"},
		{"SRDC.8", "/projects/SRD/collections/SRDC.8"},
		{"LA.CAT.5", "/projects/LA/categories/LA.CAT.5"},
		{"SEM.CL.6", "/projects/SEM/concept-lists/SEM.CL.6"},
		{"LAM.15_person", "/projects/LA/models/LAM.15"},
		{"", ""},
		{"garbage", ""},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got := ParseSemanticID(tc.input).URL()
			if got != tc.want {
				t.Errorf("URL() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestEntityURLForRefType(t *testing.T) {
	tests := []struct {
		name      string
		projectID string
		refType   string
		targetID  string
		want      string
	}{
		{"resource_model", "LA", "resource_model", "LAM.5", "/projects/LA/models/LAM.5"},
		{"collection_model", "LA", "collection_model", "LAC.8", "/projects/LA/collections/LAC.8"},
		{"concept_list", "LA", "concept_list", "LA.CL.8", "/projects/LA/concept-lists/LA.CL.8"},
		{"unknown_reftype", "LA", "garbage", "LAM.5", ""},
		{"empty_project", "", "resource_model", "LAM.5", ""},
		{"empty_target", "LA", "resource_model", "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := EntityURLForRefType(tc.projectID, tc.refType, tc.targetID)
			if got != tc.want {
				t.Errorf("EntityURLForRefType(%q,%q,%q) = %q; want %q", tc.projectID, tc.refType, tc.targetID, got, tc.want)
			}
		})
	}
}

func TestSemanticID_BelongsTo(t *testing.T) {
	sid := ParseSemanticID("SRDM.5")
	if !sid.BelongsTo("SRD") {
		t.Error("expected SRDM.5 to belong to SRD")
	}
	if sid.BelongsTo("LA") {
		t.Error("expected SRDM.5 to NOT belong to LA")
	}
}

func TestSemanticID_WithSystemName(t *testing.T) {
	sid := ParseSemanticID("LAM.15")
	if got := sid.WithSystemName("person"); got != "LAM.15_person" {
		t.Errorf("WithSystemName = %q, want %q", got, "LAM.15_person")
	}
	if got := sid.WithSystemName(""); got != "LAM.15" {
		t.Errorf("WithSystemName('') = %q, want %q", got, "LAM.15")
	}
}
