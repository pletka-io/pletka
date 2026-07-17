package field

import (
	"fmt"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/formschema"
	"github.com/pletka-io/pletka/pkg/i18n"
)

// BuildFormSchema constructs the create/edit form schema for a field.
// Canonical owner of the field form schema (per ADR-0001 — slices own
// their schemas; entityschema dispatcher delegates here).
//
// existing is *formschema.ComposedFieldInput so the user-facing schema can
// compose base-field and base-override inputs while preserving distinct
// storage ownership underneath.
func BuildFormSchema(mode string, existing *formschema.ComposedFieldInput, projectID, lang string, languages []formschema.LanguageInfo) *formschema.FormSchema {
	schema := &formschema.FormSchema{
		EntityType: "field",
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
			URL:    fmt.Sprintf("/projects/%s/fields", projectID),
		}
		schema.UI.SubmitLabel = i18n.L("field.form.submit_create", "Create Field")
		schema.UI.CancelLabel = i18n.L("forms.cancel", "Cancel")
		schema.UI.SuccessMessage = i18n.L("field.form.created", "Field created successfully")
	case formschema.ModeEdit:
		if existing != nil {
			fieldURL := fmt.Sprintf("/projects/%s/fields/%s", projectID, existing.Field.ID)
			schema.Endpoint = &formschema.SchemaEndpoint{
				Method: "PUT",
				URL:    fieldURL,
			}
			schema.Delete = &formschema.DeleteAction{
				URL:                fieldURL,
				Label:              i18n.L("field.form.delete_label", "Delete Field"),
				SuccessRedirectURL: fmt.Sprintf("/projects/%s#tab=fields", projectID),
				SuccessMessage:     i18n.L("field.form.deleted", "Field deleted"),
				Confirm: &formschema.ConfirmConfig{
					Title:        i18n.L("field.form.delete_confirm_title", "Delete this field?"),
					Message:      i18n.L("field.form.delete_confirm_message", "This removes the field from the project. Existing data referencing it will lose the link. This cannot be undone."),
					ConfirmLabel: i18n.L("forms.confirm_delete", "Delete"),
				},
			}
		}
		schema.UI.SubmitLabel = i18n.L("forms.save_changes", "Save Changes")
		schema.UI.CancelLabel = i18n.L("forms.cancel", "Cancel")
		schema.UI.SuccessMessage = i18n.L("field.form.updated", "Field updated successfully")
	case formschema.ModeView:
		schema.UI.SubmitLabel = nil
	}

	schema.Sections = []formschema.Section{
		{ID: "identity", Fields: buildIdentityFields(mode, existing)},
		{
			ID:     "ontology",
			Label:  i18n.L("field.form.section_ontology", "Ontology Path"),
			Fields: buildOntologyFields(mode, existing),
		},
	}
	overrideSchema := formschema.BuildFieldOverrideSchema(mode, overrideInput(existing), projectID, languages, lang)
	if len(overrideSchema.Sections) > 0 {
		schema.Sections = append(schema.Sections, overrideSchema.Sections...)
	}
	return schema
}

func buildIdentityFields(mode string, existing *formschema.ComposedFieldInput) []formschema.FieldDef {
	isEdit := mode == formschema.ModeEdit || mode == formschema.ModeView
	one := 1
	twoHundred := 200

	uiName := formschema.FieldDef{
		Name:     "ui_name",
		Widget:   formschema.WidgetMultilingualText,
		Required: true,
		Readonly: mode == formschema.ModeView,
		Label:    i18n.L("forms.name", "Name"),
		Help:     i18n.L("field.form.name_help", "Display name for this field"),
		Validation: &formschema.ValidationRules{
			MinLength: &one,
			MaxLength: &twoHundred,
		},
	}
	if isEdit && existing != nil {
		uiName.Value = existing.Field.UIName
	}

	desc := formschema.FieldDef{
		Name:     "description",
		Widget:   formschema.WidgetMultilingualTextarea,
		Required: false,
		Readonly: mode == formschema.ModeView,
		Label:    i18n.L("forms.description", "Description"),
		Help:     i18n.L("field.form.description_help", "Brief description of this field's purpose"),
	}
	if isEdit && existing != nil {
		desc.Value = existing.Field.Description
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
		systemName.Value = existing.Field.SystemName
	}

	return []formschema.FieldDef{uiName, desc, systemName}
}

func buildOntologyFields(mode string, existing *formschema.ComposedFieldInput) []formschema.FieldDef {
	isEdit := mode == formschema.ModeEdit || mode == formschema.ModeView

	scope := formschema.FieldDef{
		Name:     "ontology_scope",
		Widget:   formschema.WidgetOntologyPath,
		Required: true,
		Readonly: mode == formschema.ModeView,
		Label:    i18n.L("common.scope_class", "Scope Class"),
		Help:     i18n.L("field.form.scope_class_help", "The root CRM class for this field (e.g. crm:E21_Person)."),
	}
	if isEdit && existing != nil {
		scope.Value = existing.Field.OntologyScope
	}

	path := formschema.FieldDef{
		Name:     "ontology_path",
		Widget:   formschema.WidgetOntologyPath,
		Required: true,
		Readonly: mode == formschema.ModeView,
		Label:    i18n.L("field.form.ontology_path", "Ontology Path"),
		Help:     i18n.L("field.form.ontology_path_help", "Property chain from the scope class to the value."),
	}
	if isEdit && existing != nil {
		path.Value = existing.Field.PathElements
	}

	fields := []formschema.FieldDef{scope, path}

	// Legacy <br><br> subfield paths: read-only display, only when present.
	// No authoring affordance — subfields cannot be created or edited.
	if isEdit && existing != nil && len(existing.Field.SubfieldPaths) > 0 {
		fields = append(fields, formschema.FieldDef{
			Name:     "subfield_paths",
			Widget:   formschema.WidgetSubfieldPaths,
			Readonly: true,
			Label:    i18n.L("field.form.subfield_paths", "Legacy Subfield Paths"),
			Help:     i18n.L("field.form.subfield_paths_help", "Additional ontology paths imported from a single legacy field cell. Read-only — split them into separate fields in the source to remove."),
			Value:    existing.Field.SubfieldPaths,
		})
	}

	return fields
}

func overrideInput(existing *formschema.ComposedFieldInput) *formschema.FieldOverrideInput {
	if existing == nil {
		return nil
	}
	return &existing.Override
}

// ComposedFieldInputFromDomain maps a *domain.Field plus optional override
// input onto the composed form input shape used by the field schema.
func ComposedFieldInputFromDomain(f *domain.Field, override *formschema.FieldOverrideInput) *formschema.ComposedFieldInput {
	if f == nil {
		return nil
	}
	out := &formschema.ComposedFieldInput{
		Field: formschema.FieldInput{
			ID:                f.ID,
			SystemName:        f.SystemName,
			UIName:            f.UIName,
			Description:       f.Description,
			OntologyScope:     f.OntologyScope,
			PathElements:      f.PathElements,
			SubfieldPaths:     f.SubfieldPaths,
			ExpectedValueType: f.ExpectedValueType,
		},
	}
	if override != nil {
		out.Override = *override
	} else {
		out.Override.ExpectedValueType = f.ExpectedValueType
	}
	if out.Override.ExpectedValueType == "" {
		out.Override.ExpectedValueType = f.ExpectedValueType
	}
	return out
}
