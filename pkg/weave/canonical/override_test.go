package canonical

import (
	"strings"
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/google/go-cmp/cmp"
)

func TestOverride_FullyPopulated(t *testing.T) {
	maxOccurs := 5
	o := &domain.FieldOverride{
		ID:                 900,
		FieldID:            "LAF.18",
		ProjectID:          "proj-1",
		EntityType:         "model",
		EntityID:           "LAM.15",
		Position:           3,
		CollectionOrder:    1,
		DisplayName:        domain.Translations{"en": "Name", "nl": "Naam"},
		Description:        domain.Translations{"en": "A name field"},
		CollectionName:     domain.Translations{"en": "Birth"},
		CategoryID:         "LA.CAT.5",
		PartOfCollectionID: "LAC.8",
		SetValue:           "fixed",
		IsRequired:         true,
		MinOccurs:          1,
		MaxOccurs:          &maxOccurs,
		IsHidden:           false,
		Visibility:         "public",
		SemanticID:         "LAF.18",
	}

	got, err := Override(o, nil)
	if err != nil {
		t.Fatalf("Override: %v", err)
	}

	output := string(got)

	// Verify key fields are present.
	for _, want := range []string{
		"field_id: LAF.18",
		"entity_type: model",
		"entity_id: LAM.15",
		"position: 3",
		"is_required: true",
		"max_occurs: 5",
		"visibility: public",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("output missing %q\ngot:\n%s", want, output)
		}
	}

	// Verify excluded fields are absent.
	for _, excluded := range []string{"project_id:", "created_at:", "updated_at:", "staging_id:"} {
		if strings.Contains(output, excluded) {
			t.Errorf("output should not contain %q\ngot:\n%s", excluded, output)
		}
	}
}

func TestOverride_WithRefs(t *testing.T) {
	o := &domain.FieldOverride{
		FieldID:    "LAF.18",
		EntityType: "model",
		EntityID:   "LAM.15",
	}

	refs := []domain.OverrideRef{
		{RefType: "model", SemanticID: "LAM.20", Position: 2},
		{RefType: "collection", SemanticID: "LAC.3", Position: 1},
		{RefType: "model", SemanticID: "LAM.10", Position: 1},
	}

	got, err := Override(o, refs)
	if err != nil {
		t.Fatalf("Override: %v", err)
	}

	output := string(got)

	// Refs should be sorted by ref_type then position.
	// collection before model (alphabetically), then model pos 1 before pos 2.
	collIdx := strings.Index(output, "LAC.3")
	model10Idx := strings.Index(output, "LAM.10")
	model20Idx := strings.Index(output, "LAM.20")

	if collIdx == -1 || model10Idx == -1 || model20Idx == -1 {
		t.Fatalf("missing ref semantic IDs in output:\n%s", output)
	}

	if collIdx > model10Idx || model10Idx > model20Idx {
		t.Errorf("refs not in expected order (collection < model.1 < model.2):\n%s", output)
	}
}

func TestOverride_Stability(t *testing.T) {
	o := &domain.FieldOverride{
		FieldID:     "LAF.1",
		EntityType:  "",
		EntityID:    "",
		DisplayName: domain.Translations{"en": "Test"},
	}

	first, err := Override(o, nil)
	if err != nil {
		t.Fatalf("first: %v", err)
	}

	second, err := Override(o, nil)
	if err != nil {
		t.Fatalf("second: %v", err)
	}

	if diff := cmp.Diff(string(first), string(second)); diff != "" {
		t.Errorf("stability mismatch (-first +second):\n%s", diff)
	}
}

func TestOverride_NilMaxOccurs(t *testing.T) {
	o := &domain.FieldOverride{
		FieldID:    "LAF.1",
		EntityType: "",
		MaxOccurs:  nil,
	}

	got, err := Override(o, nil)
	if err != nil {
		t.Fatalf("Override: %v", err)
	}

	if !strings.Contains(string(got), "max_occurs: null") {
		t.Errorf("nil max_occurs should render as null\ngot:\n%s", string(got))
	}
}
