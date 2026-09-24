package collection

import (
	"fmt"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/formschema"
	"github.com/pletka-io/pletka/pkg/i18n"
)

// BuildFormSchema is the canonical create/edit form schema for a
// collection. Identity + ontology_scope; default_category_id surfaces
// in edit mode (project-level pre-fill for fields when this collection
// is dropped into a model).
func BuildFormSchema(mode string, existing *domain.Collection, projectID, lang string, languages []formschema.LanguageInfo) *formschema.FormSchema {
	schema := &formschema.FormSchema{
		EntityType: "collection",
		Mode:       mode,
		UI: formschema.SchemaUI{
			Languages:   languages,
			PrimaryLang: lang,
		},
	}
	switch mode {
	case formschema.ModeCreate:
		schema.Endpoint = &formschema.SchemaEndpoint{
			Method: "POST",
			URL:    fmt.Sprintf("/projects/%s/collections", projectID),
		}
		schema.UI.SubmitLabel = i18n.L("collection.form.submit_create", "Create Collection")
		schema.UI.CancelLabel = i18n.L("forms.cancel", "Cancel")
		schema.UI.SuccessMessage = i18n.L("collection.form.created", "Collection created successfully")
	case formschema.ModeEdit:
		if existing != nil {
			schema.Endpoint = &formschema.SchemaEndpoint{
				Method: "PUT",
				URL:    fmt.Sprintf("/projects/%s/collections/%s", projectID, existing.ID),
			}
		}
		schema.UI.SubmitLabel = i18n.L("forms.save_changes", "Save Changes")
		schema.UI.CancelLabel = i18n.L("forms.cancel", "Cancel")
		schema.UI.SuccessMessage = i18n.L("collection.form.updated", "Collection updated successfully")
	case formschema.ModeView:
		schema.UI.SubmitLabel = nil
	}
	schema.Sections = []formschema.Section{
		{ID: "identity", Fields: buildIdentityFields(mode, existing, projectID)},
	}
	return schema
}

func buildIdentityFields(mode string, existing *domain.Collection, projectID string) []formschema.FieldDef {
	isEdit := mode == formschema.ModeEdit || mode == formschema.ModeView
	one := 1
	twoHundred := 200

	uiName := formschema.FieldDef{
		Name:     "ui_name",
		Widget:   formschema.WidgetMultilingualText,
		Required: true,
		Readonly: mode == formschema.ModeView,
		Label:    i18n.L("forms.name", "Name"),
		Help:     i18n.L("collection.form.name_help", "Display name for this collection"),
		Validation: &formschema.ValidationRules{
			MinLength: &one,
			MaxLength: &twoHundred,
		},
	}
	if isEdit && existing != nil {
		uiName.Value = existing.UIName
	}

	desc := formschema.FieldDef{
		Name:     "description",
		Widget:   formschema.WidgetMultilingualTextarea,
		Required: false,
		Readonly: mode == formschema.ModeView,
		Label:    i18n.L("forms.description", "Description"),
		Help:     i18n.L("collection.form.description_help", "Brief description of what this collection groups"),
	}
	if isEdit && existing != nil {
		desc.Value = existing.Description
	}

	systemName := formschema.FieldDef{
		Name:                 "system_name",
		Widget:               formschema.WidgetSystemNamePreview,
		Required:             false,
		Readonly:             isEdit,
		ImmutableAfterCreate: true,
		DerivedFrom:          "ui_name.en",
		Label:                i18n.L("forms.system_name", "System Name"),
		Help:                 i18n.L("forms.system_name_help", "Auto-generated from name. Cannot be changed after creation."),
		Validation: &formschema.ValidationRules{
			Pattern:      "^[a-z][a-z0-9_]*$",
			UniqueWithin: "project_entity_type",
		},
	}
	if isEdit && existing != nil {
		systemName.Value = existing.SystemName
	}

	scope := formschema.FieldDef{
		Name:     "ontology_scope",
		Widget:   formschema.WidgetOntologyPath,
		Required: true,
		Readonly: mode == formschema.ModeView,
		Label:    i18n.L("collection.form.ontology_scope", "Ontology Scope"),
		Help:     i18n.L("collection.form.ontology_scope_help", "Root CRM class shared by the fields in this collection."),
	}
	if isEdit && existing != nil {
		scope.Value = existing.OntologyScope
	}

	defaultCategory := formschema.FieldDef{
		Name:        "default_category_id",
		Widget:      formschema.WidgetSelect,
		Required:    false,
		Readonly:    mode == formschema.ModeView,
		Label:       i18n.L("collection.form.default_category", "Default Category"),
		Help:        i18n.L("collection.form.default_category_help", "Project-level default category for this collection's fields. Models can override per context."),
		EntityType:  "category",
		OptionsURL:  fmt.Sprintf("/projects/%s/categories/options", projectID),
		CreateURL:   "/api/v1/drafts",
		CreateLabel: i18n.L("category.form.submit_create", "Create Category"),
	}
	if isEdit && existing != nil && existing.DefaultCategoryID != nil {
		defaultCategory.Value = *existing.DefaultCategoryID
	}

	return []formschema.FieldDef{uiName, desc, systemName, scope, defaultCategory}
}
