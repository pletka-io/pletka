package project

import (
	"github.com/pletka-io/pletka/pkg/formschema"
	"github.com/pletka-io/pletka/pkg/frontendrefs"
	"github.com/pletka-io/pletka/pkg/i18n"
)

// BuildEntityListSchema builds the schema for the top-level projects list.
// URLs preserve the original frontend contract.
//
// canCreate gates the toolbar's "New Project" CTA. Anonymous callers must
// not see it (alpha.pletka.io regression: anonymous users were offered a
// project-create button that 401s on submit). Pass false for anonymous,
// true for any authenticated actor.
func BuildEntityListSchema(canCreate bool, lang string, languages []formschema.LanguageInfo) *formschema.EntityListSchema {
	filters := []formschema.FilterConfig{
		{
			Key:         "institution_id",
			Label:       i18n.L("project.list.organization_filter", "Organization"),
			ParamName:   "institution_id",
			Type:        "select",
			OptionsURL:  "/projects/filters/institutions",
			OptionsType: "checklist",
		},
	}

	countColumns := []formschema.CountColumn{
		{Key: "model_count", Label: i18n.L("common.models", "Models")},
		{Key: "collection_count", Label: i18n.L("project.list.collections_short", "Coll")},
		{Key: "field_count", Label: i18n.L("common.fields", "Fields")},
		{Key: "category_count", Label: i18n.L("project.list.categories_short", "Cat"), ZeroPlaceholder: "—"},
	}

	return &formschema.EntityListSchema{
		EntityType:        "project",
		Title:             i18n.L("common.projects", "Projects"),
		DataURL:           "/projects/data",
		DataKey:           "projects",
		DetailURLTemplate: "/projects/{id}",
		EmptyState: &formschema.EmptyState{
			Icon:    "folder",
			Title:   i18n.L("project.list.empty_title", "No projects yet"),
			Message: i18n.L("project.list.empty_message", "No projects are available."),
		},
		Search: &formschema.SearchConfig{
			Placeholder: i18n.L("project.list.search_placeholder", "Search projects..."),
			ParamName:   "search",
		},
		Filters: filters,
		SortOptions: []formschema.SortOption{
			{Value: "ui_name", Label: i18n.L("forms.name", "Name")},
			{Value: "system_name", Label: i18n.L("forms.system_name", "System Name")},
			{Value: "updated_at", Label: i18n.L("project.list.last_updated", "Last Updated")},
		},
		DefaultSort: "ui_name",
		Pagination: &formschema.PaginationConfig{
			PageSize:        50,
			ParamName:       "page",
			PerPageName:     "per_page",
			PageSizeOptions: []int{25, 50, 100},
		},
		RowWidget: frontendrefs.EntityListRowWidget("editorial"),
		ViewModes: []formschema.ViewMode{
			{ID: "editorial", Label: i18n.L("common.view_editorial", "Editorial"), Default: true, Widget: frontendrefs.EntityListRowWidget("editorial")},
			{ID: "compact", Label: i18n.L("common.view_compact", "Compact"), Widget: frontendrefs.EntityListRowWidget("default")},
		},
		ColumnHeader: &formschema.ColumnHeaderConfig{
			Show:       true,
			IDLabel:    i18n.L("common.id", "ID"),
			TitleLabel: i18n.L("project.list.title_label_owner", "Project · Owner"),
		},
		RowLayout: formschema.EntityRowLayout{
			TitleField:    "ui_name",
			SubtitleField: "description",
			BylineField:   "owner",
			IdentityFields: []formschema.IdentityField{
				{Key: "id", Style: "mono"},
				{Key: "system_name", Style: "code"},
				{Key: "owner", Style: "text"},
			},
			SemanticBadges: []formschema.BadgeConfig{
				{Key: "model_count", Type: "count", Style: "blue", Label: i18n.L("common.models", "Models"), HideEmpty: true},
				{Key: "collection_count", Type: "count", Style: "purple", Label: i18n.L("common.collections", "Collections"), HideEmpty: true},
				{Key: "field_count", Type: "count", Style: "green", Label: i18n.L("common.fields", "Fields"), HideEmpty: true},
				{Key: "category_count", Type: "count", Style: "amber", Label: i18n.L("common.categories", "Categories"), HideEmpty: true},
				{Key: "owner_kind", Type: "text", Style: "gray"},
			},
			ProcessBadges: []formschema.BadgeConfig{
				{Key: "visibility", Type: "status"},
			},
			CountColumns: countColumns,
		},
		Capabilities: buildProjectListCapabilities(canCreate),
		UI: formschema.SchemaUI{
			Languages:   languages,
			PrimaryLang: lang,
		},
	}
}

