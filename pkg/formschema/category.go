package formschema

import (
	"fmt"

	"github.com/pletka-io/pletka/pkg/database/dbutil"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/i18n"
)

// BuildCategorySchema constructs the form schema for category create/edit.
// - mode: ModeCreate or ModeEdit
// - existing: the category being edited (nil for create)
// - siblingCategories: other categories in the project (for parent selection)
// - projectID: the project ULID (used to build endpoint URL)
// - lang: the user's preferred UI language
// - languages: available languages from i18n manager
func BuildCategorySchema(mode string, existing *domain.Category, siblingCategories []domain.Category, projectID string, lang string, languages []LanguageInfo) *FormSchema {
	schema := &FormSchema{
		EntityType: "category",
		Mode:       mode,
		UI: SchemaUI{
			Languages:   languages,
			PrimaryLang: lang,
		},
	}

	switch mode {
	case ModeCreate:
		schema.Endpoint = &SchemaEndpoint{
			Method: "POST",
			URL:    fmt.Sprintf("/projects/%s/categories", projectID),
		}
		schema.UI.SubmitLabel = i18n.L("category.form.submit_create", "Create Category")
		schema.UI.CancelLabel = i18n.L("forms.cancel", "Cancel")
		schema.UI.SuccessMessage = i18n.L("category.form.created", "Category created successfully")
	case ModeEdit:
		if existing != nil {
			schema.Endpoint = &SchemaEndpoint{
				Method: "PUT",
				URL:    fmt.Sprintf("/projects/%s/categories/%s", projectID, existing.ID),
			}
		}
		schema.UI.SubmitLabel = i18n.L("forms.save_changes", "Save Changes")
		schema.UI.CancelLabel = i18n.L("forms.cancel", "Cancel")
		schema.UI.SuccessMessage = i18n.L("category.form.updated", "Category updated successfully")
	case ModeView:
		// No endpoint for view mode
		schema.UI.SubmitLabel = nil
	}

	// Build fields section (no label — simple forms don't need section chrome)
	identityFields := buildCategoryIdentityFields(mode, existing)
	schema.Sections = []Section{
		{
			ID:     "identity",
			Fields: identityFields,
		},
	}

	return schema
}

func buildCategoryIdentityFields(mode string, existing *domain.Category) []FieldDef {
	isEdit := mode == ModeEdit || mode == ModeView

	// UI Name
	uiNameField := FieldDef{
		Name:     "ui_name",
		Widget:   WidgetMultilingualText,
		Required: true,
		Readonly: mode == ModeView,
		Label:    i18n.L("forms.name", "Name"),
		Help:     i18n.L("category.form.name_help", "Display name for this category"),
		Validation: &ValidationRules{
			MinLength: dbutil.Ptr(1),
			MaxLength: dbutil.Ptr(200),
		},
	}
	if isEdit && existing != nil {
		uiNameField.Value = existing.UIName
	}

	// Description
	descField := FieldDef{
		Name:     "description",
		Widget:   WidgetMultilingualTextarea,
		Required: false,
		Readonly: mode == ModeView,
		Label:    i18n.L("forms.description", "Description"),
		Help:     i18n.L("category.form.description_help", "Brief description of this category's purpose"),
	}
	if isEdit && existing != nil {
		descField.Value = existing.Description
	}

	// System Name
	systemNameField := FieldDef{
		Name:                 "system_name",
		Widget:               WidgetSystemNamePreview,
		Required:             false,
		Readonly:             isEdit,
		ImmutableAfterCreate: true,
		DerivedFrom:          "ui_name.en",
		Label:                i18n.L("forms.system_name", "System Name"),
		Help:                 i18n.L("forms.system_name_help", "Auto-generated from name. Cannot be changed after creation."),
		Validation: &ValidationRules{
			Pattern:      "^[a-z][a-z0-9_]*$",
			UniqueWithin: "project_entity_type",
		},
	}
	if isEdit && existing != nil {
		systemNameField.Value = existing.SystemName
	}

	return []FieldDef{uiNameField, descField, systemNameField}
}
