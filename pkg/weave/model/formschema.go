package model

import (
	"fmt"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/formschema"
	"github.com/pletka-io/pletka/pkg/i18n"
)

// BuildFormSchema is the canonical create/edit form schema for a model
// (per ADR-0001 — slice owns its schema; entityschema dispatcher
// delegates here). Identity-only — override editing happens through
// /{modelID}/overrides, not on this form.
func BuildFormSchema(mode string, existing *domain.Model, projectID, lang string, languages []formschema.LanguageInfo) *formschema.FormSchema {
	schema := &formschema.FormSchema{
		EntityType: "model",
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
			URL:    fmt.Sprintf("/projects/%s/models", projectID),
		}
		schema.UI.SubmitLabel = i18n.L("model.form.submit_create", "Create Model")
		schema.UI.CancelLabel = i18n.L("forms.cancel", "Cancel")
		schema.UI.SuccessMessage = i18n.L("model.form.created", "Model created successfully")
	case formschema.ModeEdit:
		if existing != nil {
			schema.Endpoint = &formschema.SchemaEndpoint{
				Method: "PUT",
				URL:    fmt.Sprintf("/projects/%s/models/%s", projectID, existing.ID),
			}
		}
		schema.UI.SubmitLabel = i18n.L("forms.save_changes", "Save Changes")
		schema.UI.CancelLabel = i18n.L("forms.cancel", "Cancel")
		schema.UI.SuccessMessage = i18n.L("model.form.updated", "Model updated successfully")
	case formschema.ModeView:
		schema.UI.SubmitLabel = nil
	}
	schema.Sections = []formschema.Section{
		{ID: "identity", Fields: buildIdentityFields(mode, existing)},
	}
	return schema
}

func buildIdentityFields(mode string, existing *domain.Model) []formschema.FieldDef {
	isEdit := mode == formschema.ModeEdit || mode == formschema.ModeView
	one := 1
	twoHundred := 200

	uiName := formschema.FieldDef{
		Name:     "ui_name",
		Widget:   formschema.WidgetMultilingualText,
		Required: true,
		Readonly: mode == formschema.ModeView,
		Label:    i18n.L("forms.name", "Name"),
		Help:     i18n.L("model.form.name_help", "Display name for this model"),
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
		Help:     i18n.L("model.form.description_help", "Brief description of what this model represents"),
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
		Label:    i18n.L("model.form.ontology_scope", "Ontology Scope"),
		Help:     i18n.L("model.form.ontology_scope_help", "CRM class for this model. Defines the entity's semantic type."),
	}
	if isEdit && existing != nil {
		scope.Value = existing.OntologyScope
	}

	modelType := formschema.FieldDef{
		Name:     "model_type",
		Widget:   formschema.WidgetSelect,
		Required: false,
		Readonly: mode == formschema.ModeView,
		Label:    i18n.L("model.form.model_type", "Type"),
		Help:     i18n.L("model.form.model_type_help", "Classification for browsing. Core models sort to the top of the model list."),
		Options: []formschema.SelectOption{
			{Value: domain.ModelTypeCore, Label: i18n.L("model.type.core", "Core")},
			{Value: domain.ModelTypeAuxiliary, Label: i18n.L("model.type.auxiliary", "Auxiliary")},
			{Value: domain.ModelTypeExample, Label: i18n.L("model.type.example", "Example")},
		},
	}
	if isEdit && existing != nil {
		modelType.Value = existing.ModelType
	}
	if mode == formschema.ModeCreate {
		modelType.Value = domain.ModelTypeCore
	}

	return []formschema.FieldDef{uiName, desc, systemName, scope, modelType}
}