// buildProjectListCapabilities assembles the Capabilities block for the
// project list schema. Edit is row-level and always emitted (the row
// itself carries can_edit per row); Create is top-level and gated on
// canCreate so anonymous callers do not see the "New Project" CTA.
func buildProjectListCapabilities(canCreate bool) *formschema.Capabilities {
	caps := &formschema.Capabilities{
		Edit: &formschema.EditCap{
			FormSchemaURLTemplate: "/projects/form-schema/project?mode=edit&entity_id={id}",
		},
	}
	if canCreate {
		caps.Create = &formschema.CreateCap{
			Label:         i18n.L("project.list.add", "New Project"),
			FormSchemaURL: "/projects/form-schema/project?mode=create",
		}
	}
	return caps
}

// BuildFormSchema builds the create/edit form schema for a project.
// existing may be nil for create mode. Canonical owner of the project
// form schema (per the pkg/weave/ module-shape ADR — slices own their
// schemas; dispatchers like entityschema delegate here).
//
// existing is *formschema.ProjectInput, the decoupled DTO defined in
// pkg/formschema/inputs.go. Callers map their store type onto ProjectInput
// before calling.
func BuildFormSchema(mode string, existing *formschema.ProjectInput, lang string, languages []formschema.LanguageInfo) *formschema.FormSchema {
	schema := &formschema.FormSchema{
		EntityType: "project",
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
			URL:    "/projects",
		}
		schema.UI.SubmitLabel = i18n.L("project.form.submit_create", "Create Project")
		schema.UI.CancelLabel = i18n.L("forms.cancel", "Cancel")
		schema.UI.SuccessMessage = i18n.L("project.form.created", "Project created — configure ontologies to start adding fields")
		schema.UI.SuccessRedirectURLTemplate = "/projects/{id}/settings"
	case formschema.ModeEdit:
		if existing != nil {
			schema.Endpoint = &formschema.SchemaEndpoint{
				Method: "PUT",
				URL:    "/projects/" + existing.ID,
			}
		}
		schema.UI.SubmitLabel = i18n.L("forms.save_changes", "Save Changes")
		schema.UI.CancelLabel = i18n.L("forms.cancel", "Cancel")
		schema.UI.SuccessMessage = i18n.L("project.form.updated", "Project updated successfully")
	case formschema.ModeView:
		schema.UI.SubmitLabel = nil
	}

	schema.Sections = []formschema.Section{
		{ID: "identity", Fields: buildIdentityFields(mode, existing)},
	}
	return schema
}

func buildIdentityFields(mode string, existing *formschema.ProjectInput) []formschema.FieldDef {
	isEdit := mode == formschema.ModeEdit || mode == formschema.ModeView
	twoMin := 2
	twoTen := 10
	twoHundred := 200
	one := 1

	uiName := formschema.FieldDef{
		Name:     "ui_name",
		Widget:   formschema.WidgetMultilingualText,
		Required: true,
		Readonly: mode == formschema.ModeView,
		Label:    i18n.L("forms.name", "Name"),
		Help:     i18n.L("project.form.name_help", "Display name for this project"),
		Validation: &formschema.ValidationRules{
			MinLength: &one,
			MaxLength: &twoHundred,
		},
	}
	if isEdit && existing != nil {
		uiName.Value = existing.UIName
	}

	idPrefix := formschema.FieldDef{
		Name:                 "id_prefix",
		Widget:               formschema.WidgetPrefixInput,
		Required:             true,
		Readonly:             isEdit,
		ImmutableAfterCreate: true,
		DerivedFrom:          "ui_name.en",
		CheckURL:             "/projects/check-prefix",
		Label:                i18n.L("project.form.id_prefix", "ID Prefix"),
		Help:                 i18n.L("project.form.id_prefix_help", "2-10 uppercase letters used in semantic IDs (e.g. \"LA\" → LAF.001). Cannot be changed after creation."),
		Validation: &formschema.ValidationRules{
			MinLength: &twoMin,
			MaxLength: &twoTen,
			Pattern:   "^[A-Z]{2,10}$",
		},
	}
	if isEdit && existing != nil {
		// ProjectInput.ID is the IDPrefix; weave_projects.id stores that prefix.
		idPrefix.Value = existing.ID
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
			Pattern: "^[a-z][a-z0-9_-]*$",
		},
	}
	if isEdit && existing != nil {
		systemName.Value = existing.SystemName
	}

	desc := formschema.FieldDef{
		Name:     "description",
		Widget:   formschema.WidgetMultilingualTextarea,
		Required: false,
		Readonly: mode == formschema.ModeView,
		Label:    i18n.L("forms.description", "Description"),
		Help:     i18n.L("project.form.description_help", "Brief description of this project"),
	}
	if isEdit && existing != nil {
		desc.Value = existing.Description
	}

	return []formschema.FieldDef{uiName, idPrefix, systemName, desc}
}
