package canonical

import (
	"strings"
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/google/go-cmp/cmp"
)

func TestModel_Populated(t *testing.T) {
	m := &domain.Model{
		Entity: domain.Entity{
			SemanticID:  "LAM.15",
			SystemName:  "person",
			UIName:      domain.Translations{"en": "Person", "nl": "Persoon"},
			Description: domain.Translations{"en": "A person entity"},
			Status:      "published",
			ProjectID:   "proj-1",
		},
		OntologyScope: domain.PathElement{
			Prefix:    "crm",
			LocalName: "E21_Person",
		},
	}

	got, err := Model(m)
	if err != nil {
		t.Fatalf("Model: %v", err)
	}

	output := string(got)

	for _, want := range []string{
		"semantic_id: LAM.15",
		"system_name: person",
		"ontology_scope: crm:E21_Person",
		"status: published",
		"deprecated: false",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("output missing %q\ngot:\n%s", want, output)
		}
	}

	if strings.Contains(output, "project_id:") {
		t.Errorf("output should not contain project_id\ngot:\n%s", output)
	}
}

func TestModel_Deprecated(t *testing.T) {
	m := &domain.Model{
		Entity: domain.Entity{
			SemanticID: "LAM.99",
			UIName:     domain.Translations{"en": "Retired"},
			Status:     "published",
			Deprecated: true,
		},
	}
	got, err := Model(m)
	if err != nil {
		t.Fatalf("Model: %v", err)
	}
	if !strings.Contains(string(got), "deprecated: true") {
		t.Errorf("expected 'deprecated: true' in output:\n%s", string(got))
	}
}

func TestModel_Stability(t *testing.T) {
	m := &domain.Model{
		Entity: domain.Entity{
			SemanticID: "LAM.1",
			UIName:     domain.Translations{"en": "Test", "nl": "Test"},
			Status:     "draft",
		},
	}

	first, err := Model(m)
	if err != nil {
		t.Fatalf("first: %v", err)
	}

	second, err := Model(m)
	if err != nil {
		t.Fatalf("second: %v", err)
	}

	if diff := cmp.Diff(string(first), string(second)); diff != "" {
		t.Errorf("stability mismatch (-first +second):\n%s", diff)
	}
}

func TestCollection_Populated(t *testing.T) {
	c := &domain.Collection{
		Entity: domain.Entity{
			SemanticID:  "LAC.8",
			SystemName:  "birth_event",
			UIName:      domain.Translations{"en": "Birth Event"},
			Description: domain.Translations{"en": "Captures birth information"},
			Status:      "published",
		},
		OntologyScope: domain.PathElement{
			Prefix:    "crm",
			LocalName: "E67_Birth",
		},
		CollectionNumber:         8,
		CanonicalCollectionOrder: 3,
	}

	got, err := Collection(c)
	if err != nil {
		t.Fatalf("Collection: %v", err)
	}

	output := string(got)

	for _, want := range []string{
		"semantic_id: LAC.8",
		"ontology_scope: crm:E67_Birth",
		"collection_number: 8",
		"canonical_collection_order: 3",
		"deprecated: false",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("output missing %q\ngot:\n%s", want, output)
		}
	}
}

func TestCollection_Deprecated(t *testing.T) {
	c := &domain.Collection{
		Entity: domain.Entity{
			SemanticID: "LAC.99",
			UIName:     domain.Translations{"en": "Retired"},
			Status:     "published",
			Deprecated: true,
		},
	}
	got, err := Collection(c)
	if err != nil {
		t.Fatalf("Collection: %v", err)
	}
	if !strings.Contains(string(got), "deprecated: true") {
		t.Errorf("expected 'deprecated: true' in output:\n%s", string(got))
	}
}

func TestCollection_Stability(t *testing.T) {
	c := &domain.Collection{
		Entity: domain.Entity{
			SemanticID: "LAC.1",
			UIName:     domain.Translations{"en": "Test"},
			Status:     "draft",
		},
	}

	first, err := Collection(c)
	if err != nil {
		t.Fatalf("first: %v", err)
	}

	second, err := Collection(c)
	if err != nil {
		t.Fatalf("second: %v", err)
	}

	if diff := cmp.Diff(string(first), string(second)); diff != "" {
		t.Errorf("stability mismatch (-first +second):\n%s", diff)
	}
}

func TestCategory_Populated(t *testing.T) {
	c := &domain.Category{
		Entity: domain.Entity{
			SemanticID:  "LA.CAT.5",
			SystemName:  "existence",
			UIName:      domain.Translations{"en": "Existence", "nl": "Bestaan"},
			Description: domain.Translations{"en": "Life events"},
			Status:      "published",
		},
		CanonicalOrder: 5,
	}

	got, err := Category(c)
	if err != nil {
		t.Fatalf("Category: %v", err)
	}

	output := string(got)

	for _, want := range []string{
		"semantic_id: LA.CAT.5",
		"system_name: existence",
		"canonical_order: 5",
		"status: published",
		"deprecated: false",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("output missing %q\ngot:\n%s", want, output)
		}
	}
}

func TestCategory_Deprecated(t *testing.T) {
	c := &domain.Category{
		Entity: domain.Entity{
			SemanticID: "LA.CAT.99",
			UIName:     domain.Translations{"en": "Retired"},
			Status:     "published",
			Deprecated: true,
		},
	}
	got, err := Category(c)
	if err != nil {
		t.Fatalf("Category: %v", err)
	}
	if !strings.Contains(string(got), "deprecated: true") {
		t.Errorf("expected 'deprecated: true' in output:\n%s", string(got))
	}
}

func TestCategory_Stability(t *testing.T) {
	c := &domain.Category{
		Entity: domain.Entity{
			SemanticID: "LA.CAT.1",
			UIName:     domain.Translations{"en": "Test"},
			Status:     "draft",
		},
		CanonicalOrder: 1,
	}

	first, err := Category(c)
	if err != nil {
		t.Fatalf("first: %v", err)
	}

	second, err := Category(c)
	if err != nil {
		t.Fatalf("second: %v", err)
	}

	if diff := cmp.Diff(string(first), string(second)); diff != "" {
		t.Errorf("stability mismatch (-first +second):\n%s", diff)
	}
}
