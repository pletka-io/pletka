package formschema

import (
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/i18n"
)

func TestBuildCompositionFieldSidebarSchemaDirectModelField(t *testing.T) {
	schema := BuildCompositionFieldSidebarSchema(
		&CompositionFieldSidebarInput{
			ExpectedValueType: "Model",
		},
		CompositionFieldSidebarOptions{
			EntityType:    "model",
			GroupWidget:   "field-group",
			CanEdit:       true,
			CanHideFields: true,
			CategoryOptions: []SelectOption{
				{Value: "", Label: domain.Translations{"en": "Uncategorized"}},
			},
			ModelOptions: []SelectOption{
				{Value: "model-1", Label: domain.Translations{"en": "Person"}},
			},
		},
		"en",
		[]LanguageInfo{{Code: "en", Name: "English", Flag: "🇬🇧"}},
	)

	fields := schema.Sections[0].Fields
	if len(fields) == 0 {
		t.Fatal("expected fields in schema")
	}

	var categoryField, modelField, hiddenField *FieldDef
	for i := range fields {
		switch fields[i].Name {
		case "category_id":
			categoryField = &fields[i]
		case "expected_resource_models":
			modelField = &fields[i]
		case "is_hidden":
			hiddenField = &fields[i]
		}
	}

	if categoryField == nil {
		t.Fatal("expected category field")
	}
	if categoryField.Readonly {
		t.Fatal("direct field category should be editable")
	}
	if categoryField.CreateURL == "" {
		t.Fatal("category field should expose create_url")
	}
	if modelField == nil {
		t.Fatal("expected allowed-model picker for Model fields")
	}
	if modelField.Widget != WidgetPillMultiSelect {
		t.Fatalf("model widget = %q", modelField.Widget)
	}
	if hiddenField == nil {
		t.Fatal("expected is_hidden field when can_hide_fields is true")
	}
}

func TestBuildCompositionFieldSidebarSchemaCollectionGroupLocksCategory(t *testing.T) {
	schema := BuildCompositionFieldSidebarSchema(
		&CompositionFieldSidebarInput{
			ExpectedValueType: "Collection",
		},
		CompositionFieldSidebarOptions{
			EntityType:        "model",
			GroupWidget:       "collection-group",
			CanEdit:           true,
			CanHideFields:     false,
			CategoryOptions:   []SelectOption{{Value: "", Label: domain.Translations{"en": "Uncategorized"}}},
			CollectionOptions: []SelectOption{{Value: "collection-1", Label: domain.Translations{"en": "Identifier"}}},
		},
		"en",
		[]LanguageInfo{{Code: "en", Name: "English", Flag: "🇬🇧"}},
	)

	var categoryField, collectionField *FieldDef
	for i := range schema.Sections[0].Fields {
		switch schema.Sections[0].Fields[i].Name {
		case "category_id":
			categoryField = &schema.Sections[0].Fields[i]
		case "expected_collection_models":
			collectionField = &schema.Sections[0].Fields[i]
		}
	}

	if categoryField == nil {
		t.Fatal("expected category field")
	}
	if !categoryField.Readonly {
		t.Fatal("collection-group field category should be readonly")
	}
	if localizableEnglish(categoryField.Help) == "" {
		t.Fatal("expected category lock help text")
	}
	if categoryField.CreateURL != "" {
		t.Fatal("readonly category field should not expose create_url")
	}
	if collectionField == nil {
		t.Fatal("expected allowed-collection picker for Collection fields")
	}
}

func TestBuildCompositionFieldSidebarSchemaConceptFieldShowsConceptLists(t *testing.T) {
	schema := BuildCompositionFieldSidebarSchema(
		&CompositionFieldSidebarInput{
			ExpectedValueType:      "Concept",
			ExpectedConceptListIDs: []string{"SEM.CL.6"},
		},
		CompositionFieldSidebarOptions{
			EntityType:  "model",
			GroupWidget: "field-group",
			CanEdit:     true,
			ConceptListOptions: []SelectOption{{
				Value:      "SEM.CL.6",
				Label:      domain.Translations{"en": "Languages"},
				SemanticID: "SEM.CL.6",
			}},
		},
		"en",
		[]LanguageInfo{{Code: "en", Name: "English", Flag: "🇬🇧"}},
	)

	var conceptListField *FieldDef
	for i := range schema.Sections[0].Fields {
		if schema.Sections[0].Fields[i].Name == "expected_concept_lists" {
			conceptListField = &schema.Sections[0].Fields[i]
			break
		}
	}
	if conceptListField == nil {
		t.Fatal("expected allowed concept-list picker for Concept fields")
	}
	if conceptListField.Widget != WidgetPillMultiSelect {
		t.Fatalf("concept-list widget = %q", conceptListField.Widget)
	}
	if got := conceptListField.Value.([]string); len(got) != 1 || got[0] != "SEM.CL.6" {
		t.Fatalf("concept-list value = %#v, want [SEM.CL.6]", conceptListField.Value)
	}
	if len(conceptListField.Options) != 1 || conceptListField.Options[0].Value != "SEM.CL.6" {
		t.Fatalf("concept-list options = %#v", conceptListField.Options)
	}
}

func localizableEnglish(v domain.Localizable) string {
	switch t := any(v).(type) {
	case domain.Translations:
		return t.Get("en")
	case i18n.LocalizedText:
		return t.Translations.Get("en")
	default:
		return ""
	}
}
