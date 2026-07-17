package csv

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"strings"
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/generators"
)

func TestRendererSpec(t *testing.T) {
	spec := NewRenderer().Spec()

	if spec.Format != generators.FormatCSV {
		t.Fatalf("format = %q", spec.Format)
	}
	if spec.ContentType != "text/csv; charset=utf-8" {
		t.Fatalf("content type = %q", spec.ContentType)
	}
	if spec.FileExtension != ".csv" {
		t.Fatalf("extension = %q", spec.FileExtension)
	}
}

func TestRendererRenderFields(t *testing.T) {
	snap := &generators.Snapshot{
		RootKind: generators.EntityModel,
		Project:  domain.Project{Entity: domain.Entity{ID: "LA"}},
		Model: &domain.Model{Entity: domain.Entity{
			ID:         "model-1",
			SemanticID: "LAM.1",
		}},
		Fields: []generators.FieldNode{
			{
				Field: domain.ResolvedField{
					ID:                 "field-1",
					SemanticID:         "LAF.1",
					SystemName:         "person_name",
					DisplayName:        domain.Translations{"en": "Person name"},
					Description:        domain.Translations{"en": "Name"},
					Position:           7,
					CategoryID:         "identity",
					PartOfCollectionID: "collection-1",
					CollectionOrder:    3,
					ExpectedValueType:  "literal",
					SetValue:           "fixed",
					IsRequired:         true,
					PathElements: []domain.PathElement{
						pe("property", "crm", "P1_is_identified_by", 0),
						pe("class", "crm", "E41_Appellation", 1),
						pe("property", "crm", "P190_has_symbolic_content", 2),
						{Type: "literal", Prefix: "rdf", LocalName: "literal", URI: "rdf:literal", Position: 3},
					},
				},
				RelativePath: "lam-1/category/identity/collection/birth/field/laf-1",
				Scope:        pe("class", "crm", "E21_Person", 0),
			},
		},
	}

	var buf bytes.Buffer
	if err := NewRenderer().Render(context.Background(), snap, &buf); err != nil {
		t.Fatalf("Render() error: %v", err)
	}

	records := readCSV(t, buf.String())
	if len(records) != 2 {
		t.Fatalf("records = %d, want 2", len(records))
	}
	if got := strings.Join(records[0], ","); got != strings.Join(header, ",") {
		t.Fatalf("header = %q", got)
	}

	row := records[1]
	assertCell(t, row, "root_kind", "model")
	assertCell(t, row, "project_id", "LA")
	assertCell(t, row, "root_semantic_id", "LAM.1")
	assertCell(t, row, "field_semantic_id", "LAF.1")
	assertCell(t, row, "ontology_scope", "crm:E21_Person")
	assertCell(t, row, "ontology_path", "->crm:P1_is_identified_by->crm:E41_Appellation->crm:P190_has_symbolic_content->rdf:literal")
	assertCell(t, row, "expected_value_type", "literal")
	assertCell(t, row, "set_value", "fixed")
	assertCell(t, row, "is_required", "true")

	var label map[string]string
	if err := json.Unmarshal([]byte(row[indexOf("field_label")]), &label); err != nil {
		t.Fatalf("field_label is not JSON: %v", err)
	}
	if label["en"] != "Person name" {
		t.Fatalf("label = %#v", label)
	}

	var scope domain.PathElement
	if err := json.Unmarshal([]byte(row[indexOf("ontology_scope_element")]), &scope); err != nil {
		t.Fatalf("ontology_scope_element is not JSON: %v", err)
	}
	if scope.LocalName != "E21_Person" {
		t.Fatalf("scope = %#v", scope)
	}

	var elements []domain.PathElement
	if err := json.Unmarshal([]byte(row[indexOf("ontology_path_elements")]), &elements); err != nil {
		t.Fatalf("ontology_path_elements is not JSON: %v", err)
	}
	if len(elements) != 4 {
		t.Fatalf("path elements = %d", len(elements))
	}
}

func TestRendererRejectsSnapshotErrors(t *testing.T) {
	snap := &generators.Snapshot{
		Report: generators.Report{
			Errors: []generators.Diagnostic{{Code: "legacy_inverse_path"}},
		},
	}

	var buf bytes.Buffer
	if err := NewRenderer().Render(context.Background(), snap, &buf); err == nil {
		t.Fatal("Render() error = nil, want error")
	}
}

func readCSV(t *testing.T, s string) [][]string {
	t.Helper()
	records, err := csv.NewReader(strings.NewReader(s)).ReadAll()
	if err != nil {
		t.Fatalf("read csv: %v", err)
	}
	return records
}

func assertCell(t *testing.T, row []string, column string, want string) {
	t.Helper()
	if got := row[indexOf(column)]; got != want {
		t.Fatalf("%s = %q, want %q", column, got, want)
	}
}

func indexOf(column string) int {
	for i, name := range header {
		if name == column {
			return i
		}
	}
	panic("unknown column: " + column)
}

func pe(kind, prefix, localName string, position int) domain.PathElement {
	return domain.PathElement{
		Type:      kind,
		URI:       prefix + ":" + localName,
		Prefix:    prefix,
		LocalName: localName,
		Position:  position,
	}
}
