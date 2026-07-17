package formschema

import (
	"github.com/pletka-io/pletka/pkg/i18n"
)

// BuildFieldOverrideSchema constructs the contextual override form for a field.
// The user-facing composed field form includes expected_value_type here so it
// can drive conditional inputs, even though that value is still stored on the
// base Field rather than on the override row.
func BuildFieldOverrideSchema(mode string, existing *FieldOverrideInput, projectID string, languages []LanguageInfo, lang string) *FormSchema {
	schema := &FormSchema{
		EntityType: "field-override",
		Mode:       mode,
		UI: SchemaUI{
			Languages:   languages,
			PrimaryLang: lang,
		},
	}

	switch mode {
	case ModeCreate:
		schema.UI.SubmitLabel = i18n.L("field_override.form.submit_create", "Create Override")
		schema.UI.CancelLabel = i18n.L("forms.cancel", "Cancel")
	case ModeEdit:
		schema.UI.SubmitLabel = i18n.L("field_override.form.submit_save", "Save Override")
		schema.UI.CancelLabel = i18n.L("forms.cancel", "Cancel")
	case ModeView:
		schema.UI.SubmitLabel = nil
	}

	schema.Sections = []Section{
		{
			ID:     "override",
			Label:  i18n.L("field_override.form.section", "Override"),
			Fields: buildFieldOverrideFields(mode, existing, projectID),
		},
	}

	return schema
}

func buildFieldOverrideFields(mode string, existing *FieldOverrideInput, projectID string) []FieldDef {
	expectedValueTypeField := FieldDef{
		Name:     "expected_value_type",
		Widget:   WidgetSelect,
		Required: false,
		Readonly: mode == ModeView,
		Label:    i18n.L("field_override.form.expected_value_type", "Expected Value Type"),
		Options: []SelectOption{
			{Value: "String", Label: i18n.L("field_override.value_type.string", "String")},
			{Value: "URI", Label: i18n.L("field_override.value_type.uri", "URI")},
			{Value: "Date", Label: i18n.L("field_override.value_type.date", "Date")},
			{Value: "Integer", Label: i18n.L("field_override.value_type.integer", "Integer")},
			{Value: "Model", Label: i18n.L("field_override.value_type.model", "Model")},
			{Value: "Collection", Label: i18n.L("field_override.value_type.collection", "Collection")},
			{Value: "Concept", Label: i18n.L("field_override.value_type.concept", "Concept")},
			{Value: "GeoJson", Label: i18n.L("field_override.value_type.geojson", "GeoJson")},
			{Value: "file/bitstream", Label: i18n.L("field_override.value_type.file", "File/Bitstream")},
		},
	}
	if existing != nil {
		expectedValueTypeField.Value = existing.ExpectedValueType
	}

	modelField := FieldDef{
		Name:        "expected_resource_models",
		Widget:      WidgetPillMultiSelect,
		Required:    false,
		Readonly:    mode == ModeView,
		Label:       i18n.L("field_override.form.expected_models", "Expected Models"),
		EntityType:  "model",
		OptionsURL:  "/projects/" + projectID + "/models/options",
		CreateURL:   "/api/v1/drafts",
		CreateLabel: i18n.L("field_override.form.create_draft_model", "Create Draft Model"),
		VisibleWhen: &VisibilityRule{Field: "expected_value_type", Equals: "Model"},
	}
	if existing != nil {
		modelField.Value = existing.ExpectedModelIDs
	}

	collectionField := FieldDef{
		Name:        "expected_collection_models",
		Widget:      WidgetPillMultiSelect,
		Required:    false,
		Readonly:    mode == ModeView,
		Label:       i18n.L("field_override.form.expected_collections", "Expected Collections"),
		EntityType:  "collection",
		OptionsURL:  "/projects/" + projectID + "/collections/options",
		CreateURL:   "/api/v1/drafts",
		CreateLabel: i18n.L("field_override.form.create_draft_collection", "Create Draft Collection"),
		VisibleWhen: &VisibilityRule{Field: "expected_value_type", Equals: "Collection"},
	}
	if existing != nil {
		collectionField.Value = existing.ExpectedCollectionIDs
	}

	setValueField := FieldDef{
		Name:     "set_value",
		Widget:   WidgetText,
		Required: false,
		Readonly: mode == ModeView,
		Label:    i18n.L("field_override.form.set_value", "Set Value"),
	}
	if existing != nil {
		setValueField.Value = existing.SetValue
	}

	categoryField := FieldDef{
		Name:        "category_id",
		Widget:      WidgetSelect,
		Required:    true,
		Readonly:    mode == ModeView,
		Label:       i18n.L("field_override.form.category", "Category"),
		EntityType:  "category",
		OptionsURL:  "/projects/" + projectID + "/categories/options",
		CreateURL:   "/api/v1/drafts",
		CreateLabel: i18n.L("category.form.submit_create", "Create Category"),
	}
	if existing != nil {
		categoryField.Value = existing.CategoryID
	}

	return []FieldDef{
		expectedValueTypeField,
		modelField,
		collectionField,
		setValueField,
		categoryField,
	}
}
