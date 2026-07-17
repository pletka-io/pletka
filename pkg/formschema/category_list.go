package formschema

import (
	"fmt"

	"github.com/pletka-io/pletka/pkg/i18n"
)

// BuildCategoryListSchema constructs the list schema for category management.
func BuildCategoryListSchema(projectID string, lang string, languages []LanguageInfo) *ListSchema {
	pid := projectID

	return &ListSchema{
		EntityType: "category",
		Title:      i18n.L("category.list.title", "Categories"),
		EmptyState: &EmptyState{
			Icon:    "tag",
			Title:   i18n.L("category.list.empty_title", "No categories"),
			Message: i18n.L("category.list.empty_message", "Get started by creating your first category."),
		},
		DataURL: fmt.Sprintf("/projects/%s/categories", pid),
		DataKey: "categories",
		Caps: Capabilities{
			Reorder: &ReorderCap{
				Enabled:    true,
				URL:        fmt.Sprintf("/projects/%s/categories/reorder", pid),
				OrderField: "category_ids",
			},
			InlineRename: &InlineRenameCap{
				Enabled:         true,
				Field:           "ui_name",
				SaveURLTemplate: fmt.Sprintf("/projects/%s/categories/{id}", pid),
			},
			Create: &CreateCap{
				Label:         i18n.L("category.list.add", "Add Category"),
				FormSchemaURL: fmt.Sprintf("/projects/%s/form-schema/category?mode=create", pid),
			},
			Edit: &EditCap{
				FormSchemaURLTemplate: fmt.Sprintf("/projects/%s/form-schema/category?mode=edit&entity_id={id}", pid),
			},
			Delete: &DeleteCap{
				URLTemplate: fmt.Sprintf("/projects/%s/categories/{id}", pid),
				Reassignment: &ReassignCap{
					Enabled:     true,
					EntityLabel: i18n.L("category.list.reassign_entity_label", "fields"),
					CountField:  "field_count",
					OptionsFrom: "siblings",
				},
			},
			Stats: &StatsCap{
				URLTemplate: fmt.Sprintf("/projects/%s/categories/{id}/stats", pid),
			},
		},
		Columns: []Column{
			{
				Key:     "ui_name",
				Label:   i18n.L("forms.name", "Name"),
				Type:    "translation",
				Primary: true,
			},
			{
				Key:       "description",
				Label:     i18n.L("forms.description", "Description"),
				Type:      "translation",
				Secondary: true,
				Truncate:  true,
			},
			{
				Key:        "field_count",
				Label:      i18n.L("common.fields", "Fields"),
				Type:       "badge",
				BadgeStyle: "blue",
				ZeroStyle:  "gray",
			},
			{
				Key:        "_overrides",
				Label:      i18n.L("category.list.overrides", "Overrides"),
				Type:       "computed_badge",
				Compute:    "model_field_count + collection_field_count",
				BadgeStyle: "purple",
				HideZero:   true,
			},
		},
		RowActions: []RowAction{
			{ID: "edit", Icon: "pencil", Label: i18n.L("common.edit", "Edit")},
			{ID: "stats", Icon: "chart-bar", Label: i18n.L("common.statistics", "Statistics")},
			{ID: "delete", Icon: "trash", Label: i18n.L("common.delete", "Delete"), Style: "danger"},
		},
		UI: SchemaUI{
			Languages:   languages,
			PrimaryLang: lang,
		},
	}
}
