package formschema

import (
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
)

func TestBuildCategorySchema_Create(t *testing.T) {
	categories := []domain.Category{
		{Entity: domain.Entity{ID: "cat1", UIName: domain.Translations{"en": "Existence"}}},
	}

	langs := []LanguageInfo{{Code: "en", Name: "English"}, {Code: "nl", Name: "Nederlands", Flag: "🇳🇱"}}
	schema := BuildCategorySchema(ModeCreate, nil, categories, "01H", "en", langs)

	if schema.EntityType != "category" {
		t.Errorf("EntityType = %q, want 'category'", schema.EntityType)
	}
	if schema.Mode != ModeCreate {
		t.Errorf("Mode = %q, want 'create'", schema.Mode)
	}
	if schema.Endpoint == nil {
		t.Fatal("Endpoint should not be nil for create mode")
	}
	if schema.Endpoint.Method != "POST" {
		t.Errorf("Endpoint.Method = %q, want 'POST'", schema.Endpoint.Method)
	}

	// Should have identity section with ui_name, description, system_name
	if len(schema.Sections) == 0 {
		t.Fatal("expected at least one section")
	}
	identity := schema.Sections[0]
	if identity.ID != "identity" {
		t.Errorf("first section ID = %q, want 'identity'", identity.ID)
	}

	fieldNames := make(map[string]bool)
	for _, f := range identity.Fields {
		fieldNames[f.Name] = true
	}
	for _, expected := range []string{"ui_name", "description", "system_name"} {
		if !fieldNames[expected] {
			t.Errorf("missing field %q in identity section", expected)
		}
	}

	// system_name should be immutable_after_create and derived_from ui_name.en
	for _, f := range identity.Fields {
		if f.Name == "system_name" {
			if !f.ImmutableAfterCreate {
				t.Error("system_name should be immutable_after_create")
			}
			if f.DerivedFrom != "ui_name.en" {
				t.Errorf("system_name.DerivedFrom = %q, want 'ui_name.en'", f.DerivedFrom)
			}
		}
	}
}

func TestBuildCategorySchema_Edit(t *testing.T) {
	existing := &domain.Category{
		Entity: domain.Entity{
			ID:          "cat1",
			SystemName:  "existence",
			UIName:      domain.Translations{"en": "Existence", "nl": "Bestaan"},
			Description: domain.Translations{"en": "Things that exist"},
		},
	}

	langs := []LanguageInfo{{Code: "en", Name: "English"}, {Code: "nl", Name: "Nederlands", Flag: "🇳🇱"}}
	schema := BuildCategorySchema(ModeEdit, existing, nil, "01H", "en", langs)

	if schema.Mode != ModeEdit {
		t.Errorf("Mode = %q, want 'edit'", schema.Mode)
	}
	if schema.Endpoint == nil || schema.Endpoint.Method != "PUT" {
		t.Error("edit mode should have PUT endpoint")
	}

	// system_name should be readonly in edit mode
	for _, sec := range schema.Sections {
		for _, f := range sec.Fields {
			if f.Name == "system_name" && !f.Readonly {
				t.Error("system_name should be readonly in edit mode")
			}
		}
	}
}
