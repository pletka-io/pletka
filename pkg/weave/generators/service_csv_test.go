package generators_test

import (
	"bytes"
	"context"
	"encoding/csv"
	"strings"
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/generators"
	weavecsv "github.com/pletka-io/pletka/pkg/weave/generators/csv"
)

func TestServiceGenerateCollectionCSV(t *testing.T) {
	registry, err := generators.NewRegistry(weavecsv.NewRenderer())
	if err != nil {
		t.Fatalf("NewRegistry() error: %v", err)
	}
	project := domain.Project{Entity: domain.Entity{ID: "LA"}}
	collection := domain.Collection{
		Entity:        domain.Entity{ID: "collection-1", SemanticID: "LAC.1", SystemName: "birth"},
		OntologyScope: pe("class", "crm", "E67_Birth", 0),
	}
	projectReader := stubProjectReader{project: project}
	namespaceReader := stubNamespaceReader{
		namespaces: []*domain.NamespaceBinding{
			{Prefix: "crm", Namespace: "http://www.cidoc-crm.org/cidoc-crm/", Weight: 10},
		},
	}
	collectionReader := stubCollectionReader{
		collection: collection,
		collectionFields: []domain.ResolvedField{
			resolvedField("field-2", "LAF.2", "late", 20),
			resolvedField("field-1", "LAF.1", "early", 10),
		},
	}
	svc := generators.NewService(&projectReader, nil, &collectionReader, nil, &namespaceReader, registry)

	var buf bytes.Buffer
	if err := svc.GenerateCollection(context.Background(), "LA", "collection-1", generators.FormatCSV, &buf, generators.Options{}); err != nil {
		t.Fatalf("GenerateCollection() error: %v", err)
	}

	records := readCSV(t, buf.String())
	if len(records) != 3 {
		t.Fatalf("records = %d, want 3", len(records))
	}
	if got := cell(records[1], "field_semantic_id"); got != "LAF.1" {
		t.Fatalf("first field = %q, want LAF.1", got)
	}
	if got := cell(records[2], "field_semantic_id"); got != "LAF.2" {
		t.Fatalf("second field = %q, want LAF.2", got)
	}
	if got := cell(records[1], "root_semantic_id"); got != "LAC.1" {
		t.Fatalf("root semantic id = %q", got)
	}
	if got := cell(records[1], "ontology_scope"); got != "crm:E67_Birth" {
		t.Fatalf("ontology scope = %q", got)
	}
}

func TestServiceGenerateModelCSV(t *testing.T) {
	registry, err := generators.NewRegistry(weavecsv.NewRenderer())
	if err != nil {
		t.Fatalf("NewRegistry() error: %v", err)
	}
	project := domain.Project{Entity: domain.Entity{ID: "LA"}}
	model := domain.Model{
		Entity:        domain.Entity{ID: "model-1", SemanticID: "LAM.1", SystemName: "person"},
		OntologyScope: pe("class", "crm", "E21_Person", 0),
	}
	modelReader := stubModelReader{
		model: model,
		view: domain.ModelView{
			ModelID:   model.ID,
			ProjectID: project.ID,
			Categories: []domain.CategoryGroup{
				{
					ID:       "identity",
					Name:     domain.Translations{"en": "Identity"},
					Position: 1,
					Collections: []domain.CollectionGroup{
						{
							ID:       "__direct__",
							Name:     domain.Translations{"en": "Direct fields"},
							Position: 1,
							Fields: []domain.ResolvedField{
								resolvedField("field-1", "LAF.1", "name", 1),
							},
						},
					},
				},
			},
		},
	}
	projectReader := stubProjectReader{project: project}
	namespaceReader := stubNamespaceReader{}
	svc := generators.NewService(&projectReader, &modelReader, &stubCollectionReader{}, nil, &namespaceReader, registry)

	var buf bytes.Buffer
	if err := svc.GenerateModel(context.Background(), "LA", "model-1", generators.FormatCSV, &buf, generators.Options{}); err != nil {
		t.Fatalf("GenerateModel() error: %v", err)
	}

	records := readCSV(t, buf.String())
	if len(records) != 2 {
		t.Fatalf("records = %d, want 2", len(records))
	}
	if got := cell(records[1], "root_kind"); got != "model" {
		t.Fatalf("root kind = %q", got)
	}
	if got := cell(records[1], "root_semantic_id"); got != "LAM.1" {
		t.Fatalf("root semantic id = %q", got)
	}
	if got := cell(records[1], "field_semantic_id"); got != "LAF.1" {
		t.Fatalf("field = %q", got)
	}
}

