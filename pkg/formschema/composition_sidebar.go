package formschema

import (
	"strconv"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/i18n"
)

type CompositionFieldSidebarInput struct {
	DisplayName            domain.Translations
	Description            domain.Translations
	CategoryID             string
	ExpectedValueType      string
	ExpectedModelIDs       []string
	ExpectedCollectionIDs  []string
	ExpectedConceptListIDs []string
	SetValue               string
	MinOccurs              int
	MaxOccurs              *int
	IsRequired             bool
	IsHidden               bool
	Visibility             string
}

type CompositionFieldSidebarOptions struct {
	EntityType         string
	GroupWidget        string
	CanEdit            bool
	CanHideFields      bool
	CategoryOptions    []SelectOption
	ModelOptions       []SelectOption
	CollectionOptions  []SelectOption
	ConceptListOptions []SelectOption
}

type CompositionCollectionSidebarInput struct {
	Name       domain.Translations
	CategoryID string
}

type CompositionCollectionSidebarOptions struct {
	CanEdit         bool
	CategoryOptions []SelectOption
}

func BuildCompositionFieldSidebarSchema(
	existing *CompositionFieldSidebarInput,
	opts CompositionFieldSidebarOptions,
	lang string,
	languages []LanguageInfo,
) *FormSchema {
	schema := &FormSchema{
		EntityType: "composition-field-override",
		Mode:       ModeOverride,
		UI: SchemaUI{
			Languages:   languages,
			PrimaryLang: lang,
		},
		Sections: []Section{{
			ID:     "override",
			Fields: buildCompositionFieldSidebarFields(existing, opts),
		}},
	}
	return schema
}

func buildCompositionFieldSidebarFields(
	existing *CompositionFieldSidebarInput,
	opts CompositionFieldSidebarOptions,
) []FieldDef {
	readonly := !opts.CanEdit
	fields := []FieldDef{
		{
			Name:     "display_name",
			Widget:   WidgetMultilingualText,
			Readonly: readonly,
			Label:    i18n.L("forms.fields.display_name", "Display name"),
			Value:    translationsValue(existing, func(in *CompositionFieldSidebarInput) domain.Translations { return in.DisplayName }),
		},
		{
			Name:     "description",
			Widget:   WidgetMultilingualTextarea,
			Readonly: readonly,
			Label:    i18n.L("forms.description", "Description"),
			Value:    translationsValue(existing, func(in *CompositionFieldSidebarInput) domain.Translations { return in.Description }),
		},
	}

	if opts.EntityType != "collection" {
		categoryField := FieldDef{
			Name:        "category_id",
			Widget:      WidgetSelect,
			Readonly:    readonly,
			Label:       i18n.L("field_override.form.category", "Category"),
			Options:     opts.CategoryOptions,
			CreateURL:   "/api/v1/drafts",
			CreateLabel: i18n.L("category.form.submit_create", "Create Category"),
			EntityType:  "category",
			Value:       stringValue(existing, func(in *CompositionFieldSidebarInput) string { return in.CategoryID }),
		}
		if opts.GroupWidget == "collection-group" {
			categoryField.Readonly = true
			categoryField.Help = i18n.L("composition_sidebar.category_locked_help", "Category is locked for fields inside a collection group. Move the collection group to change every field together.")
			categoryField.CreateURL = ""
			categoryField.CreateLabel = nil
		}
		fields = append(fields, categoryField)
	}

	switch existingValueType(existing) {
	case "Model", "Reference Model":
		fields = append(fields, FieldDef{
			Name:        "expected_resource_models",
			Widget:      WidgetPillMultiSelect,
			Readonly:    readonly,
			Label:       i18n.L("composition_sidebar.allowed_models", "Allowed models"),
			Help:        i18n.L("composition_sidebar.allowed_models_help", "Pick the models this field may point at. Empty means any model."),
			EntityType:  "model",
			CreateURL:   "/api/v1/drafts",
			CreateLabel: i18n.L("field_override.form.create_draft_model", "Create Draft Model"),
			Options:     opts.ModelOptions,
			Value:       stringSliceValue(existing, func(in *CompositionFieldSidebarInput) []string { return in.ExpectedModelIDs }),
		})
	case "Collection", "Reference Collection":
		fields = append(fields, FieldDef{
			Name:        "expected_collection_models",
			Widget:      WidgetPillMultiSelect,
			Readonly:    readonly,
			Label:       i18n.L("composition_sidebar.allowed_collections", "Allowed collections"),
			Help:        i18n.L("composition_sidebar.allowed_collections_help", "Pick the collections this field may point at. Empty means any collection."),
			EntityType:  "collection",
			CreateURL:   "/api/v1/drafts",
			CreateLabel: i18n.L("field_override.form.create_draft_collection", "Create Draft Collection"),
			Options:     opts.CollectionOptions,
			Value:       stringSliceValue(existing, func(in *CompositionFieldSidebarInput) []string { return in.ExpectedCollectionIDs }),
		})
	case "Concept":
		fields = append(fields, FieldDef{
			Name:        "expected_concept_lists",
			Widget:      WidgetPillMultiSelect,
			Readonly:    readonly,
			Label:       i18n.L("composition_sidebar.allowed_concept_lists", "Allowed concept lists"),
			Help:        i18n.L("composition_sidebar.allowed_concept_lists_help", "Pick the controlled lists this concept field may use. Empty means no controlled list is enforced yet."),
			EntityType:  "concept-list",
			CreateURL:   "/api/v1/drafts",
			CreateLabel: i18n.L("field_override.form.create_draft_concept_list", "Create Draft Concept List"),
			Options:     opts.ConceptListOptions,
			Value:       stringSliceValue(existing, func(in *CompositionFieldSidebarInput) []string { return in.ExpectedConceptListIDs }),
		})
	}

	fields = append(fields,
		FieldDef{
			Name:     "set_value",
			Widget:   WidgetText,
			Readonly: readonly,
			Label:    i18n.L("composition_sidebar.set_value", "Set value"),
			Help:     i18n.L("composition_sidebar.set_value_help", "Force a fixed value at this override level. Leave empty to inherit."),
			Value:    stringValue(existing, func(in *CompositionFieldSidebarInput) string { return in.SetValue }),
		},
		FieldDef{
			Name:     "min_occurs",
			Widget:   WidgetNumber,
			Readonly: readonly,
			Label:    i18n.L("composition_sidebar.min_occurs", "Min occurs"),
			Value:    intStringValue(existing, func(in *CompositionFieldSidebarInput) int { return in.MinOccurs }),
		},
		FieldDef{
			Name:     "max_occurs",
			Widget:   WidgetNumber,
			Readonly: readonly,
			Label:    i18n.L("composition_sidebar.max_occurs", "Max occurs"),
			Help:     i18n.L("composition_sidebar.max_occurs_help", "Leave empty for no maximum."),
			Value:    optionalIntStringValue(existing, func(in *CompositionFieldSidebarInput) *int { return in.MaxOccurs }),
		},
		FieldDef{
			Name:     "is_required",
			Widget:   WidgetCheckbox,
			Readonly: readonly,
			Label:    i18n.L("composition_sidebar.required", "Required"),
			Value:    boolValue(existing, func(in *CompositionFieldSidebarInput) bool { return in.IsRequired }),
		},
	)

	if opts.CanHideFields {
		fields = append(fields, FieldDef{
			Name:     "is_hidden",
			Widget:   WidgetCheckbox,
			Readonly: readonly,
			Label:    i18n.L("composition_sidebar.hidden", "Hidden"),
			Value:    boolValue(existing, func(in *CompositionFieldSidebarInput) bool { return in.IsHidden }),
		})
	}

	fields = append(fields, FieldDef{
		Name:     "visibility",
		Widget:   WidgetSelect,
		Readonly: readonly,
		Label:    i18n.L("composition_sidebar.visibility_label", "Visibility"),
		Options: []SelectOption{
			{Value: "", Label: i18n.L("composition_sidebar.visibility.inherit", "inherit")},
			{Value: "public", Label: i18n.L("composition_sidebar.visibility.public", "public")},
			{Value: "internal", Label: i18n.L("composition_sidebar.visibility.internal", "internal")},
			{Value: "private", Label: i18n.L("composition_sidebar.visibility.private", "private")},
		},
		Value: stringValue(existing, func(in *CompositionFieldSidebarInput) string { return in.Visibility }),
	})

	return fields
}

