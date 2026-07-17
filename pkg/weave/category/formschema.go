package category

import (
	"fmt"

	"github.com/pletka-io/pletka/pkg/database/dbutil"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/formschema"
	"github.com/pletka-io/pletka/pkg/i18n"
)

// Form-schema builders for the Category slice. These produce the JSON
// contract the Svelte ListManager + FormRenderer islands consume to render
// list views and create/edit forms.
//
// All URLs are absolute — the schema is the canonical source of truth for
// where mutations go. Frontend never constructs API URLs.

// ---------------------------------------------------------------------------
// List schema
// ---------------------------------------------------------------------------

// BuildListSchema returns the ListSchema for the categories list view.
//
// Capabilities exposed:
//   - Reorder: drag-and-drop (canonical_order)
//   - InlineRename: ui_name only
//   - Create: opens the create form
//   - Edit: opens the edit form
//   - Delete: cascades via reassignment OR errors with 409 (in-use blocking
//     handled by the service; the schema exposes the URL regardless and the
//     UI surfaces the "deprecate instead" message on conflict)
//   - Deprecate / Activate: lifecycle row actions
//   - Stats: opens the usage modal (override counts + samples)
//
// Columns include the new in_use boolean and deprecated badge so the
// frontend can render row state without a follow-up call.
func BuildListSchema(projectID, lang string, languages []formschema.LanguageInfo) *formschema.ListSchema {
	pid := projectID

	return &formschema.ListSchema{
		EntityType: "category",
		Title:      i18n.L("category.list.title", "Categories"),
		EmptyState: &formschema.EmptyState{
			Icon:    "tag",
			Title:   i18n.L("category.list.empty_title", "No categories"),
			Message: i18n.L("category.list.empty_message", "Get started by creating your first category."),
		},
		DataURL: fmt.Sprintf("/projects/%s/categories", pid),
		DataKey: "categories",
		Caps: formschema.Capabilities{
			Reorder: &formschema.ReorderCap{
				Enabled:    true,
				URL:        fmt.Sprintf("/projects/%s/categories/reorder", pid),
				OrderField: "category_ids",
			},
			InlineRename: &formschema.InlineRenameCap{
				Enabled:         true,
				Field:           "ui_name",
				SaveURLTemplate: fmt.Sprintf("/projects/%s/categories/{id}", pid),
			},
			Create: &formschema.CreateCap{
				Label:         i18n.L("category.list.add", "Add Category"),
				FormSchemaURL: fmt.Sprintf("/projects/%s/categories/form-schema", pid),
			},
			Edit: &formschema.EditCap{
				FormSchemaURLTemplate: fmt.Sprintf("/projects/%s/categories/{id}/form-schema", pid),
			},
			Delete: &formschema.DeleteCap{
				URLTemplate: fmt.Sprintf("/projects/%s/categories/{id}", pid),
				Reassignment: &formschema.ReassignCap{
					Enabled:     true,
					EntityLabel: i18n.L("category.list.reassign_entity_label", "fields"),
					CountField:  "field_count",
					OptionsFrom: "siblings",
				},
			},
			Stats: &formschema.StatsCap{
				URLTemplate: fmt.Sprintf("/projects/%s/categories/{id}/stats", pid),
			},
		},
		Columns: []formschema.Column{
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
				Key:        "origin_label",
				Label:      i18n.L("common.origin", "Origin"),
				Type:       "text_badge",
				BadgeStyle: "amber",
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
			{
				Key:        "deprecated",
				Label:      i18n.L("common.status", "Status"),
				Type:       "deprecated_badge",
				BadgeStyle: "amber",
				HideZero:   true,
			},
		},
		RowActions: []formschema.RowAction{
			{ID: "edit", Icon: "pencil", Label: i18n.L("common.edit", "Edit")},
			{ID: "stats", Icon: "chart-bar", Label: i18n.L("common.statistics", "Statistics")},
			{
				ID:    "deprecate",
				Icon:  "archive-box",
				Label: i18n.L("common.deprecate", "Deprecate"),
				// Visible when the row is currently active. The frontend
				// resolves visibility from the row payload (deprecated=false).
				VisibleWhen: "!deprecated",
				URLTemplate: fmt.Sprintf("/projects/%s/categories/{id}/deprecate", pid),
				Method:      "POST",
			},
			{
				ID:          "activate",
				Icon:        "arrow-uturn-up",
				Label:       i18n.L("common.activate", "Activate"),
				VisibleWhen: "deprecated",
				URLTemplate: fmt.Sprintf("/projects/%s/categories/{id}/activate", pid),
				Method:      "POST",
			},
			{
				ID:    "delete",
				Icon:  "trash",
				Label: i18n.L("common.delete", "Delete"),
				Style: "danger",
				// in_use=true rows still show this action — the service
				// returns 409 with a "deprecate instead" message rendered by
				// the frontend.
			},
		},
		UI: formschema.SchemaUI{
			Languages:   languages,
			PrimaryLang: lang,
		},
	}
}

// ---------------------------------------------------------------------------
// Form schemas
// ---------------------------------------------------------------------------

// BuildCreateForm returns the FormSchema for the create dialog/page. Submits
// to POST /projects/{projectID}/categories.
func BuildCreateForm(projectID, lang string, languages []formschema.LanguageInfo) *formschema.FormSchema {
	return buildForm(formschema.ModeCreate, nil, projectID, lang, languages)
}

