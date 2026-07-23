package canonical

import (
	"strings"
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/google/go-cmp/cmp"
)

func TestField_Populated(t *testing.T) {
	f := &domain.Field{
		Entity: domain.Entity{
			ID:          "LAF.18",
			SemanticID:  "LAF.18",
			SystemName:  "person_name",
			UIName:      domain.Translations{"en": "Person name"},
			Description: domain.Translations{"en": "The name of a person"},
			Status:      "published",
			ProjectID:   "proj-1",
		},
		OntologyScope: domain.PathElement{
			Prefix:    "crm",
			LocalName: "E21_Person",
		},
		PathElements: []domain.PathElement{
			{Prefix: "crm", LocalName: "E21_Person"},
			{Prefix: "crm", LocalName: "P1_is_identified_by"},
			{Prefix: "crm", LocalName: "E33_E41_Linguistic_Appellation"},
		},
		ExpectedValueType: "literal",
	}

	got, err := Field(f)
	if err != nil {
		t.Fatalf("Field: %v", err)
	}

	output := string(got)

	for _, want := range []string{
		"semantic_id: LAF.18",
		"system_name: person_name",
		"local_name: E21_Person",
		"expected_value_type: literal",
		"status: published",
		"deprecated: false",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("output missing %q\ngot:\n%s", want, output)
		}
	}

	// Verify excluded fields.
	for _, excluded := range []string{"project_id:", "created_at:", "updated_at:"} {
		if strings.Contains(output, excluded) {
			t.Errorf("output should not contain %q\ngot:\n%s", excluded, output)
		}
	}
}

func TestField_Deprecated(t *testing.T) {
	f := &domain.Field{
		Entity: domain.Entity{
			SemanticID: "LAF.99",
			UIName:     domain.Translations{"en": "Retired"},
			Status:     "published",
			Deprecated: true,
		},
	}
	got, err := Field(f)
	if err != nil {
		t.Fatalf("Field: %v", err)
	}
	if !strings.Contains(string(got), "deprecated: true") {
		t.Errorf("expected 'deprecated: true' in output:\n%s", string(got))
	}
}

func TestField_EmptyOptionals(t *testing.T) {
	f := &domain.Field{
		Entity: domain.Entity{
			SemanticID: "LAF.1",
			Status:     "draft",
		},
	}

	got, err := Field(f)
	if err != nil {
		t.Fatalf("Field: %v", err)
	}

	if len(got) == 0 {
		t.Error("expected non-empty output")
	}

	// Stability check.
	second, err := Field(f)
	if err != nil {
		t.Fatalf("second Field: %v", err)
	}

	if diff := cmp.Diff(string(got), string(second)); diff != "" {
		t.Errorf("stability mismatch (-first +second):\n%s", diff)
	}
}
