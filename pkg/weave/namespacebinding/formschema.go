package namespacebinding

import (
	"fmt"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/formschema"
	"github.com/pletka-io/pletka/pkg/frontendrefs"
	"github.com/pletka-io/pletka/pkg/i18n"
)

// Form-schema builders for the namespace-binding slice.
//
// All URLs are absolute and point at the slice's mount. Frontend never
// constructs API paths — every URL it uses comes through the schema.

// BuildListSchema returns the ListSchema for the bindings pane. System /
// imported rows are marked read-only via PerRowReadonlyField so the
// ListManager hides their row actions. No deprecation / lifecycle on
// bindings — they're either user-mutable or read-only.
func BuildListSchema(projectID, lang string, languages []formschema.LanguageInfo) *formschema.ListSchema {
	pid := projectID
	return &formschema.ListSchema{
		EntityType: "namespace-binding",
		Title:      i18n.L("namespace_binding.list.title", "Namespace bindings"),
		EmptyState: &formschema.EmptyState{
			Icon:    "link",
			Title:   i18n.L("namespace_binding.list.empty_title", "No namespace bindings"),
			Message: i18n.L("namespace_binding.list.empty_message", "Add a prefix-to-namespace binding to get started."),
		},
		DataURL:             fmt.Sprintf("/projects/%s/namespace-bindings", pid),
		PerRowReadonlyField: "_readonly",
		Caps: formschema.Capabilities{
			Create: &formschema.CreateCap{
				Label:         i18n.L("namespace_binding.list.add", "Add binding"),
				FormSchemaURL: fmt.Sprintf("/projects/%s/namespace-bindings/form-schema", pid),
			},
			Edit: &formschema.EditCap{
				FormSchemaURLTemplate: fmt.Sprintf("/projects/%s/namespace-bindings/{id}/form-schema", pid),
			},
			Delete: &formschema.DeleteCap{
				URLTemplate: fmt.Sprintf("/projects/%s/namespace-bindings/{id}", pid),
			},
		},
		Columns: []formschema.Column{
			{
				Key:     "prefix",
				Label:   i18n.L("namespace_binding.list.prefix", "Prefix"),
				Type:    "text",
				Primary: true,
			},
			{
				Key:       "namespace",
				Label:     i18n.L("namespace_binding.list.namespace", "Namespace"),
				Type:      "text",
				Secondary: true,
			},
			{
				Key:        "weight",
				Label:      i18n.L("namespace_binding.list.weight", "Weight"),
				Type:       "badge",
				BadgeStyle: "gray",
			},
			{
				Key:   "source",
				Label: i18n.L("namespace_binding.list.source", "Source"),
				Type:  "text_badge",
				BadgeStyleByValue: map[string]string{
					"system":   "gray",
					"user":     "blue",
					"ontology": "purple",
					"manifest": "green",
				},
			},
		},
		RowActions: []formschema.RowAction{
			{ID: "edit", Icon: "pencil", Label: i18n.L("common.edit", "Edit")},
			{
				ID:    "delete",
				Icon:  "trash",
				Label: i18n.L("common.remove", "Remove"),
				Style: "danger",
			},
		},
		UI: formschema.SchemaUI{
			Languages:   languages,
			PrimaryLang: lang,
		},
	}
}

func BuildGlobalListSchema(lang string, languages []formschema.LanguageInfo) *formschema.ListSchema {
	return &formschema.ListSchema{
		EntityType: "namespace-binding",
		Title:      i18n.L("namespace_binding.list.global_title", "Global namespaces"),
		EmptyState: &formschema.EmptyState{
			Icon:    "link",
			Title:   i18n.L("namespace_binding.list.global_empty_title", "No global namespaces"),
			Message: i18n.L("namespace_binding.list.global_empty_message", "Add a shared namespace binding for all projects."),
		},
		DataURL: "/admin/namespaces",
		Caps: formschema.Capabilities{
			Create: &formschema.CreateCap{
				Label:         i18n.L("namespace_binding.list.global_add", "Add namespace"),
				FormSchemaURL: "/admin/namespaces/form-schema",
			},
			Edit: &formschema.EditCap{
				FormSchemaURLTemplate: "/admin/namespaces/{id}/form-schema",
			},
			Delete: &formschema.DeleteCap{
				URLTemplate: "/admin/namespaces/{id}",
			},
			Stats: &formschema.StatsCap{
				URLTemplate: "/admin/namespaces/{id}/stats",
			},
		},
		Columns: []formschema.Column{
			{
				Key:     "prefix",
				Label:   i18n.L("namespace_binding.list.prefix", "Prefix"),
				Type:    "text",
				Primary: true,
			},
			{
				Key:       "namespace",
				Label:     i18n.L("namespace_binding.list.namespace", "Namespace"),
				Type:      "text",
				Secondary: true,
			},
			{
				Key:        "weight",
				Label:      i18n.L("namespace_binding.list.weight", "Weight"),
				Type:       "badge",
				BadgeStyle: "gray",
			},
			{
				Key:   "source",
				Label: i18n.L("namespace_binding.list.source", "Source"),
				Type:  "text_badge",
				BadgeStyleByValue: map[string]string{
					"system":   "gray",
					"user":     "blue",
					"ontology": "purple",
					"manifest": "green",
				},
			},
		},
		RowActions: []formschema.RowAction{
			{ID: "edit", Icon: "pencil", Label: i18n.L("common.edit", "Edit"), VisibleWhen: "_mutable"},
			{ID: "stats", Icon: "chart-bar", Label: i18n.L("common.statistics", "Statistics")},
			{ID: "delete", Icon: "trash", Label: i18n.L("common.remove", "Remove"), Style: "danger", VisibleWhen: "_mutable"},
		},
		UI: formschema.SchemaUI{
			Languages:   languages,
			PrimaryLang: lang,
		},
	}
}

