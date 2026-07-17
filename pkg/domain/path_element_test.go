package domain_test

import (
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/google/go-cmp/cmp"
)

func TestPathElementPrefixedName(t *testing.T) {
	tests := []struct {
		name string
		pe   domain.PathElement
		want string
	}{
		{
			name: "simple class",
			pe:   domain.PathElement{Prefix: "crm", LocalName: "E21_Person"},
			want: "crm:E21_Person",
		},
		{
			name: "no prefix",
			pe:   domain.PathElement{LocalName: "E21_Person"},
			want: "E21_Person",
		},
		{
			name: "multi-typed cross-ontology",
			pe: domain.PathElement{
				Prefix: "crm", LocalName: "E29_Design_or_Procedure",
				AdditionalTypes: []domain.TypeRef{
					{Prefix: "crmdig", LocalName: "D1_Digital_Object"},
				},
			},
			want: "crm:E29_Design_or_Procedure/crmdig:D1_Digital_Object",
		},
		{
			name: "multi-typed same ontology",
			pe: domain.PathElement{
				Prefix: "crm", LocalName: "E6_Destruction",
				AdditionalTypes: []domain.TypeRef{
					{Prefix: "crm", LocalName: "E7_Activity"},
				},
			},
			want: "crm:E6_Destruction/crm:E7_Activity",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.pe.PrefixedName()
			if got != tt.want {
				t.Errorf("PrefixedName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPathElementAllTypes(t *testing.T) {
	pe := domain.PathElement{
		Prefix: "crm", LocalName: "E29_Design_or_Procedure",
		URI: "crm:E29_Design_or_Procedure", ClassCode: "E29",
		AdditionalTypes: []domain.TypeRef{
			{Prefix: "crmdig", LocalName: "D1_Digital_Object", URI: "crmdig:D1_Digital_Object", ClassCode: "D1"},
		},
	}

	got := pe.AllTypes()
	want := []domain.TypeRef{
		{Prefix: "crm", LocalName: "E29_Design_or_Procedure", URI: "crm:E29_Design_or_Procedure", ClassCode: "E29"},
		{Prefix: "crmdig", LocalName: "D1_Digital_Object", URI: "crmdig:D1_Digital_Object", ClassCode: "D1"},
	}

	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("AllTypes() mismatch (-want +got):\n%s", diff)
	}
}

func TestOntologyPathWithMultiType(t *testing.T) {
	f := domain.Field{
		PathElements: []domain.PathElement{
			{Prefix: "crmdig", LocalName: "L11i_was_output_of", Type: "property", Position: 0},
			{Prefix: "crmdig", LocalName: "D7_Digital_Machine_Event", Type: "class", Position: 1},
			{Prefix: "crm", LocalName: "P33_used_specific_technique", Type: "property", Position: 2},
			{
				Prefix: "crm", LocalName: "E29_Design_or_Procedure", Type: "class", Position: 3,
				AdditionalTypes: []domain.TypeRef{
					{Prefix: "crmdig", LocalName: "D1_Digital_Object"},
				},
			},
		},
	}

	got := f.OntologyPath()
	want := "->crmdig:L11i_was_output_of->crmdig:D7_Digital_Machine_Event->crm:P33_used_specific_technique->crm:E29_Design_or_Procedure/crmdig:D1_Digital_Object"
	if got != want {
		t.Errorf("OntologyPath() =\n  %q\nwant:\n  %q", got, want)
	}
}
