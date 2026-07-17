package formschema

import (
	"fmt"

	"github.com/pletka-io/pletka/pkg/i18n"
)

// BuildConceptListEntityListSchema creates an EntityListSchema for controlled
// concept lists exposed by a project.
func BuildConceptListEntityListSchema(projectID, lang string, languages []LanguageInfo) *EntityListSchema {
	return &EntityListSchema{
		EntityType:        "concept-list",
		Title:             i18n.L("concept_list.list.title", "Concept lists"),
		DataURL:           fmt.Sprintf("/api/v2/projects/%s/concept-lists", projectID),
		DataKey:           "concept_lists",
		DetailURLTemplate: fmt.Sprintf("/projects/%s/concept-lists/{id}", projectID),
		EditURLTemplate:   fmt.Sprintf("/api/v2/projects/%s/concept-lists/{id}/form-schema", projectID),
		ProjectID:         projectID,
		EmptyState: &EmptyState{
			Icon:    "tag",
			Title:   i18n.L("concept_list.list.empty_title", "No concept lists"),
			Message: i18n.L("concept_list.list.empty_message", "Controlled lists imported or created for this project will appear here."),
		},
		Search: &SearchConfig{
			Placeholder: i18n.L("concept_list.list.search_placeholder", "Search concept lists..."),
			ParamName:   "search",
		},
		Filters: []FilterConfig{
			{
				Key:       "status",
				Label:     i18n.L("common.status", "Status"),
				ParamName: "status",
				Type:      "select",
				Options: []FilterOption{
					{Value: "draft", Label: i18n.L("status.draft", "Draft")},
					{Value: "published", Label: i18n.L("status.published", "Published")},
					{Value: "deprecated", Label: i18n.L("status.deprecated", "Deprecated")},
				},
				OptionsType: "checklist",
				Multi:       true,
			},
			{
				Key:         "vocabulary_id",
				Label:       i18n.L("concept_list.list.source_vocabulary", "Source vocabulary"),
				ParamName:   "vocabulary_id",
				Type:        "select",
				OptionsURL:  fmt.Sprintf("/api/v2/projects/%s/concept-lists/filters/vocabularies", projectID),
				OptionsType: "checklist",
				Multi:       true,
			},
		},
		SortOptions: []SortOption{
			{Value: "ui_name", Label: i18n.L("forms.name", "Name")},
			{Value: "semantic_id", Label: i18n.L("forms.semantic_id", "Semantic ID")},
			{Value: "system_name", Label: i18n.L("forms.system_name", "System Name")},
			{Value: "vocabulary", Label: i18n.L("concept_list.list.source_vocabulary", "Source vocabulary")},
			{Value: "entries", Label: i18n.L("concept_list.list.entries", "Entries")},
			{Value: "bound_fields", Label: i18n.L("concept_list.list.bound_fields", "Bound fields")},
		},
		DefaultSort: "ui_name",
		Pagination: &PaginationConfig{
			PageSize:        50,
			ParamName:       "page",
			PerPageName:     "per_page",
			PageSizeOptions: []int{25, 50, 100},
		},
		RowLayout: EntityRowLayout{
			TitleField:    "ui_name",
			SubtitleField: "description",
			IdentityFields: []IdentityField{
				{Key: "semantic_id", Style: "mono"},
				{Key: "system_name", Style: "code"},
			},
			SemanticBadges: []BadgeConfig{
				{Key: "vocabulary_label", Type: "text", Style: "blue", Label: i18n.L("concept_list.list.source_vocabulary", "Source vocabulary"), HideEmpty: true},
				{Key: "list_type_label", Type: "text", Style: "purple", Label: i18n.L("concept_list.list.parent_term", "Parent term"), HideEmpty: true},
				{Key: "entry_count", Type: "count", Style: "green", Label: i18n.L("concept_list.list.entries", "Entries")},
				{Key: "bound_field_count", Type: "count", Style: "amber", Label: i18n.L("concept_list.list.bound_fields", "Bound fields"), HideEmpty: true},
			},
			ProcessBadges: []BadgeConfig{
				{Key: "status", Type: "status"},
			},
		},
		Capabilities: &Capabilities{
			Create: &CreateCap{
				Label:         i18n.L("concept_list.list.add", "Add Concept List"),
				FormSchemaURL: fmt.Sprintf("/api/v2/projects/%s/concept-lists/form-schema", projectID),
			},
			Edit: &EditCap{
				FormSchemaURLTemplate: fmt.Sprintf("/api/v2/projects/%s/concept-lists/{id}/form-schema", projectID),
			},
			Delete: &DeleteCap{
				URLTemplate: fmt.Sprintf("/api/v2/projects/%s/concept-lists/{id}", projectID),
			},
		},
		RowActions: []RowAction{
			{ID: "edit", Icon: "pencil", Label: i18n.L("common.edit", "Edit")},
			{
				ID:    "delete",
				Icon:  "trash",
				Label: i18n.L("common.delete", "Delete"),
				Style: "danger",
			},
		},
		UI: SchemaUI{
			Languages:   languages,
			PrimaryLang: lang,
		},
	}
}
