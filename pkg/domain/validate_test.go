package domain_test

import (
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/google/go-cmp/cmp"
)

func TestFieldValid(t *testing.T) {
	tests := []struct {
		name    string
		field   domain.Field
		wantErr []string
	}{
		{
			name: "valid field with reference model",
			field: domain.Field{
				Entity: domain.Entity{
					ID:          "LAF.309",
					SystemName:  "appellation",
					UIName:      domain.Translations{"en": "Appellation"},
					Description: domain.Translations{"en": "Identifies the person by name"},
					Status:      "draft",
					ProjectID:   "LA",
				},
				OntologyScope: domain.PathElement{
					Type: "class", URI: "crm:E21_Person",
					Prefix: "crm", LocalName: "E21_Person", ClassCode: "E21",
				},
				PathElements: []domain.PathElement{
					{Type: "property", URI: "crm:P1_is_identified_by", Prefix: "crm", LocalName: "P1_is_identified_by", Position: 0},
					{Type: "class", URI: "crm:E33_E41_Linguistic_Appellation", Prefix: "crm", LocalName: "E33_E41_Linguistic_Appellation", Position: 1, ClassCode: "E33_E41"},
				},
				ExpectedValueType: "Reference Model",
			},
			wantErr: nil,
		},
		{
			name:    "empty field",
			field:   domain.Field{},
			wantErr: []string{"id is required", "project_id is required", "ui_name is required", "description is required", "ontology_scope is required", "path_elements is required", "expected_value_type is required"},
		},
		{
			name: "literal value type without value_id is ok",
			field: domain.Field{
				Entity: domain.Entity{
					ID: "LAF.2", SystemName: "date", ProjectID: "LA",
					UIName:      domain.Translations{"en": "Date"},
					Description: domain.Translations{"en": "A date"},
					Status:      "draft",
				},
				OntologyScope:     domain.PathElement{Type: "class", URI: "crm:E21_Person", Prefix: "crm", LocalName: "E21_Person", ClassCode: "E21"},
				PathElements:      []domain.PathElement{{Type: "property", Position: 0}},
				ExpectedValueType: "Date",
			},
			wantErr: nil,
		},
		{
			name: "multi-class scope with both URIs is valid",
			field: domain.Field{
				Entity: domain.Entity{
					ID: "LAF.50", SystemName: "image_label", ProjectID: "LA",
					UIName:      domain.Translations{"en": "Image label"},
					Description: domain.Translations{"en": "Label of an image"},
					Status:      "draft",
				},
				OntologyScope: domain.PathElement{
					Type: "class", URI: "crmdig:D1_Digital_Object",
					Prefix: "crmdig", LocalName: "D1_Digital_Object", ClassCode: "D1",
					AdditionalTypes: []domain.TypeRef{
						{URI: "crm:E36_Visual_Item", Prefix: "crm", LocalName: "E36_Visual_Item", ClassCode: "E36"},
					},
				},
				PathElements:      []domain.PathElement{{Type: "property", Position: 0}},
				ExpectedValueType: "Text",
			},
			wantErr: nil,
		},
		{
			name: "additional type missing URI rejected",
			field: domain.Field{
				Entity: domain.Entity{
					ID: "LAF.51", SystemName: "broken_scope", ProjectID: "LA",
					UIName:      domain.Translations{"en": "Broken"},
					Description: domain.Translations{"en": "Broken scope"},
					Status:      "draft",
				},
				OntologyScope: domain.PathElement{
					Type: "class", URI: "crm:E21_Person",
					Prefix: "crm", LocalName: "E21_Person", ClassCode: "E21",
					AdditionalTypes: []domain.TypeRef{
						{Prefix: "crm", LocalName: "E36_Visual_Item"},
					},
				},
				PathElements:      []domain.PathElement{{Type: "property", Position: 0}},
				ExpectedValueType: "Text",
			},
			wantErr: []string{"ontology_scope.additional_types[0].uri is required"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := tt.field.Valid()
			var gotMsgs []string
			for _, e := range errs {
				gotMsgs = append(gotMsgs, e.Error())
			}
			if diff := cmp.Diff(tt.wantErr, gotMsgs); diff != "" {
				t.Errorf("Field.Valid() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestModelValid(t *testing.T) {
	tests := []struct {
		name    string
		model   domain.Model
		wantErr []string
	}{
		{
			name: "valid model",
			model: domain.Model{
				Entity: domain.Entity{
					ID: "LAM.1", ProjectID: "LA",
					UIName:      domain.Translations{"en": "Person"},
					Description: domain.Translations{"en": "A person entity"},
					Status:      "draft",
				},
				OntologyScope: domain.PathElement{Type: "class", URI: "crm:E21_Person", Prefix: "crm", LocalName: "E21_Person", ClassCode: "E21"},
			},
			wantErr: nil,
		},
		{
			name:    "empty model",
			model:   domain.Model{},
			wantErr: []string{"id is required", "project_id is required", "ui_name is required", "description is required", "ontology_scope is required"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := tt.model.Valid()
			var gotMsgs []string
			for _, e := range errs {
				gotMsgs = append(gotMsgs, e.Error())
			}
			if diff := cmp.Diff(tt.wantErr, gotMsgs); diff != "" {
				t.Errorf("Model.Valid() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestCollectionValid(t *testing.T) {
	tests := []struct {
		name       string
		collection domain.Collection
		wantErr    []string
	}{
		{
			name: "valid collection",
			collection: domain.Collection{
				Entity: domain.Entity{
					ID: "LAC.17", ProjectID: "LA",
					UIName:      domain.Translations{"en": "Birth"},
					Description: domain.Translations{"en": "Birth event collection"},
					Status:      "draft",
				},
				OntologyScope: domain.PathElement{Type: "class", URI: "crm:E67_Birth", Prefix: "crm", LocalName: "E67_Birth", ClassCode: "E67"},
			},
			wantErr: nil,
		},
		{
			name:       "empty collection",
			collection: domain.Collection{},
			wantErr:    []string{"id is required", "project_id is required", "ui_name is required", "description is required", "ontology_scope is required"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := tt.collection.Valid()
			var gotMsgs []string
			for _, e := range errs {
				gotMsgs = append(gotMsgs, e.Error())
			}
			if diff := cmp.Diff(tt.wantErr, gotMsgs); diff != "" {
				t.Errorf("Collection.Valid() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestCategoryValid(t *testing.T) {
	tests := []struct {
		name     string
		category domain.Category
		wantErr  []string
	}{
		{
			name: "valid category",
			category: domain.Category{
				Entity: domain.Entity{
					ID: "LA.CAT.5", ProjectID: "LA",
					UIName:      domain.Translations{"en": "Events"},
					Description: domain.Translations{"en": "Event-related fields"},
					Status:      "draft",
				},
				CanonicalOrder: 5,
			},
			wantErr: nil,
		},
		{
			name:     "empty category",
			category: domain.Category{},
			wantErr:  []string{"id is required", "project_id is required", "ui_name is required", "description is required"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := tt.category.Valid()
			var gotMsgs []string
			for _, e := range errs {
				gotMsgs = append(gotMsgs, e.Error())
			}
			if diff := cmp.Diff(tt.wantErr, gotMsgs); diff != "" {
				t.Errorf("Category.Valid() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