func BuildGlobalEntityListSchema(lang string, languages []formschema.LanguageInfo) *formschema.EntityListSchema {
	return &formschema.EntityListSchema{
		EntityType:        "namespace-binding",
		Title:             i18n.L("namespace_binding.list.global_entity_title", "Global Namespaces"),
		DataURL:           "/admin/namespaces/data",
		DataKey:           "items",
		DetailURLTemplate: "",
		ProjectID:         "",
		EmptyState: &formschema.EmptyState{
			Icon:    "link",
			Title:   i18n.L("namespace_binding.list.global_empty_title", "No global namespaces"),
			Message: i18n.L("namespace_binding.list.global_empty_message", "Add a shared namespace binding for all projects."),
		},
		Search: &formschema.SearchConfig{
			Placeholder: i18n.L("namespace_binding.list.search_placeholder", "Search namespaces..."),
			ParamName:   "search",
		},
		Filters: []formschema.FilterConfig{
			{
				Key:       "source",
				Label:     i18n.L("namespace_binding.list.source", "Source"),
				ParamName: "source",
				Type:      "select",
				Options: []formschema.FilterOption{
					{Value: "system", Label: i18n.L("namespace_binding.list.source_system", "System")},
					{Value: "user", Label: i18n.L("namespace_binding.list.source_user", "User")},
					{Value: "ontology", Label: i18n.L("namespace_binding.list.source_ontology", "Ontology")},
					{Value: "manifest", Label: i18n.L("namespace_binding.list.source_manifest", "Manifest")},
				},
			},
		},
		SortOptions: []formschema.SortOption{
			{Value: "prefix", Label: i18n.L("namespace_binding.list.prefix", "Prefix")},
			{Value: "namespace", Label: i18n.L("namespace_binding.list.namespace", "Namespace")},
			{Value: "weight", Label: i18n.L("namespace_binding.list.weight", "Weight")},
			{Value: "source", Label: i18n.L("namespace_binding.list.source", "Source")},
		},
		DefaultSort: "prefix",
		Pagination: &formschema.PaginationConfig{
			PageSize:        25,
			ParamName:       "page",
			PerPageName:     "per_page",
			PageSizeOptions: []int{25, 50, 100},
		},
		RowWidget: frontendrefs.EntityListRowWidget("default"),
		ViewModes: []formschema.ViewMode{
			{ID: "detailed", Label: i18n.L("namespace_binding.list.view_detailed", "Detailed"), Default: true},
			{ID: "compact", Label: i18n.L("common.view_compact", "Compact")},
		},
		RowLayout: formschema.EntityRowLayout{
			TitleField:    "prefix",
			SubtitleField: "namespace",
			IdentityFields: []formschema.IdentityField{
				{Key: "weight_display", Style: "code"},
			},
			SemanticBadges: []formschema.BadgeConfig{
				{Key: "source", Type: "text", Style: "gray"},
			},
		},
		Capabilities: &formschema.Capabilities{
			Create: &formschema.CreateCap{
				Label:         i18n.L("namespace_binding.list.global_add", "Add namespace"),
				FormSchemaURL: "/admin/namespaces/form-schema",
			},
			Edit: &formschema.EditCap{
				FormSchemaURLTemplate: "/admin/namespaces/{id}/form-schema",
			},
			Delete: &formschema.DeleteCap{
				URLTemplate: "/admin/namespaces/{id}",
			},
			Stats: &formschema.StatsCap{
				URLTemplate: "/admin/namespaces/{id}/stats",
			},
		},
		RowActions: []formschema.RowAction{
			{ID: "edit", Icon: "pencil", Label: i18n.L("common.edit", "Edit"), VisibleWhen: "_mutable"},
			{ID: "stats", Icon: "chart-bar", Label: i18n.L("common.statistics", "Statistics")},
			{ID: "delete", Icon: "trash", Label: i18n.L("common.remove", "Remove"), Style: "danger", VisibleWhen: "_mutable"},
		},
		UI: formschema.SchemaUI{
			Languages:   languages,
			PrimaryLang: lang,
		},
	}
}