func TestServiceGenerateFieldCSV(t *testing.T) {
	registry, err := generators.NewRegistry(weavecsv.NewRenderer())
	if err != nil {
		t.Fatalf("NewRegistry() error: %v", err)
	}
	project := domain.Project{Entity: domain.Entity{ID: "LA"}}
	field := domain.Field{
		Entity: domain.Entity{
			ID:         "field-1",
			SemanticID: "LAF.1",
			SystemName: "name",
			UIName:     domain.Translations{"en": "Name"},
			ProjectID:  "LA",
		},
		OntologyScope:     pe("class", "crm", "E21_Person", 0),
		PathElements:      []domain.PathElement{pe("property", "crm", "P1_is_identified_by", 0)},
		ExpectedValueType: "literal",
	}
	projectReader := stubProjectReader{project: project}
	namespaceReader := stubNamespaceReader{}
	fieldReader := stubFieldReader{
		field: field,
		resolved: domain.ResolvedField{
			ID:                field.ID,
			SemanticID:        field.SemanticID,
			SystemName:        field.SystemName,
			DisplayName:       field.UIName,
			ExpectedValueType: field.ExpectedValueType,
			PathElements:      field.PathElements,
			SetValue:          "fixed",
		},
	}
	svc := generators.NewService(&projectReader, nil, nil, &fieldReader, &namespaceReader, registry)

	var buf bytes.Buffer
	if err := svc.GenerateField(context.Background(), "LA", "field-1", generators.FormatCSV, &buf, generators.Options{}); err != nil {
		t.Fatalf("GenerateField() error: %v", err)
	}

	records := readCSV(t, buf.String())
	if len(records) != 2 {
		t.Fatalf("records = %d, want 2", len(records))
	}
	if got := cell(records[1], "root_kind"); got != "field" {
		t.Fatalf("root kind = %q", got)
	}
	if got := cell(records[1], "set_value"); got != "fixed" {
		t.Fatalf("set value = %q", got)
	}
}

type stubProjectReader struct {
	project    domain.Project
	ontologies []domain.ResolvedOntologyVersion
}

func (f *stubProjectReader) GetByID(ctx context.Context, id string) (*domain.Project, error) {
	return &f.project, ctx.Err()
}

func (f *stubProjectReader) ResolvedOntologyVersions(ctx context.Context, projectID string, opts domain.ResolvedOntologyVersionOpts) ([]domain.ResolvedOntologyVersion, error) {
	return f.ontologies, ctx.Err()
}

type stubCollectionReader struct {
	collection       domain.Collection
	collectionFields []domain.ResolvedField
}

func (f *stubCollectionReader) Get(ctx context.Context, projectID, id string) (*domain.Collection, error) {
	return &f.collection, ctx.Err()
}

func (f *stubCollectionReader) View(ctx context.Context, projectID, id string) ([]domain.ResolvedField, error) {
	return f.collectionFields, ctx.Err()
}

type stubNamespaceReader struct {
	namespaces []*domain.NamespaceBinding
}

func (f *stubNamespaceReader) ListForProject(ctx context.Context, projectID string) ([]*domain.NamespaceBinding, error) {
	return f.namespaces, ctx.Err()
}

type stubModelReader struct {
	model domain.Model
	view  domain.ModelView
}

func (f *stubModelReader) Get(ctx context.Context, projectID, id string) (*domain.Model, error) {
	return &f.model, ctx.Err()
}

func (f *stubModelReader) View(ctx context.Context, projectID, id string) (*domain.ModelView, error) {
	return &f.view, ctx.Err()
}

type stubFieldReader struct {
	field    domain.Field
	resolved domain.ResolvedField
}

func (f *stubFieldReader) Get(ctx context.Context, projectID, id string) (*domain.Field, error) {
	return &f.field, ctx.Err()
}

func (f *stubFieldReader) Resolved(ctx context.Context, projectID, id string) (*domain.ResolvedField, error) {
	return &f.resolved, ctx.Err()
}

func resolvedField(id, semanticID, name string, position int) domain.ResolvedField {
	return domain.ResolvedField{
		ID:          id,
		SemanticID:  semanticID,
		SystemName:  name,
		DisplayName: domain.Translations{"en": name},
		Position:    position,
		PathElements: []domain.PathElement{
			pe("property", "crm", "P1_is_identified_by", 0),
		},
	}
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

func readCSV(t *testing.T, s string) [][]string {
	t.Helper()
	records, err := csv.NewReader(strings.NewReader(s)).ReadAll()
	if err != nil {
		t.Fatalf("read csv: %v", err)
	}
	return records
}

func cell(row []string, column string) string {
	return row[columnIndex(column)]
}

func columnIndex(column string) int {
	columns := []string{
		"root_kind",
		"project_id",
		"root_semantic_id",
		"field_semantic_id",
		"field_system_name",
		"field_label",
		"field_description",
		"field_relative_path",
		"category_id",
		"collection_id",
		"collection_order",
		"position",
		"ontology_scope",
		"ontology_scope_element",
		"ontology_path",
		"expected_value_type",
		"set_value",
		"is_required",
		"is_hidden",
		"ontology_path_elements",
	}
	for i, name := range columns {
		if name == column {
			return i
		}
	}
	panic("unknown column: " + column)
}
