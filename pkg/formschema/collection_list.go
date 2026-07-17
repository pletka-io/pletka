package formschema

import (
	"fmt"

	"github.com/pletka-io/pletka/pkg/i18n"
)

// BuildCollectionEntityListSchema creates an EntityListSchema for the collections list view.
func BuildCollectionEntityListSchema(projectID, lang string, languages []LanguageInfo) *EntityListSchema {
	return &EntityListSchema{
		EntityType:        "collection",
		Title:             i18n.L("common.collections", "Collections"),
		DataURL:           fmt.Sprintf("/projects/%s/collections", projectID),
		DataKey:           "collections",
		DetailURLTemplate: fmt.Sprintf("/projects/%s/collections/{id}", projectID),
		ProjectID:         projectID,
		EmptyState: &EmptyState{
			Icon:    "collection",
			Title:   i18n.L("collection.list.empty_title", "No collections"),
			Message: i18n.L("collection.list.empty_message", "Create the first collection to group fields by ontology context."),
		},
		Search: &SearchConfig{
			Placeholder: i18n.L("collection.list.search_placeholder", "Search collections..."),
			ParamName:   "search",
		},
		Filters: []FilterConfig{
			{
				Key:         "scope_class",
				Label:       i18n.L("common.scope_class", "Scope Class"),
				ParamName:   "scope_class",
				Type:        "select",
				OptionsURL:  fmt.Sprintf("/projects/%s/collections/filters/scope-classes", projectID),
				OptionsType: "checklist",
				Multi:       true,
			},
			{
				Key:         "category_id",
				Label:       i18n.L("common.category", "Category"),
				ParamName:   "category_id",
				Type:        "select",
				OptionsURL:  fmt.Sprintf("/projects/%s/collections/filters/categories", projectID),
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
				{Key: "field_count", Type: "count", Style: "blue", Label: i18n.L("common.fields", "Fields"), HideEmpty: true},
			},
			// See model_list.go for why the origin_label text badge is
			// gone — the ownership badge already renders the
			// LocalizedText via tr().
			ProcessBadges: []BadgeConfig{
				deprecatedFlagBadge(),
				{Key: "status", Type: "status"},
				{Key: "", Type: "ownership"},
			},
		},
		Capabilities: &Capabilities{
			Create: &CreateCap{
				Label:         i18n.L("collection.list.add", "Add Collection"),
				FormSchemaURL: fmt.Sprintf("/projects/%s/form-schema/collection?mode=create", projectID),
			},
			Adopt: &AdoptCap{
				Label:            i18n.L("collection.list.adopt", "Adopt existing"),
				OptionsURL:       fmt.Sprintf("/projects/%s/adoptable/collection", projectID),
				URL:              fmt.Sprintf("/projects/%s/adoptions", projectID),
				DestinationLabel: i18n.LF("common.adopt_destination", "Adopting into {project}", map[string]string{"project": projectID}),
			},
			Edit: &EditCap{
				FormSchemaURLTemplate: fmt.Sprintf("/projects/%s/form-schema/collection?mode=edit&entity_id={id}", projectID),
			},
			Delete: &DeleteCap{
				URLTemplate: fmt.Sprintf("/projects/%s/collections/{id}", projectID),
			},
		},
		RowActions: lifecycleRowActions(projectID, "collections"),
		UI: SchemaUI{
			Languages:   languages,
			PrimaryLang: lang,
		},
	}
}
