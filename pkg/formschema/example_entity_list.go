package formschema

import (
	"fmt"
	"net/url"

	"github.com/pletka-io/pletka/pkg/frontendrefs"
	"github.com/pletka-io/pletka/pkg/i18n"
)

// BuildExampleEntityListSchema creates an EntityListSchema for project examples.
func BuildExampleEntityListSchema(projectID, lang string, languages []LanguageInfo, entityType, entityID string) *EntityListSchema {
	dataURL := fmt.Sprintf("/projects/%s/examples", projectID)
	if entityType != "" || entityID != "" {
		params := url.Values{}
		if entityType != "" {
			params.Set("entity_type", entityType)
		}
		if entityID != "" {
			params.Set("entity_id", entityID)
		}
		dataURL += "?" + params.Encode()
	}
	return &EntityListSchema{
		EntityType:        "example",
		Title:             i18n.L("common.examples", "Examples"),
		DataURL:           dataURL,
		DataKey:           "items",
		DetailURLTemplate: "",
		ProjectID:         projectID,
		EmptyState: &EmptyState{
			Icon:    "document-text",
			Title:   i18n.L("example.list.empty_title", "No examples"),
			Message: i18n.L("example.list.empty_message", "Create a worked example to test whether your model can actually be filled in."),
		},
		Search: &SearchConfig{
			Placeholder: i18n.L("example.list.search_placeholder", "Search examples..."),
			ParamName:   "search",
		},
		Filters: []FilterConfig{
			{
				Key:       "status",
				Label:     i18n.L("common.status", "Status"),
				ParamName: "status",
				Type:      "select",
				Options: []FilterOption{
					{Value: "draft", Label: i18n.L("common.draft", "Draft")},
					{Value: "valid", Label: i18n.L("common.valid", "Valid")},
					{Value: "has_issues", Label: i18n.L("common.has_issues", "Has Issues")},
				},
				OptionsType: "checklist",
			},
		},
		SortOptions: []SortOption{
			{Value: "updated_at", Label: i18n.L("common.updated", "Updated")},
			{Value: "created_at", Label: i18n.L("common.created", "Created")},
			{Value: "status", Label: i18n.L("common.status", "Status")},
		},
		DefaultSort: "updated_at",
		Pagination: &PaginationConfig{
			PageSize:        30,
			ParamName:       "page",
			PerPageName:     "per_page",
			PageSizeOptions: []int{25, 50, 100},
		},
		Editor: &EntityListEditor{
			Widget: frontendrefs.EntityListEditorWidget("example-workspace"),
			// URLs the example-workspace editor calls; the widget fills
			// {id} client-side (compliant template substitution).
			Endpoints: map[string]string{
				"list_url":            fmt.Sprintf("/projects/%s/examples", projectID),
				"detail_url_template": fmt.Sprintf("/projects/%s/examples/{id}", projectID),
				"form_schema_url":     fmt.Sprintf("/projects/%s/examples/form-schema", projectID),
				// Browser-facing URL of one example; the route redirects a
				// browser to the project page with the item open.
				"page_url_template": fmt.Sprintf("/projects/%s/examples/{id}", projectID),
			},
		},
		RowLayout: EntityRowLayout{
			TitleField:    "title",
			SubtitleField: "description",
			IdentityFields: []IdentityField{
				{Key: "entity_name"},
				{Key: "entity_id", Style: "mono"},
			},
			SemanticBadges: []BadgeConfig{
				{Key: "entity_type_label", Type: "text", Style: "blue"},
			},
			ProcessBadges: []BadgeConfig{
				{Key: "status", Type: "status"},
			},
		},
		Capabilities: &Capabilities{
			Create: &CreateCap{
				Label:         i18n.L("example.list.add", "Add Example"),
				FormSchemaURL: "",
			},
			Edit: &EditCap{
				FormSchemaURLTemplate: "",
			},
			Delete: &DeleteCap{
				URLTemplate: fmt.Sprintf("/projects/%s/examples/{id}", projectID),
			},
		},
		RowActions: []RowAction{
			{
				ID:    "edit",
				Icon:  "pencil",
				Label: i18n.L("common.edit", "Edit"),
			},
		},
		UI: SchemaUI{
			Languages:   languages,
			PrimaryLang: lang,
		},
	}
}
