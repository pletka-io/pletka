package example

import (
	"context"
	"strings"
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
	if len(section.Groups) != 2 {
		t.Fatalf("groups = %d, want 2", len(section.Groups))
	}
	directGroup := section.Groups[0]
	if directGroup.ID != "__direct__" || len(directGroup.Fields) != 1 {
		t.Fatalf("direct group = %+v, want 1 field", directGroup)
	}
	if !hasIssue(directGroup.Fields[0].Issues, "missing_required_value", domain.ExampleIssueError) {
		t.Fatalf("direct field issues = %#v, want missing_required_value", directGroup.Fields[0].Issues)
	}
	groupField := section.Groups[1].Fields[0]
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
	got := schema.Sections[0].Groups[0].Fields[0]
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

// stubModelView is a model M1 with one Model-typed field (override 21, field F1)
// whose allowed targets are the given resource model IDs.
func stubModelView(resourceModels ...string) *domain.ModelView {
	view := singleFieldModelView(21, "F1", "Model", false, 0, nil)
	f := &view.Categories[0].Collections[0].Fields[0]
	for _, id := range resourceModels {
		f.ResourceModels = append(f.ResourceModels, domain.EntityRef{ID: id, SemanticID: id})
	}
	return view
}

func stubValue(targetID, label string) domain.ExampleValue {
	var target *string
	if targetID != "" {
		target = ptr(targetID)
	}
	return domain.ExampleValue{
		OverrideID: 21,
		FieldID:    "F1",
		ValueKind:  domain.ExampleValueKindExampleRef,
		ValuePayload: domain.ExampleValuePayload{
			Kind:           domain.ExampleValueKindExampleRef,
			TargetEntityID: target,
			TargetLabel:    ptr(label),
		},
	}
}

func findStub(store *fakeStore, exceptID string) *domain.Example {
	for id, ex := range store.examples {
		if id != exceptID {
			return ex
		}
	}
	return nil
}

func TestServiceCreateMaterializesStubExample(t *testing.T) {
	store := newFakeStore()
	svc := NewService(store, fakeViews{models: map[string]*domain.ModelView{"M1": stubModelView("M2")}})

	record, err := svc.Create(context.Background(), "P1", CreateInput{
		EntityType: domain.ExampleEntityTypeModel,
		EntityID:   "M1",
		Lang:       "nl",
		Values:     []domain.ExampleValue{stubValue("", "Van Gogh")}, // target inferred: single resource model
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if len(store.examples) != 2 {
		t.Fatalf("want 2 stored examples (record + stub), got %d", len(store.examples))
	}
	stub := findStub(store, record.Example.ID)
	if stub == nil {
		t.Fatal("stub example not stored")
	}
	if stub.EntityType != domain.ExampleEntityTypeModel || stub.EntityID != "M2" {
		t.Fatalf("stub target = %s/%s, want model/M2", stub.EntityType, stub.EntityID)
	}
	if stub.Status != domain.ExampleStatusDraft {
		t.Fatalf("stub status = %s, want draft", stub.Status)
	}
	if got := stub.Title["nl"]; got != "Van Gogh" {
		t.Fatalf("stub title[nl] = %q, want Van Gogh", got)
	}
	if len(store.values[stub.ID]) != 0 {
		t.Fatalf("stub must have no values, got %d", len(store.values[stub.ID]))
	}
	v := record.Values[0]
	if v.ValuePayload.ExampleID == nil || *v.ValuePayload.ExampleID != stub.ID {
		t.Fatalf("value payload example_id = %v, want stub id %s", v.ValuePayload.ExampleID, stub.ID)
	}
	if v.LinkedExampleID == nil || *v.LinkedExampleID != stub.ID {
		t.Fatalf("linked_example_id column = %v, want %s", v.LinkedExampleID, stub.ID)
	}
	if !record.Validation.Valid {
		t.Fatalf("expected valid record, issues = %+v", record.Validation.Issues)
	}
}

func TestServiceCreateStubDefaultsTitleLangToEnglish(t *testing.T) {
	store := newFakeStore()
	svc := NewService(store, fakeViews{models: map[string]*domain.ModelView{"M1": stubModelView("M2")}})
	record, err := svc.Create(context.Background(), "P1", CreateInput{
		EntityType: domain.ExampleEntityTypeModel, EntityID: "M1",
		Values: []domain.ExampleValue{stubValue("M2", "Irises")},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if got := findStub(store, record.Example.ID).Title["en"]; got != "Irises" {
		t.Fatalf("stub title[en] = %q, want Irises", got)
	}
}

func TestServiceUpdateMaterializesStubExample(t *testing.T) {
	store := newFakeStore()
	svc := NewService(store, fakeViews{models: map[string]*domain.ModelView{"M1": stubModelView("M2")}})
	created, err := svc.Create(context.Background(), "P1", CreateInput{
		EntityType: domain.ExampleEntityTypeModel, EntityID: "M1",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	updated, err := svc.Update(context.Background(), "P1", created.Example.ID, UpdateInput{
		Values: []domain.ExampleValue{stubValue("", "Van Gogh")},
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if len(store.examples) != 2 {
		t.Fatalf("want 2 stored examples after update, got %d", len(store.examples))
	}
	if updated.Values[0].ValuePayload.ExampleID == nil {
		t.Fatal("update did not link the stub")
	}
	// Re-saving the returned values (now carrying example_id) must not create a second stub.
	if _, err := svc.Update(context.Background(), "P1", created.Example.ID, UpdateInput{Values: updated.Values}); err != nil {
		t.Fatalf("second Update() error = %v", err)
	}
	if len(store.examples) != 2 {
		t.Fatalf("re-save created a duplicate stub: %d examples", len(store.examples))
	}
}

func TestServiceStubGuards(t *testing.T) {
	cases := []struct {
		name    string
		view    *domain.ModelView
		value   domain.ExampleValue
		wantErr string
	}{
		{
			name:    "ambiguous target with two resource models and no target id",
			view:    stubModelView("M2", "M3"),
			value:   stubValue("", "Van Gogh"),
			wantErr: "target_entity_id is required",
		},
		{
			name:    "ambiguous target when any model is allowed",
			view:    stubModelView(),
			value:   stubValue("", "Van Gogh"),
			wantErr: "target_entity_id is required",
		},
		{
			name:    "target outside resource models",
			view:    stubModelView("M2"),
			value:   stubValue("M9", "Van Gogh"),
			wantErr: "not an allowed target",
		},
		{
			name: "non-Model field",
			view: singleFieldModelView(21, "F1", "String", false, 0, nil),
			value: domain.ExampleValue{
				OverrideID: 21, FieldID: "F1", ValueKind: domain.ExampleValueKindExampleRef,
				ValuePayload: domain.ExampleValuePayload{Kind: domain.ExampleValueKindExampleRef, TargetLabel: ptr("x")},
			},
			wantErr: "only be created for Model-typed fields",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := newFakeStore()
			svc := NewService(store, fakeViews{models: map[string]*domain.ModelView{"M1": tc.view}})
			_, err := svc.Create(context.Background(), "P1", CreateInput{
				EntityType: domain.ExampleEntityTypeModel, EntityID: "M1",
				Values: []domain.ExampleValue{tc.value},
			})
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v, want containing %q", err, tc.wantErr)
			}
			if len(store.examples) != 0 {
				t.Fatalf("no example may be stored on a guard failure, got %d", len(store.examples))
			}
		})
	}
}

func TestServiceStubGuardFailsOnSecondValueStoresNothing(t *testing.T) {
	store := newFakeStore()
	svc := NewService(store, fakeViews{models: map[string]*domain.ModelView{"M1": stubModelView("M2")}})

	first := stubValue("", "Van Gogh")
	second := stubValue("M9", "Bad")
	second.OccurrenceIndex = 1

	_, err := svc.Create(context.Background(), "P1", CreateInput{
		EntityType: domain.ExampleEntityTypeModel, EntityID: "M1",
		Values: []domain.ExampleValue{first, second},
	})
	if err == nil || !strings.Contains(err.Error(), "not an allowed target") {
		t.Fatalf("err = %v, want containing %q", err, "not an allowed target")
	}
	if len(store.examples) != 0 {
		t.Fatalf("a guard failure on the second value must not leave the first value's stub stored, got %d examples", len(store.examples))
	}
}

func TestServiceStubIgnoresBlankLabelAndExplicitLink(t *testing.T) {
	store := newFakeStore()
	svc := NewService(store, fakeViews{models: map[string]*domain.ModelView{"M1": stubModelView("M2")}})
	linked := &domain.Example{ID: "EX-LINKED", ProjectID: "P1", EntityType: domain.ExampleEntityTypeModel, EntityID: "M2", Status: domain.ExampleStatusDraft}
	if err := store.CreateWithValues(context.Background(), linked, nil); err != nil {
		t.Fatal(err)
	}
	explicit := stubValue("", "ignored label")
	explicit.ValuePayload.ExampleID = ptr("EX-LINKED")
	blank := stubValue("", "   ")
	blank.OccurrenceIndex = 1

	record, err := svc.Create(context.Background(), "P1", CreateInput{
		EntityType: domain.ExampleEntityTypeModel, EntityID: "M1",
		Values: []domain.ExampleValue{explicit, blank},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if len(store.examples) != 2 { // linked + the record itself, no stubs
		t.Fatalf("want 2 examples, got %d", len(store.examples))
	}
	if got := *record.Values[0].ValuePayload.ExampleID; got != "EX-LINKED" {
		t.Fatalf("explicit link overwritten: %s", got)
	}
}

// TestServiceStubSameLabelTwiceCreatesOneDraft locks George's case: the same
// new draft typed in two fields of one unsaved example becomes one draft,
// linked from both occurrences.
func TestServiceStubSameLabelTwiceCreatesOneDraft(t *testing.T) {
	store := newFakeStore()
	svc := NewService(store, fakeViews{models: map[string]*domain.ModelView{"M1": stubModelView("M2")}})
	first := stubValue("", "Van Gogh")
	second := stubValue("M2", "  Van Gogh ") // same target after inference, same label after trimming
	second.OccurrenceIndex = 1
	record, err := svc.Create(context.Background(), "P1", CreateInput{
		EntityType: domain.ExampleEntityTypeModel, EntityID: "M1",
		Values: []domain.ExampleValue{first, second},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if len(store.examples) != 2 { // parent + ONE draft
		t.Fatalf("want 2 stored examples, got %d", len(store.examples))
	}
	a, b := record.Values[0].ValuePayload.ExampleID, record.Values[1].ValuePayload.ExampleID
	if a == nil || b == nil || *a != *b {
		t.Fatalf("both occurrences must link the same draft, got %v and %v", a, b)
	}
	other := stubValue("", "Gauguin")
	other.OccurrenceIndex = 2
	if _, err := svc.Update(context.Background(), "P1", record.Example.ID, UpdateInput{Values: append(record.Values, other)}); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if len(store.examples) != 3 { // a different label still creates its own draft
		t.Fatalf("want 3 stored examples after adding Gauguin, got %d", len(store.examples))
	}
}

// namingViews is fakeViews plus a Models() reader, mirroring the optional
// interface the real store satisfies.
type namingViews struct {
	fakeViews
	names map[string]domain.Translations
}

func (v namingViews) Models() domain.WeaveModelStore { return namingModels{names: v.names} }

type namingModels struct {
	domain.WeaveModelStore // nil; only GetByID is called
	names                  map[string]domain.Translations
}

func (m namingModels) GetByID(_ context.Context, id string) (*domain.Model, error) {
	name, ok := m.names[id]
	if !ok {
		return nil, nil
	}
	mod := &domain.Model{}
	mod.ID = id
	mod.UIName = name
	return mod, nil
}

func TestBuildFormSchemaNamesTargetModel(t *testing.T) {
	views := namingViews{
		fakeViews: fakeViews{models: map[string]*domain.ModelView{"M1": singleFieldModelView(11, "F1", "String", false, 0, nil)}},
		names:     map[string]domain.Translations{"M1": {"en": "Physical Thing", "nl": "Fysiek object"}},
	}
	svc := NewService(newFakeStore(), views)
	schema, err := svc.BuildFormSchema(context.Background(), "P1", formschema.ModeCreate, "model", "M1", "", "en", nil)
	if err != nil {
		t.Fatalf("BuildFormSchema() error = %v", err)
	}
	if got := schema.Target.Name["en"]; got != "Physical Thing" {
		t.Fatalf("target name = %q, want Physical Thing", got)
	}
	if got := schema.Target.Name["nl"]; got != "Fysiek object" {
		t.Fatalf("target name[nl] = %q", got)
	}
	// Without a namer the id stays the fallback.
	plain := NewService(newFakeStore(), views.fakeViews)
	schema, err = plain.BuildFormSchema(context.Background(), "P1", formschema.ModeCreate, "model", "M1", "", "en", nil)
	if err != nil {
		t.Fatalf("BuildFormSchema() error = %v", err)
	}
	if got := schema.Target.Name["en"]; got != "M1" {
		t.Fatalf("fallback target name = %q, want M1", got)
	}
}

// TestBuildFormSchemaKeepsCollectionOrderIncludingDirectFields: the form
// emits the direct-fields bucket as a group at its own position rather than
// always first, matching the model page.
func TestBuildFormSchemaKeepsCollectionOrderIncludingDirectFields(t *testing.T) {
	view := &domain.ModelView{ModelID: "M1", ProjectID: "P1", Categories: []domain.CategoryGroup{{
		ID: "CAT1", Name: domain.Translations{"en": "Names"}, Position: 1,
		Collections: []domain.CollectionGroup{
			{ID: "C1", Name: domain.Translations{"en": "Name"}, Position: 1, Fields: []domain.ResolvedField{resolvedField(11, "F1", "Name", "String", false, 0, nil)}},
			{ID: "__direct__", Name: domain.Translations{"en": "Direct Fields"}, Position: 2, Fields: []domain.ResolvedField{resolvedField(12, "F2", "Equivalent", "URI", false, 0, nil)}},
			{ID: "C2", Name: domain.Translations{"en": "Identifier"}, Position: 3, Fields: []domain.ResolvedField{resolvedField(13, "F3", "Identifier", "String", false, 0, nil)}},
		},
	}}}
	svc := NewService(newFakeStore(), fakeViews{models: map[string]*domain.ModelView{"M1": view}})
	schema, err := svc.BuildFormSchema(context.Background(), "P1", formschema.ModeCreate, "model", "M1", "", "en", nil)
	if err != nil {
		t.Fatalf("BuildFormSchema() error = %v", err)
	}
	sec := schema.Sections[0]
	if len(sec.DirectFields) != 0 {
		t.Fatalf("direct_fields must be empty now, got %d", len(sec.DirectFields))
	}
	ids := make([]string, 0, len(sec.Groups))
	for _, g := range sec.Groups {
		ids = append(ids, g.ID)
	}
	if len(ids) != 3 || ids[0] != "C1" || ids[1] != "__direct__" || ids[2] != "C2" {
		t.Fatalf("groups = %v, want [C1 __direct__ C2]", ids)
	}
	if sec.Groups[1].Label["en"] != "Direct Fields" || sec.Groups[1].Position != 2 {
		t.Fatalf("direct group = %+v", sec.Groups[1])
	}
}

// TestServiceCreateFillsSlotPath: today's clients send override_id +
// occurrence_index only; the service derives the depth-one slot path.
func TestServiceCreateFillsSlotPath(t *testing.T) {
	store := newFakeStore()
	svc := NewService(store, fakeViews{models: map[string]*domain.ModelView{"M1": singleFieldModelView(11, "F1", "String", false, 0, nil)}})
	record, err := svc.Create(context.Background(), "P1", CreateInput{
		EntityType: domain.ExampleEntityTypeModel, EntityID: "M1",
		Values: []domain.ExampleValue{
			{OverrideID: 11, FieldID: "F1", OccurrenceIndex: 1, ValueKind: domain.ExampleValueKindString, ValuePayload: domain.ExampleValuePayload{StringValue: ptr("b")}},
			{OverrideID: 11, FieldID: "F1", OccurrenceIndex: 0, ValueKind: domain.ExampleValueKindString, ValuePayload: domain.ExampleValuePayload{StringValue: ptr("a")}},
		},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if got := record.Values[0].SlotPath; got != "11:0" {
		t.Fatalf("values[0].SlotPath = %q, want 11:0 (sorted first)", got)
	}
	if got := record.Values[1].SlotPath; got != "11:1" {
		t.Fatalf("values[1].SlotPath = %q, want 11:1", got)
	}
	if got := store.values[record.Example.ID][0].SlotPath; got != "11:0" {
		t.Fatalf("stored SlotPath = %q", got)
	}
}

// TestServiceCreateDerivesLeafFromSlotPath: a value that arrives with only a
// slot path gets override_id/occurrence_index derived, so validation and the
// old columns keep working.
func TestServiceCreateDerivesLeafFromSlotPath(t *testing.T) {
	store := newFakeStore()
	svc := NewService(store, fakeViews{models: map[string]*domain.ModelView{"M1": singleFieldModelView(11, "F1", "String", false, 0, nil)}})
	record, err := svc.Create(context.Background(), "P1", CreateInput{
		EntityType: domain.ExampleEntityTypeModel, EntityID: "M1",
		Values: []domain.ExampleValue{
			{SlotPath: "11:2", FieldID: "F1", ValueKind: domain.ExampleValueKindString, ValuePayload: domain.ExampleValuePayload{StringValue: ptr("c")}},
		},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	v := record.Values[0]
	if v.OverrideID != 11 || v.OccurrenceIndex != 2 {
		t.Fatalf("derived override/occurrence = %d/%d, want 11/2", v.OverrideID, v.OccurrenceIndex)
	}
	if !record.Validation.Valid {
		t.Fatalf("expected valid, issues = %+v", record.Validation.Issues)
	}
}

func TestServiceCreateRejectsMalformedSlotPath(t *testing.T) {
	svc := NewService(newFakeStore(), fakeViews{models: map[string]*domain.ModelView{"M1": singleFieldModelView(11, "F1", "String", false, 0, nil)}})
	_, err := svc.Create(context.Background(), "P1", CreateInput{
		EntityType: domain.ExampleEntityTypeModel, EntityID: "M1",
		Values: []domain.ExampleValue{{SlotPath: "nope", FieldID: "F1", ValueKind: domain.ExampleValueKindString, ValuePayload: domain.ExampleValuePayload{StringValue: ptr("c")}}},
	})
	if err == nil || !strings.Contains(err.Error(), "slot_path") {
		t.Fatalf("err = %v, want a slot_path error", err)
	}
}