func BuildCompositionCollectionSidebarSchema(
	existing *CompositionCollectionSidebarInput,
	opts CompositionCollectionSidebarOptions,
	lang string,
	languages []LanguageInfo,
) *FormSchema {
	return &FormSchema{
		EntityType: "composition-collection-group",
		Mode:       ModeOverride,
		UI: SchemaUI{
			Languages:   languages,
			PrimaryLang: lang,
		},
		Sections: []Section{{
			ID: "override",
			Fields: []FieldDef{
				{
					Name:     "name",
					Widget:   WidgetMultilingualText,
					Readonly: !opts.CanEdit,
					Label:    i18n.L("composition_sidebar.collection_name", "Collection name"),
					Help:     i18n.L("composition_sidebar.collection_name_help", "Override the collection name in this context. Empty languages fall back to the base collection name."),
					Value:    translationsValue(existing, func(in *CompositionCollectionSidebarInput) domain.Translations { return in.Name }),
				},
				{
					Name:        "category_id",
					Widget:      WidgetSelect,
					Readonly:    !opts.CanEdit,
					Label:       i18n.L("field_override.form.category", "Category"),
					Help:        i18n.L("composition_sidebar.collection_category_help", "Moving the category moves the whole collection group and every field inside it."),
					Options:     opts.CategoryOptions,
					CreateURL:   "/api/v1/drafts",
					CreateLabel: i18n.L("category.form.submit_create", "Create Category"),
					EntityType:  "category",
					Value:       stringValue(existing, func(in *CompositionCollectionSidebarInput) string { return in.CategoryID }),
				},
			},
		}},
	}
}

func existingValueType(existing *CompositionFieldSidebarInput) string {
	if existing == nil {
		return ""
	}
	return existing.ExpectedValueType
}

func translationsValue[T any](existing *T, getter func(*T) domain.Translations) any {
	if existing == nil {
		return domain.Translations{}
	}
	return getter(existing)
}

func stringValue[T any](existing *T, getter func(*T) string) any {
	if existing == nil {
		return ""
	}
	return getter(existing)
}

func stringSliceValue[T any](existing *T, getter func(*T) []string) any {
	if existing == nil {
		return []string{}
	}
	return getter(existing)
}

func boolValue[T any](existing *T, getter func(*T) bool) any {
	if existing == nil {
		return false
	}
	return getter(existing)
}

func intStringValue[T any](existing *T, getter func(*T) int) any {
	if existing == nil {
		return "0"
	}
	return strconv.Itoa(getter(existing))
}

func optionalIntStringValue[T any](existing *T, getter func(*T) *int) any {
	if existing == nil {
		return ""
	}
	v := getter(existing)
	if v == nil {
		return ""
	}
	return strconv.Itoa(*v)
}