// BuildCreateForm returns the FormSchema for the create dialog. POSTs to
// /projects/{projectID}/namespace-bindings.
func BuildCreateForm(projectID, lang string, languages []formschema.LanguageInfo) *formschema.FormSchema {
	return &formschema.FormSchema{
		EntityType: "namespace-binding",
		Mode:       formschema.ModeCreate,
		Endpoint: &formschema.SchemaEndpoint{
			Method: "POST",
			URL:    fmt.Sprintf("/projects/%s/namespace-bindings", projectID),
		},
		UI: formschema.SchemaUI{
			Languages:      languages,
			PrimaryLang:    lang,
			SubmitLabel:    i18n.L("namespace_binding.form.submit_create", "Add binding"),
			CancelLabel:    i18n.L("forms.cancel", "Cancel"),
			SuccessMessage: i18n.L("namespace_binding.form.created", "Namespace binding created"),
		},
		Sections: []formschema.Section{
			{ID: "identity", Fields: bindingFields(nil)},
		},
	}
}

// BuildEditForm returns the FormSchema for editing existing. PATCHes to
// /projects/{projectID}/namespace-bindings/{id}.
func BuildEditForm(projectID string, binding *domain.NamespaceBinding, lang string, languages []formschema.LanguageInfo) *formschema.FormSchema {
	id := ""
	if binding != nil {
		id = binding.ID
	}
	return &formschema.FormSchema{
		EntityType: "namespace-binding",
		Mode:       formschema.ModeEdit,
		Endpoint: &formschema.SchemaEndpoint{
			Method: "PATCH",
			URL:    fmt.Sprintf("/projects/%s/namespace-bindings/%s", projectID, id),
		},
		UI: formschema.SchemaUI{
			Languages:      languages,
			PrimaryLang:    lang,
			SubmitLabel:    i18n.L("forms.save_changes", "Save Changes"),
			CancelLabel:    i18n.L("forms.cancel", "Cancel"),
			SuccessMessage: i18n.L("namespace_binding.form.updated", "Namespace binding updated"),
		},
		Sections: []formschema.Section{
			{ID: "identity", Fields: bindingFields(binding)},
		},
	}
}

func BuildGlobalCreateForm(lang string, languages []formschema.LanguageInfo) *formschema.FormSchema {
	return &formschema.FormSchema{
		EntityType: "namespace-binding",
		Mode:       formschema.ModeCreate,
		Endpoint: &formschema.SchemaEndpoint{
			Method: "POST",
			URL:    "/admin/namespaces",
		},
		UI: formschema.SchemaUI{
			Languages:      languages,
			PrimaryLang:    lang,
			SubmitLabel:    i18n.L("namespace_binding.form.submit_create_global", "Add namespace"),
			CancelLabel:    i18n.L("forms.cancel", "Cancel"),
			SuccessMessage: i18n.L("namespace_binding.form.created_global", "Global namespace created"),
		},
		Sections: []formschema.Section{
			{ID: "identity", Fields: bindingFields(nil)},
		},
	}
}

func BuildGlobalEditForm(binding *domain.NamespaceBinding, lang string, languages []formschema.LanguageInfo) *formschema.FormSchema {
	id := ""
	if binding != nil {
		id = binding.ID
	}
	return &formschema.FormSchema{
		EntityType: "namespace-binding",
		Mode:       formschema.ModeEdit,
		Endpoint: &formschema.SchemaEndpoint{
			Method: "PATCH",
			URL:    fmt.Sprintf("/admin/namespaces/%s", id),
		},
		UI: formschema.SchemaUI{
			Languages:      languages,
			PrimaryLang:    lang,
			SubmitLabel:    i18n.L("forms.save_changes", "Save Changes"),
			CancelLabel:    i18n.L("forms.cancel", "Cancel"),
			SuccessMessage: i18n.L("namespace_binding.form.updated_global", "Global namespace updated"),
		},
		Sections: []formschema.Section{
			{ID: "identity", Fields: bindingFields(binding)},
		},
	}
}

// bindingFields renders the prefix / namespace / weight trio used by both
// create and edit forms. existing == nil → defaults; non-nil → populated
// values for edit mode.
func bindingFields(existing *domain.NamespaceBinding) []formschema.FieldDef {
	var prefix, namespace string
	var weight int64 = 10 // default for create

	if existing != nil {
		prefix = existing.Prefix
		namespace = existing.Namespace
		weight = existing.Weight
	}

	return []formschema.FieldDef{
		{
			Name:     "prefix",
			Widget:   formschema.WidgetText,
			Required: true,
			Label:    i18n.L("namespace_binding.form.prefix", "Prefix"),
			Help:     i18n.L("namespace_binding.form.prefix_help", "Short identifier used in curie-style references (e.g. crm)."),
			Value:    prefix,
		},
		{
			Name:     "namespace",
			Widget:   formschema.WidgetText,
			Required: true,
			Label:    i18n.L("namespace_binding.form.namespace", "Namespace"),
			Help:     i18n.L("namespace_binding.form.namespace_help", "Full namespace URI the prefix expands to."),
			Value:    namespace,
		},
		{
			Name:   "weight",
			Widget: formschema.WidgetNumber,
			Label:  i18n.L("namespace_binding.form.weight", "Weight"),
			Help:   i18n.L("namespace_binding.form.weight_help_create", "Lower weight resolves first. Defaults to 10."),
			Value:  weight,
		},
	}
}
