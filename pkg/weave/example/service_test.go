package example

import (
	"context"
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/formschema"
)

type fakeStore struct {
	examples map[string]*domain.Example
	values   map[string][]domain.ExampleValue
	concepts map[string]bool
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		examples: map[string]*domain.Example{},
		values:   map[string][]domain.ExampleValue{},
	}
}

func (s *fakeStore) CreateWithValues(_ context.Context, ex *domain.Example, values []domain.ExampleValue) error {
	copyEx := *ex
	s.examples[ex.ID] = &copyEx
	s.values[ex.ID] = append([]domain.ExampleValue(nil), values...)
	return nil
}

func (s *fakeStore) GetByID(_ context.Context, id string) (*domain.Example, error) {
	ex := s.examples[id]
	if ex == nil {
		return nil, nil
	}
	copyEx := *ex
	return &copyEx, nil
}

func (s *fakeStore) UpdateWithValues(_ context.Context, ex *domain.Example, values []domain.ExampleValue) error {
	copyEx := *ex
	s.examples[ex.ID] = &copyEx
	s.values[ex.ID] = append([]domain.ExampleValue(nil), values...)
	return nil
}

func (s *fakeStore) Delete(_ context.Context, id string) error {
	delete(s.examples, id)
	delete(s.values, id)
	return nil
}

func (s *fakeStore) List(_ context.Context, _ ...domain.QueryOption) ([]*domain.Example, int64, error) {
	out := make([]*domain.Example, 0, len(s.examples))
	for _, ex := range s.examples {
		copyEx := *ex
		out = append(out, &copyEx)
	}
	return out, int64(len(out)), nil
}

func (s *fakeStore) ListValues(_ context.Context, exampleID string) ([]domain.ExampleValue, error) {
	return append([]domain.ExampleValue(nil), s.values[exampleID]...), nil
}

func (s *fakeStore) ConceptURIAllowedForLists(_ context.Context, uri string, _ []string) (bool, error) {
	if s.concepts == nil {
		return true, nil
	}
	return s.concepts[uri], nil
}

type fakeViews struct {
	models map[string]*domain.ModelView
}

func (v fakeViews) ModelView(_ context.Context, modelID, _ string) (*domain.ModelView, error) {
	return v.models[modelID], nil
}

