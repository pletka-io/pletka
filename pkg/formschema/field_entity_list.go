package formschema

import (
	"fmt"

	"github.com/pletka-io/pletka/pkg/i18n"
)

// BuildFieldEntityListSchema creates an EntityListSchema for the fields list view.
func BuildFieldEntityListSchema(projectID, lang string, languages []LanguageInfo) *EntityListSchema {
	return &EntityListSchema{
		EntityType:        "field",
		Title:             i18n.L("field.list.title", "Fields"),
		DataURL:           fmt.Sprintf("/projects/%s/fields", projectID),
		DataKey:           "fields",
		DetailURLTemplate: fmt.Sprintf("/projects/%s/fields/{id}", projectID),
		ProjectID:         projectID,
		EmptyState: &EmptyState{
			Icon:    "field",
			Title:   i18n.L("field.list.empty_title", "No fields"),
			Message: i18n.L("field.list.empty_message", "Get started by creating your first field."),
		},
		Search: &SearchConfig{
			Placeholder: i18n.L("field.list.search_placeholder", "Search fields..."),
			ParamName:   "search",
		},
		Filters: []FilterConfig{
			{
				Key:         "ontology_scope",
				Label:       i18n.L("model.list.ontology_scope", "Ontology Scope"),
				ParamName:   "ontology_scope",
				Type:        "select",
				OptionsURL:  fmt.Sprintf("/projects/%s/fields/filters/ontology-scopes", projectID),
				OptionsType: "checklist",
				Multi:       true,
			},
			{
				Key:         "category_id",
				Label:       i18n.L("common.category", "Category"),
				ParamName:   "category_id",
				Type:        "select",
				OptionsURL:  fmt.Sprintf("/projects/%s/fields/filters/categories", projectID),
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
			PageSize:        50,
			ParamName:       "page",
			PerPageName:     "per_page",
			PageSizeOptions: []int{25, 50, 100},
		},
		ViewModes: []ViewMode{
			{ID: "detailed", Label: i18n.L("namespace_binding.list.view_detailed", "Detailed"), Default: true},
			{ID: "compact", Label: i18n.L("common.view_compact", "Compact")},
		},
		RowLayout: EntityRowLayout{
			TitleField:    "ui_name",
			SubtitleField: "description",
			// Scope class leads the row as a short-code box; the full
			// prefixed name follows as a purple line-2 pill.
			LeadBox: &LeadBoxConfig{Key: "scope_class_code", Style: "purple"},
			// The field's ontology path renders as a class/property pill chain
			// on its own line (detailed view) — the default row handles this via
			// PathField, so fields no longer need a bespoke FieldCard widget.
			PathField: "path_elements",
			IdentityFields: []IdentityField{
				{Key: "semantic_id", Style: "mono"},
				{Key: "system_name", Style: "code"},
			},
			SemanticBadges: []BadgeConfig{
				{Key: "ontology_scope", Type: "text", Style: "purple"},
				{Key: "expected_value_type", Type: "text", Style: "blue"},
				// Reuse signal for cleanup: how many models/collections use the
				// field here, and how many OTHER projects reuse it.
				{Key: "model_count", Type: "count", Style: "blue", Label: i18n.L("field.list.model_usage", "models"), HideEmpty: true},
				{Key: "collection_count", Type: "count", Style: "purple", Label: i18n.L("field.list.collection_usage", "collections"), HideEmpty: true},
				{Key: "other_project_count", Type: "count", Style: "amber", Label: i18n.L("field.list.other_projects", "other projects"), HideEmpty: true},
			},
			ProcessBadges: []BadgeConfig{
				deprecatedFlagBadge(),
				{Key: "status", Type: "status"},
				{Key: "", Type: "ownership"},
			},
		},
		Capabilities: &Capabilities{
			Create: &CreateCap{
				Label:         i18n.L("field.list.add", "Add Field"),
				FormSchemaURL: fmt.Sprintf("/projects/%s/form-schema/field?mode=create", projectID),
			},
			Edit: &EditCap{
				FormSchemaURLTemplate: fmt.Sprintf("/projects/%s/form-schema/field?mode=edit&entity_id={id}", projectID),
			},
			Delete: &DeleteCap{
				URLTemplate: fmt.Sprintf("/projects/%s/fields/{id}", projectID),
			},
		},
		RowActions: lifecycleRowActions(projectID, "fields"),
		UI: SchemaUI{
			Languages:   languages,
			PrimaryLang: lang,
		},
	}
}