// BuildEditForm returns the FormSchema for editing existing. Submits to
// PUT /projects/{projectID}/categories/{id}.
func BuildEditForm(projectID string, existing *domain.Category, lang string, languages []formschema.LanguageInfo) *formschema.FormSchema {
	return buildForm(formschema.ModeEdit, existing, projectID, lang, languages)
}

// buildForm centralises field assembly for both modes.
func buildForm(mode string, existing *domain.Category, projectID, lang string, languages []formschema.LanguageInfo) *formschema.FormSchema {
	schema := &formschema.FormSchema{
		EntityType: "category",
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
			URL:    fmt.Sprintf("/projects/%s/categories", projectID),
		}
		schema.UI.SubmitLabel = i18n.L("category.form.submit_create", "Create Category")
		schema.UI.CancelLabel = i18n.L("forms.cancel", "Cancel")
		schema.UI.SuccessMessage = i18n.L("category.form.created", "Category created successfully")

	case formschema.ModeEdit:
		if existing != nil {
			schema.Endpoint = &formschema.SchemaEndpoint{
				Method: "PUT",
				URL:    fmt.Sprintf("/projects/%s/categories/%s", projectID, existing.ID),
			}
		}
		schema.UI.SubmitLabel = i18n.L("forms.save_changes", "Save Changes")
		schema.UI.CancelLabel = i18n.L("forms.cancel", "Cancel")
		schema.UI.SuccessMessage = i18n.L("category.form.updated", "Category updated successfully")
	}

	sections := make([]formschema.Section, 0, 2)
	if mode == formschema.ModeEdit && existing != nil && existing.Origin.Kind == domain.OriginAdopted {
		sections = append(sections, formschema.Section{
			ID:    "provenance",
			Label: i18n.L("common.origin", "Origin"),
			Fields: []formschema.FieldDef{
				{
					Name:     "origin_summary",
					Widget:   formschema.WidgetText,
					Readonly: true,
					Label:    i18n.L("common.origin", "Origin"),
					Value:    categoryOriginSummary(existing.Origin),
					Help:     i18n.L("category.form.origin_help", "This category was copied in from a parent project."),
				},
				{
					Name:     "detach_warning",
					Widget:   formschema.WidgetText,
					Readonly: true,
					Label:    i18n.L("category.form.detach_label", "Detach on edit"),
					Value:    "Changing the name or description turns this copied category into a local project category. Reordering does not detach it.",
					Help:     i18n.L("category.form.detach_help", "After a structural edit, the parent adoption receipt is removed and the category becomes local."),
				},
			},
		})
	}
	sections = append(sections, formschema.Section{
		ID:     "identity",
		Label:  i18n.L("forms.identity", "Identity"),
		Fields: buildIdentityFields(mode, existing),
	})
	schema.Sections = sections

	return schema
}

// buildIdentityFields assembles the ui_name + description + system_name
// trio that every entity edit form starts with.
func buildIdentityFields(mode string, existing *domain.Category) []formschema.FieldDef {
	isEdit := mode == formschema.ModeEdit || mode == formschema.ModeView
	nameHelp := i18n.L("category.form.name_help", "Display name for this category")
	descriptionHelp := i18n.L("category.form.description_help", "Brief description of this category's purpose")
	if isEdit && existing != nil && existing.Origin.Kind == domain.OriginAdopted {
		nameHelp = i18n.L("category.form.name_help_detach", "Changing the name detaches this copied category from its parent provenance.")
		descriptionHelp = i18n.L("category.form.description_help_detach", "Changing the description detaches this copied category from its parent provenance.")
	}

	uiName := formschema.FieldDef{
		Name:     "ui_name",
		Widget:   formschema.WidgetMultilingualText,
		Required: true,
		Readonly: mode == formschema.ModeView,
		Label:    i18n.L("forms.name", "Name"),
		Help:     nameHelp,
		Validation: &formschema.ValidationRules{
			MinLength: dbutil.Ptr(1),
			MaxLength: dbutil.Ptr(200),
		},
	}
	if isEdit && existing != nil {
		uiName.Value = existing.UIName
	}

	desc := formschema.FieldDef{
		Name:     "description",
		Widget:   formschema.WidgetMultilingualTextarea,
		Required: false,
		Readonly: mode == formschema.ModeView,
		Label:    i18n.L("forms.description", "Description"),
		Help:     descriptionHelp,
	}
	if isEdit && existing != nil {
		desc.Value = existing.Description
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
			Pattern:      "^[a-z][a-z0-9_]*$",
			UniqueWithin: "project_entity_type",
		},
	}
	if isEdit && existing != nil {
		systemName.Value = existing.SystemName
	}

	return []formschema.FieldDef{uiName, desc, systemName}
}


func categoryOriginSummary(origin domain.Origin) string {
	if origin.Kind != domain.OriginAdopted {
		return ""
	}
	if origin.SourceProjectLabel != "" && origin.SourceProjectID != "" {
		return fmt.Sprintf("Adopted from %s (%s)", origin.SourceProjectLabel, origin.SourceProjectID)
	}
	if origin.SourceProjectID != "" {
		return fmt.Sprintf("Adopted from %s", origin.SourceProjectID)
	}
	return "Adopted"
}