func TestServiceCreateNormalizesValueKind(t *testing.T) {
	store := newFakeStore()
	svc := NewService(store, fakeViews{
		models: map[string]*domain.ModelView{
			"M1": singleFieldModelView(11, "F1", "String", false, 0, nil),
		},
	})

	record, err := svc.Create(context.Background(), "P1", CreateInput{
		EntityType: domain.ExampleEntityTypeModel,
		EntityID:   "M1",
		Values: []domain.ExampleValue{
			{
				OverrideID: 11,
				FieldID:    "F1",
				ValueKind:  domain.ExampleValueKindString,
				ValuePayload: domain.ExampleValuePayload{
					StringValue: ptr("hello"),
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if record.Example.Status != domain.ExampleStatusValid {
		t.Fatalf("status = %s, want %s", record.Example.Status, domain.ExampleStatusValid)
	}
	if got := record.Values[0].ValueKind; got != domain.ExampleValueKindString {
		t.Fatalf("ValueKind = %s, want %s", got, domain.ExampleValueKindString)
	}
	if got := record.Values[0].ValuePayload.Kind; got != domain.ExampleValueKindString {
		t.Fatalf("ValuePayload.Kind = %s, want %s", got, domain.ExampleValueKindString)
	}
	if got := store.values[record.Example.ID][0].TextValue; got == nil || *got != "hello" {
		t.Fatalf("stored TextValue = %#v, want hello", got)
	}
}

func TestServiceBuildFormSchemaGroupsFieldsAndValues(t *testing.T) {
	store := newFakeStore()
	store.examples["EX1"] = &domain.Example{
		ID:         "EX1",
		ProjectID:  "P1",
		EntityType: domain.ExampleEntityTypeModel,
		EntityID:   "M1",
	}
	store.values["EX1"] = []domain.ExampleValue{
		{
			ExampleID:       "EX1",
			OverrideID:      22,
			FieldID:         "F2",
			OccurrenceIndex: 0,
			ValueKind:       domain.ExampleValueKindString,
			ValuePayload: domain.ExampleValuePayload{
				Kind:        domain.ExampleValueKindString,
				StringValue: ptr("grouped"),
			},
		},
	}

	svc := NewService(store, fakeViews{
		models: map[string]*domain.ModelView{
			"M1": {
				ModelID:   "M1",
				ProjectID: "P1",
				Categories: []domain.CategoryGroup{
					{
						ID:       "CAT1",
						Name:     domain.Translations{"en": "Main"},
						Position: 1,
						Collections: []domain.CollectionGroup{
							{
								ID:       "__direct__",
								Name:     domain.Translations{"en": "Direct Fields"},
								Position: 0,
								Fields: []domain.ResolvedField{
									resolvedField(11, "F1", "Name", "String", true, 0, nil),
								},
							},
							{
								ID:       "COL1",
								Name:     domain.Translations{"en": "Identity"},
								Position: 1,
								Fields: []domain.ResolvedField{
									resolvedField(22, "F2", "Identifier", "String", false, 0, nil),
								},
							},
						},
					},
				},
			},
		},
	})

	schema, err := svc.BuildFormSchema(
		context.Background(),
		"P1",
		formschema.ModeEdit,
		"",
		"",
		"EX1",
		"en",
		[]formschema.LanguageInfo{{Code: "en", Name: "English"}},
	)
	if err != nil {
		t.Fatalf("BuildFormSchema() error = %v", err)
	}
	if schema.Endpoint == nil || schema.Endpoint.Method != "PUT" || schema.Endpoint.URL != "/projects/P1/examples/EX1" {
		t.Fatalf("endpoint = %#v", schema.Endpoint)
	}
	if got := schema.Target.Name["en"]; got != "M1" {
		t.Fatalf("target name = %q, want model fallback", got)
	}
	if len(schema.Sections) != 1 {
		t.Fatalf("sections = %d, want 1", len(schema.Sections))
	}
	section := schema.Sections[0]
	if len(section.DirectFields) != 1 {
		t.Fatalf("direct fields = %d, want 1", len(section.DirectFields))
	}
	if !hasIssue(section.DirectFields[0].Issues, "missing_required_value", domain.ExampleIssueError) {
		t.Fatalf("direct field issues = %#v, want missing_required_value", section.DirectFields[0].Issues)
	}
	if len(section.Groups) != 1 {
		t.Fatalf("groups = %d, want 1", len(section.Groups))
	}
	groupField := section.Groups[0].Fields[0]
	if len(groupField.Occurrences) != 1 {
		t.Fatalf("occurrences = %d, want 1", len(groupField.Occurrences))
	}
	if got := groupField.Occurrences[0].Value.StringValue; got == nil || *got != "grouped" {
		t.Fatalf("grouped value = %#v, want grouped", got)
	}
}

func TestServiceBuildFormSchemaExposesConceptSources(t *testing.T) {
	store := newFakeStore()
	field := resolvedField(11, "F1", "Language", "Concept", false, 0, nil)
	field.ConceptLists = []domain.EntityRef{
		{
			ID:         "SEM.CL.6",
			SemanticID: "SEM.CL.6",
			Name:       domain.Translations{"en": "Languages"},
			URL:        "/projects/SEM/concept-lists/SEM.CL.6",
		},
	}
	svc := NewService(store, fakeViews{
		models: map[string]*domain.ModelView{
			"M1": {
				ModelID:   "M1",
				ProjectID: "P1",
				Categories: []domain.CategoryGroup{
					{
						ID:       "CAT1",
						Name:     domain.Translations{"en": "Main"},
						Position: 1,
						Collections: []domain.CollectionGroup{
							{ID: "__direct__", Name: domain.Translations{"en": "Direct Fields"}, Fields: []domain.ResolvedField{field}},
						},
					},
				},
			},
		},
	})

	schema, err := svc.BuildFormSchema(
		context.Background(),
		"P1",
		formschema.ModeCreate,
		string(domain.ExampleEntityTypeModel),
		"M1",
		"",
		"en",
		[]formschema.LanguageInfo{{Code: "en", Name: "English"}},
	)
	if err != nil {
		t.Fatalf("BuildFormSchema() error = %v", err)
	}
	got := schema.Sections[0].DirectFields[0]
	if len(got.ConceptSources) != 1 {
		t.Fatalf("ConceptSources = %#v, want one source", got.ConceptSources)
	}
	if got.ConceptSources[0].SearchURL != "/api/v2/concept-lists/SEM.CL.6/entries/search" {
		t.Fatalf("SearchURL = %q", got.ConceptSources[0].SearchURL)
	}
}

func TestValidateModelValuesFlagsWrongKindAndStaleOverride(t *testing.T) {
	store := newFakeStore()
	svc := NewService(store, fakeViews{
		models: map[string]*domain.ModelView{
			"M1": singleFieldModelView(11, "F1", "String", false, 0, nil),
		},
	})

	report, err := svc.validateModelValues(context.Background(), "P1", "M1", []domain.ExampleValue{
		{
			OverrideID:      11,
			FieldID:         "F1",
			OccurrenceIndex: 0,
			ValueKind:       domain.ExampleValueKindInteger,
			ValuePayload: domain.ExampleValuePayload{
				Kind:        domain.ExampleValueKindInteger,
				NumberValue: floatPtr(42),
			},
		},
		{
			OverrideID:      999,
			FieldID:         "F9",
			OccurrenceIndex: 0,
			ValueKind:       domain.ExampleValueKindString,
			ValuePayload: domain.ExampleValuePayload{
				Kind:        domain.ExampleValueKindString,
				StringValue: ptr("stale"),
			},
		},
	})
	if err != nil {
		t.Fatalf("validateModelValues() error = %v", err)
	}
	if report.Valid {
		t.Fatalf("report.Valid = true, want false")
	}
	if !hasIssue(report.Issues, "wrong_value_kind", domain.ExampleIssueError) {
		t.Fatalf("issues = %#v, want wrong_value_kind error", report.Issues)
	}
	if !hasIssue(report.Issues, "stale_override", domain.ExampleIssueWarning) {
		t.Fatalf("issues = %#v, want stale_override warning", report.Issues)
	}
}

func TestValidateModelValuesFlagsConceptOutsideAllowedList(t *testing.T) {
	store := newFakeStore()
	store.concepts = map[string]bool{"https://vocab.getty.edu/aat/300388277": false}
	field := resolvedField(11, "F1", "Language", "Concept", false, 0, nil)
	field.ConceptLists = []domain.EntityRef{{ID: "SEM.CL.6", SemanticID: "SEM.CL.6"}}
	svc := NewService(store, fakeViews{
		models: map[string]*domain.ModelView{
			"M1": {
				ModelID:   "M1",
				ProjectID: "P1",
				Categories: []domain.CategoryGroup{
					{
						ID:       "CAT1",
						Name:     domain.Translations{"en": "Main"},
						Position: 1,
						Collections: []domain.CollectionGroup{
							{ID: "__direct__", Name: domain.Translations{"en": "Direct Fields"}, Fields: []domain.ResolvedField{field}},
						},
					},
				},
			},
		},
	})

	report, err := svc.validateModelValues(context.Background(), "P1", "M1", []domain.ExampleValue{
		{
			OverrideID:      11,
			FieldID:         "F1",
			OccurrenceIndex: 0,
			ValueKind:       domain.ExampleValueKindConcept,
			ValuePayload: domain.ExampleValuePayload{
				Kind:       domain.ExampleValueKindConcept,
				ConceptURI: ptr("https://vocab.getty.edu/aat/300388277"),
			},
		},
	})
	if err != nil {
		t.Fatalf("validateModelValues() error = %v", err)
	}
	if !hasIssue(report.Issues, "concept_not_in_allowed_list", domain.ExampleIssueError) {
		t.Fatalf("issues = %#v, want concept_not_in_allowed_list error", report.Issues)
	}
}

func singleFieldModelView(overrideID int64, fieldID, expected string, required bool, minOccurs int, maxOccurs *int) *domain.ModelView {
	return &domain.ModelView{
		ModelID:   "M1",
		ProjectID: "P1",
		Categories: []domain.CategoryGroup{
			{
				ID:       "CAT1",
				Name:     domain.Translations{"en": "Main"},
				Position: 1,
				Collections: []domain.CollectionGroup{
					{
						ID:       "__direct__",
						Name:     domain.Translations{"en": "Direct Fields"},
						Position: 0,
						Fields: []domain.ResolvedField{
							resolvedField(overrideID, fieldID, "Field", expected, required, minOccurs, maxOccurs),
						},
					},
				},
			},
		},
	}
}

func resolvedField(overrideID int64, fieldID, label, expected string, required bool, minOccurs int, maxOccurs *int) domain.ResolvedField {
	return domain.ResolvedField{
		ID:                fieldID,
		SemanticID:        fieldID,
		DisplayName:       domain.Translations{"en": label},
		ExpectedValueType: expected,
		IsRequired:        required,
		MinOccurs:         minOccurs,
		MaxOccurs:         maxOccurs,
		OverrideID:        overrideID,
	}
}

func hasIssue(issues []domain.ExampleIssue, code string, severity domain.ExampleIssueSeverity) bool {
	for _, issue := range issues {
		if issue.Code == code && issue.Severity == severity {
			return true
		}
	}
	return false
}

func ptr(v string) *string { return &v }

func floatPtr(v float64) *float64 { return &v }
