package csvexport

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/google/go-cmp/cmp"
)

func strPtr(s string) *string { return &s }
func intPtr(i int) *int       { return &i }

// parseCSVRow splits a CSV line respecting quoted fields.
func parseCSVRow(t *testing.T, line string) []string {
	t.Helper()
	r := csv.NewReader(strings.NewReader(line))
	record, err := r.Read()
	if err != nil {
		t.Fatalf("failed to parse CSV row: %v", err)
	}
	return record
}

func TestWriteFields(t *testing.T) {
	t.Run("one record produces header and data row", func(t *testing.T) {
		fields := []*domain.Field{
			{
				Entity: domain.Entity{
					SemanticID:  "LAF.309",
					SystemName:  "person_name",
					UIName:      domain.Translations{"en": "Person Name", "nl": "Persoonsnaam"},
					Description: domain.Translations{"en": "Name of a person"},
					Status:      "published",
					ProjectID:   "proj-001",
				},
				OntologyScope: domain.PathElement{
					Prefix:    "crm",
					LocalName: "E21_Person",
				},
				PathElements: []domain.PathElement{
					{Prefix: "crm", LocalName: "E21_Person"},
					{Prefix: "crm", LocalName: "P1_is_identified_by"},
					{Prefix: "crm", LocalName: "E41_Appellation"},
				},
				ExpectedValueType: "literal",
			},
		}
		baseMeta := map[string]FieldOverrideMeta{
			"": {SetValue: "fixed-value", CategoryID: "cat-001"},
		}

		var buf bytes.Buffer
		if err := WriteFields(&buf, fields, nil, baseMeta); err != nil {
			t.Fatalf("WriteFields() error: %v", err)
		}

		lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
		if len(lines) != 2 {
			t.Fatalf("expected 2 lines (header + 1 row), got %d", len(lines))
		}

		wantHeader := "semantic_id,system_name,ui_name,description,ontology_path,ontology_scope,expected_value_type,set_value,category_id,status,_expected_refs"
		if diff := cmp.Diff(wantHeader, lines[0]); diff != "" {
			t.Errorf("header mismatch (-want +got):\n%s", diff)
		}

		row := parseCSVRow(t, lines[1])
		if diff := cmp.Diff("->crm:E21_Person->crm:P1_is_identified_by->crm:E41_Appellation", row[4]); diff != "" {
			t.Errorf("ontology_path mismatch (-want +got):\n%s", diff)
		}
		if diff := cmp.Diff("crm:E21_Person", row[5]); diff != "" {
			t.Errorf("ontology_scope mismatch (-want +got):\n%s", diff)
		}
		if diff := cmp.Diff("fixed-value", row[7]); diff != "" {
			t.Errorf("set_value mismatch (-want +got):\n%s", diff)
		}
		if diff := cmp.Diff("cat-001", row[8]); diff != "" {
			t.Errorf("category_id mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("expected_refs column populated from map", func(t *testing.T) {
		fields := []*domain.Field{
			{Entity: domain.Entity{ID: "f1", SemanticID: "LAF.18"}},
			{Entity: domain.Entity{ID: "f2", SemanticID: "LAF.22"}},
		}
		refs := map[string]string{
			"f1": "LAM.9,LAM.4",
			// f2 has no refs
		}

		var buf bytes.Buffer
		if err := WriteFields(&buf, fields, refs, nil); err != nil {
			t.Fatalf("WriteFields() error: %v", err)
		}

		lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
		row0 := parseCSVRow(t, lines[1])
		if diff := cmp.Diff("LAM.9,LAM.4", row0[10]); diff != "" {
			t.Errorf("_expected_refs mismatch (-want +got):\n%s", diff)
		}

		row1 := parseCSVRow(t, lines[2])
		if row1[10] != "" {
			t.Errorf("expected empty _expected_refs for f2, got %q", row1[10])
		}
	})

	t.Run("translations cells are valid JSON", func(t *testing.T) {
		fields := []*domain.Field{
			{
				Entity: domain.Entity{
					SemanticID:  "LAF.1",
					UIName:      domain.Translations{"en": "Test", "nl": "Proef"},
					Description: domain.Translations{"en": "A test field"},
				},
			},
		}

		var buf bytes.Buffer
		if err := WriteFields(&buf, fields, nil, nil); err != nil {
			t.Fatalf("WriteFields() error: %v", err)
		}

		lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
		row := parseCSVRow(t, lines[1])

		var uiName map[string]string
		if err := json.Unmarshal([]byte(row[2]), &uiName); err != nil {
			t.Errorf("ui_name is not valid JSON: %q, err: %v", row[2], err)
		}
		want := map[string]string{"en": "Test", "nl": "Proef"}
		if diff := cmp.Diff(want, uiName); diff != "" {
			t.Errorf("ui_name mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("nil input writes header only", func(t *testing.T) {
		var buf bytes.Buffer
		if err := WriteFields(&buf, nil, nil, nil); err != nil {
			t.Fatalf("WriteFields() error: %v", err)
		}

		lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
		if len(lines) != 1 {
			t.Fatalf("expected 1 line (header only), got %d", len(lines))
		}
	})

	t.Run("nil optional fields produce empty strings", func(t *testing.T) {
		fields := []*domain.Field{
			{Entity: domain.Entity{SemanticID: "LAF.2"}},
		}

		var buf bytes.Buffer
		if err := WriteFields(&buf, fields, nil, nil); err != nil {
			t.Fatalf("WriteFields() error: %v", err)
		}

		lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
		row := parseCSVRow(t, lines[1])
		if row[7] != "" {
			t.Errorf("expected empty set_value, got %q", row[7])
		}
		if row[8] != "" {
			t.Errorf("expected empty category_id, got %q", row[8])
		}
	})
}

func TestWriteModels(t *testing.T) {
	t.Run("one record produces header and data row", func(t *testing.T) {
		models := []*domain.Model{
			{
				Entity: domain.Entity{
					SemanticID:  "LAM.15",
					SystemName:  "person",
					UIName:      domain.Translations{"en": "Person", "nl": "Persoon"},
					Description: domain.Translations{"en": "A person entity"},
					Status:      "published",
				},
				OntologyScope: domain.PathElement{
					Prefix:    "crm",
					LocalName: "E21_Person",
				},
			},
		}

		var buf bytes.Buffer
		if err := WriteModels(&buf, models); err != nil {
			t.Fatalf("WriteModels() error: %v", err)
		}

		lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
		if len(lines) != 2 {
			t.Fatalf("expected 2 lines, got %d", len(lines))
		}

		wantHeader := "semantic_id,system_name,ui_name,description,ontology_scope,status"
		if diff := cmp.Diff(wantHeader, lines[0]); diff != "" {
			t.Errorf("header mismatch (-want +got):\n%s", diff)
		}

		row := parseCSVRow(t, lines[1])
		if diff := cmp.Diff("LAM.15", row[0]); diff != "" {
			t.Errorf("semantic_id mismatch (-want +got):\n%s", diff)
		}
		if diff := cmp.Diff("crm:E21_Person", row[4]); diff != "" {
			t.Errorf("ontology_scope mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("nil input writes header only", func(t *testing.T) {
		var buf bytes.Buffer
		if err := WriteModels(&buf, nil); err != nil {
			t.Fatalf("WriteModels() error: %v", err)
		}

		lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
		if len(lines) != 1 {
			t.Fatalf("expected 1 line (header only), got %d", len(lines))
		}
	})
}

func TestWriteCollections(t *testing.T) {
	t.Run("one record produces header and data row", func(t *testing.T) {
		collections := []*domain.Collection{
			{
				Entity: domain.Entity{
					SemanticID:  "LAC.8",
					SystemName:  "birth_event",
					UIName:      domain.Translations{"en": "Birth Event"},
					Description: domain.Translations{"en": "Birth event collection"},
					Status:      "published",
				},
				OntologyScope: domain.PathElement{
					Prefix:    "crm",
					LocalName: "E67_Birth",
				},
				CanonicalCollectionOrder: 3,
			},
		}

		var buf bytes.Buffer
		if err := WriteCollections(&buf, collections); err != nil {
			t.Fatalf("WriteCollections() error: %v", err)
		}

		lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
		if len(lines) != 2 {
			t.Fatalf("expected 2 lines, got %d", len(lines))
		}

		wantHeader := "semantic_id,system_name,ui_name,description,ontology_scope,status,collection_number,canonical_collection_order"
		if diff := cmp.Diff(wantHeader, lines[0]); diff != "" {
			t.Errorf("header mismatch (-want +got):\n%s", diff)
		}

		row := parseCSVRow(t, lines[1])
		if diff := cmp.Diff("crm:E67_Birth", row[4]); diff != "" {
			t.Errorf("ontology_scope mismatch (-want +got):\n%s", diff)
		}
		if diff := cmp.Diff("3", row[7]); diff != "" {
			t.Errorf("canonical_collection_order mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("nil input writes header only", func(t *testing.T) {
		var buf bytes.Buffer
		if err := WriteCollections(&buf, nil); err != nil {
			t.Fatalf("WriteCollections() error: %v", err)
		}

		lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
		if len(lines) != 1 {
			t.Fatalf("expected 1 line (header only), got %d", len(lines))
		}
	})
}

func TestWriteCategories(t *testing.T) {
	t.Run("one record produces header and data row", func(t *testing.T) {
		categories := []*domain.Category{
			{
				Entity: domain.Entity{
					SemanticID:  "LACAT.5",
					SystemName:  "identification",
					UIName:      domain.Translations{"en": "Identification", "nl": "Identificatie"},
					Description: domain.Translations{"en": "Identification category"},
				},
				CanonicalOrder: 2,
			},
		}

		var buf bytes.Buffer
		if err := WriteCategories(&buf, categories); err != nil {
			t.Fatalf("WriteCategories() error: %v", err)
		}

		lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
		if len(lines) != 2 {
			t.Fatalf("expected 2 lines, got %d", len(lines))
		}

		wantHeader := "semantic_id,system_name,ui_name,description,canonical_order,status"
		if diff := cmp.Diff(wantHeader, lines[0]); diff != "" {
			t.Errorf("header mismatch (-want +got):\n%s", diff)
		}

		row := parseCSVRow(t, lines[1])
		if diff := cmp.Diff("LACAT.5", row[0]); diff != "" {
			t.Errorf("semantic_id mismatch (-want +got):\n%s", diff)
		}
		if diff := cmp.Diff("2", row[4]); diff != "" {
			t.Errorf("canonical_order mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("nil input writes header only", func(t *testing.T) {
		var buf bytes.Buffer
		if err := WriteCategories(&buf, nil); err != nil {
			t.Fatalf("WriteCategories() error: %v", err)
		}

		lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
		if len(lines) != 1 {
			t.Fatalf("expected 1 line (header only), got %d", len(lines))
		}
	})
}

func TestWriteModelFieldOverrides(t *testing.T) {
	t.Run("one record produces header and data row", func(t *testing.T) {
		overrides := []OverrideRow{
			{
				EntitySemanticID:  "LAM.15",
				FieldSemanticID:   "LAF.309",
				Position:          3,
				DisplayName:       domain.Translations{"en": "Custom Name"},
				Description:       domain.Translations{"en": "Override desc"},
				CategoryID:        "cat-001",
				ExpectedValueType: "resource",
				SetValue:          "fixed",
				IsRequired:        true,
				MinOccurs:         1,
				MaxOccurs:         intPtr(5),
				IsHidden:          false,
				Refs:              "LAM.15,LAC.8",
			},
		}

		var buf bytes.Buffer
		if err := WriteModelFieldOverrides(&buf, overrides); err != nil {
			t.Fatalf("WriteModelFieldOverrides() error: %v", err)
		}

		lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
		if len(lines) != 2 {
			t.Fatalf("expected 2 lines, got %d", len(lines))
		}

		wantHeader := "model_semantic_id,field_semantic_id,position,collection_order,display_name,description,collection_name,category_id,expected_value_type,set_value,is_required,min_occurs,max_occurs,is_hidden,visibility,refs"
		if diff := cmp.Diff(wantHeader, lines[0]); diff != "" {
			t.Errorf("header mismatch (-want +got):\n%s", diff)
		}

		row := parseCSVRow(t, lines[1])
		if diff := cmp.Diff("LAM.15", row[0]); diff != "" {
			t.Errorf("model_semantic_id mismatch (-want +got):\n%s", diff)
		}
		if diff := cmp.Diff("true", row[10]); diff != "" {
			t.Errorf("is_required mismatch (-want +got):\n%s", diff)
		}
		if diff := cmp.Diff("5", row[12]); diff != "" {
			t.Errorf("max_occurs mismatch (-want +got):\n%s", diff)
		}
		if diff := cmp.Diff("LAM.15,LAC.8", row[15]); diff != "" {
			t.Errorf("refs mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("nil max_occurs produces empty string", func(t *testing.T) {
		overrides := []OverrideRow{
			{FieldSemanticID: "LAF.1", MaxOccurs: nil},
		}

		var buf bytes.Buffer
		if err := WriteModelFieldOverrides(&buf, overrides); err != nil {
			t.Fatalf("WriteModelFieldOverrides() error: %v", err)
		}

		lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
		row := parseCSVRow(t, lines[1])
		if row[12] != "" {
			t.Errorf("expected empty max_occurs, got %q", row[12])
		}
	})

	t.Run("nil input writes header only", func(t *testing.T) {
		var buf bytes.Buffer
		if err := WriteModelFieldOverrides(&buf, nil); err != nil {
			t.Fatalf("WriteModelFieldOverrides() error: %v", err)
		}

		lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
		if len(lines) != 1 {
			t.Fatalf("expected 1 line (header only), got %d", len(lines))
		}
	})
}

func TestWriteCollectionFieldOverrides(t *testing.T) {
	t.Run("one record produces header and data row", func(t *testing.T) {
		overrides := []OverrideRow{
			{
				EntitySemanticID:   "LAC.8",
				FieldSemanticID:    "LAF.309",
				Position:           2,
				DisplayName:        domain.Translations{"en": "Birth Name"},
				Description:        domain.Translations{"en": "Name at birth"},
				CategoryID:         "cat-002",
				PartOfCollectionID: "LAC.3",
				Refs:               "LAM.15",
			},
		}

		var buf bytes.Buffer
		if err := WriteCollectionFieldOverrides(&buf, overrides); err != nil {
			t.Fatalf("WriteCollectionFieldOverrides() error: %v", err)
		}

		lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
		if len(lines) != 2 {
			t.Fatalf("expected 2 lines, got %d", len(lines))
		}

		wantHeader := "collection_semantic_id,field_semantic_id,position,display_name,description,category_id,part_of_collection_id,expected_value_type,set_value,visibility,refs"
		if diff := cmp.Diff(wantHeader, lines[0]); diff != "" {
			t.Errorf("header mismatch (-want +got):\n%s", diff)
		}

		row := parseCSVRow(t, lines[1])
		if diff := cmp.Diff("LAC.8", row[0]); diff != "" {
			t.Errorf("collection_semantic_id mismatch (-want +got):\n%s", diff)
		}
		if diff := cmp.Diff("LAC.3", row[6]); diff != "" {
			t.Errorf("part_of_collection_id mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("nil input writes header only", func(t *testing.T) {
		var buf bytes.Buffer
		if err := WriteCollectionFieldOverrides(&buf, nil); err != nil {
			t.Fatalf("WriteCollectionFieldOverrides() error: %v", err)
		}

		lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
		if len(lines) != 1 {
			t.Fatalf("expected 1 line (header only), got %d", len(lines))
		}
	})
}

func TestWriteBaseFieldOverrides(t *testing.T) {
	t.Run("one record produces header and data row", func(t *testing.T) {
		overrides := []OverrideRow{
			{
				FieldSemanticID:   "LAF.309",
				Position:          1,
				DisplayName:       domain.Translations{"en": "Person Name"},
				Description:       domain.Translations{"en": "Base description"},
				CategoryID:        "cat-001",
				ExpectedValueType: "literal",
				SetValue:          "",
				IsRequired:        false,
				Refs:              "LAM.15",
			},
		}

		var buf bytes.Buffer
		if err := WriteBaseFieldOverrides(&buf, overrides); err != nil {
			t.Fatalf("WriteBaseFieldOverrides() error: %v", err)
		}

		lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
		if len(lines) != 2 {
			t.Fatalf("expected 2 lines, got %d", len(lines))
		}

		wantHeader := "field_semantic_id,position,display_name,description,category_id,expected_value_type,set_value,is_required,visibility,refs"
		if diff := cmp.Diff(wantHeader, lines[0]); diff != "" {
			t.Errorf("header mismatch (-want +got):\n%s", diff)
		}

		row := parseCSVRow(t, lines[1])
		if diff := cmp.Diff("LAF.309", row[0]); diff != "" {
			t.Errorf("field_semantic_id mismatch (-want +got):\n%s", diff)
		}
		if diff := cmp.Diff("false", row[7]); diff != "" {
			t.Errorf("is_required mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("nil input writes header only", func(t *testing.T) {
		var buf bytes.Buffer
		if err := WriteBaseFieldOverrides(&buf, nil); err != nil {
			t.Fatalf("WriteBaseFieldOverrides() error: %v", err)
		}

		lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
		if len(lines) != 1 {
			t.Fatalf("expected 1 line (header only), got %d", len(lines))
		}
	})
}

func TestWriteOntologies(t *testing.T) {
	t.Run("one record produces header and data row", func(t *testing.T) {
		addedAt := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)
		ontologies := []OntologyRow{
			{
				OntologyName:  "CIDOC-CRM",
				OntologyLabel: "CIDOC Conceptual Reference Model",
				Version:       "7.1.3",
				OntologyURI:   "http://www.cidoc-crm.org/cidoc-crm/",
				VersionIRI:    "http://www.cidoc-crm.org/cidoc-crm/7.1.3",
				IsPrimary:     true,
				IsActive:      true,
				ClassCount:    84,
				PropertyCount: 168,
				AddedAt:       addedAt,
				UsageNotes:    "Primary ontology for cultural heritage",
			},
		}

		var buf bytes.Buffer
		if err := WriteOntologies(&buf, ontologies); err != nil {
			t.Fatalf("WriteOntologies() error: %v", err)
		}

		lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
		if len(lines) != 2 {
			t.Fatalf("expected 2 lines, got %d", len(lines))
		}

		wantHeader := "ontology_name,ontology_label,version,ontology_uri,version_iri,is_primary,is_active,class_count,property_count,added_at,usage_notes"
		if diff := cmp.Diff(wantHeader, lines[0]); diff != "" {
			t.Errorf("header mismatch (-want +got):\n%s", diff)
		}

		row := parseCSVRow(t, lines[1])
		if diff := cmp.Diff("CIDOC-CRM", row[0]); diff != "" {
			t.Errorf("ontology_name mismatch (-want +got):\n%s", diff)
		}
		if diff := cmp.Diff("true", row[5]); diff != "" {
			t.Errorf("is_primary mismatch (-want +got):\n%s", diff)
		}
		if diff := cmp.Diff("84", row[7]); diff != "" {
			t.Errorf("class_count mismatch (-want +got):\n%s", diff)
		}
		if diff := cmp.Diff("2025-01-15T10:30:00Z", row[9]); diff != "" {
			t.Errorf("added_at mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("nil input writes header only", func(t *testing.T) {
		var buf bytes.Buffer
		if err := WriteOntologies(&buf, nil); err != nil {
			t.Fatalf("WriteOntologies() error: %v", err)
		}

		lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
		if len(lines) != 1 {
			t.Fatalf("expected 1 line (header only), got %d", len(lines))
		}
	})
}
