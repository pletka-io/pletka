package formschema

import (
	"fmt"

	"github.com/pletka-io/pletka/pkg/i18n"
)

// BuildModelEntityListSchema creates an EntityListSchema for the models list view.
func BuildModelEntityListSchema(projectID, lang string, languages []LanguageInfo) *EntityListSchema {
	return &EntityListSchema{
		EntityType:        "model",
		Title:             i18n.L("common.models", "Models"),
		DataURL:           fmt.Sprintf("/projects/%s/models", projectID),
		DataKey:           "models",
		DetailURLTemplate: fmt.Sprintf("/projects/%s/models/{id}", projectID),
		ProjectID:         projectID,
		EmptyState: &EmptyState{
			Icon:    "cube",
			Title:   i18n.L("model.list.empty_title", "No models"),
			Message: i18n.L("model.list.empty_message", "Create the first model to start composing fields."),
		},
		Search: &SearchConfig{
			Placeholder: i18n.L("model.list.search_placeholder", "Search models..."),
			ParamName:   "search",
		},
		Filters: []FilterConfig{
			{
				Key:       "model_type",
				Label:     i18n.L("model.list.model_type", "Type"),
				ParamName: "model_type",
				Type:      "select",
				Options: []FilterOption{
					{Value: "core", Label: i18n.L("model.type.core", "Core")},
					{Value: "auxiliary", Label: i18n.L("model.type.auxiliary", "Auxiliary")},
					{Value: "example", Label: i18n.L("model.type.example", "Example")},
				},
				OptionsType: "checklist",
			},
			{
				Key:         "scope_class",
				Label:       i18n.L("common.scope_class", "Scope Class"),
				ParamName:   "scope_class",
				Type:        "select",
				OptionsURL:  fmt.Sprintf("/projects/%s/models/filters/scope-classes", projectID),
				OptionsType: "checklist",
				Multi:       true,
			},
			{
				Key:       "origin_kind",
				Label:     i18n.L("common.origin", "Origin"),
				ParamName: "origin_kind",
				Type:      "select",
				Options: []FilterOption{
					{Value: "own", Label: i18n.L("common.origin_state.own", "Own"), Default: true},
					{Value: "forked", Label: i18n.L("common.origin_state.adapted", "Adapted"), Default: true},
					{Value: "adopted", Label: i18n.L("common.origin_state.adopted_explicit", "Adopted (explicit)")},
					{Value: "adopted_reference", Label: i18n.L("common.origin_state.adopted_reference", "Adopted (by reference)")},
				},
				OptionsType: "checklist",
				Multi:       true,
			},
		},
		SortOptions: []SortOption{
			{Value: "ui_name", Label: i18n.L("forms.name", "Name")},
			{Value: "name", Label: i18n.L("forms.system_name", "System Name")},
			{Value: "scope", Label: i18n.L("model.list.ontology_scope", "Ontology Scope")},
		},
		DefaultSort: "ui_name",
		Pagination: &PaginationConfig{
			PageSize:        30,
			ParamName:       "page",
			PerPageName:     "per_page",
			PageSizeOptions: []int{25, 50, 100},
		},
		RowLayout: EntityRowLayout{
			TitleField:    "ui_name",
			SubtitleField: "description",
			// Scope class leads the row as a short-code box; the full
			// prefixed name follows as a purple line-2 pill.
			LeadBox: &LeadBoxConfig{Key: "scope_class_code", Style: "purple"},
			IdentityFields: []IdentityField{
				{Key: "semantic_id", Style: "mono"},
				{Key: "system_name", Style: "code"},
			},
			SemanticBadges: []BadgeConfig{
				{Key: "ontology_scope", Type: "text", Style: "purple"},
				{Key: "model_type", Type: "text", Style: "blue", HideEmpty: true},
				{Key: "field_count", Type: "count", Style: "blue", Label: i18n.L("common.fields", "Fields"), HideEmpty: true},
				{Key: "category_count", Type: "count", Style: "amber", Label: i18n.L("common.categories", "Categories"), HideEmpty: true},
				{Key: "collection_count", Type: "count", Style: "purple", Label: i18n.L("common.collections", "Collections"), HideEmpty: true},
			},
			// Origin label rendering moved into the ownership badge (line
			// above): the schema-driven LocalizedText refactor made
			// origin_label a Translations object, so the legacy text
			// badge that read item.origin_label as a string would
			// stringify to "[object Object]". The ownership badge
			// already calls tr() on the same field with the per-state
			// color.
			ProcessBadges: []BadgeConfig{
				deprecatedFlagBadge(),
				{Key: "status", Type: "status"},
				{Key: "", Type: "ownership"},
			},
		},
		Capabilities: &Capabilities{
			Create: &CreateCap{
				Label:         i18n.L("model.list.add", "Add Model"),
				FormSchemaURL: fmt.Sprintf("/projects/%s/form-schema/model?mode=create", projectID),
			},
			Adopt: &AdoptCap{
				Label:            i18n.L("model.list.adopt", "Adopt existing"),
				OptionsURL:       fmt.Sprintf("/projects/%s/adoptable/model", projectID),
				URL:              fmt.Sprintf("/projects/%s/adoptions", projectID),
				DestinationLabel: i18n.LF("common.adopt_destination", "Adopting into {project}", map[string]string{"project": projectID}),
			},
			Edit: &EditCap{
				FormSchemaURLTemplate: fmt.Sprintf("/projects/%s/form-schema/model?mode=edit&entity_id={id}", projectID),
			},
			Delete: &DeleteCap{
				URLTemplate: fmt.Sprintf("/projects/%s/models/{id}", projectID),
			},
		},
		RowActions: lifecycleRowActions(projectID, "models"),
		UI: SchemaUI{
			Languages:   languages,
			PrimaryLang: lang,
		},
	}
}
