package attribution

import (
	"fmt"

	"github.com/pletka-io/pletka/pkg/formschema"
	"github.com/pletka-io/pletka/pkg/i18n"
)

// BuildSettingsListSchema describes the curator-facing settings panel
// for project credits. Renders one row per attribution
// grouped by kind, with delete row actions.
//
// The data URL emits the bare rows; the frontend groups by `kind` and
// orders by `position` for display. Reorder hits a separate endpoint
// that takes (kind, actor_ids_in_order).
func BuildSettingsListSchema(projectID, lang string, languages []formschema.LanguageInfo) *formschema.ListSchema {
	pid := projectID
	return &formschema.ListSchema{
		EntityType: "project-attribution",
		EmptyState: &formschema.EmptyState{
			Icon:    "users",
			Title:   i18n.L("project_attribution.list.empty_title", "No credits yet"),
			Message: i18n.L("project_attribution.list.empty_message", "Add authors, funders, or adopters to credit them on the project overview."),
		},
		DataURL: fmt.Sprintf("/projects/%s/settings/attributions", pid),
		DataKey: "attributions",
		Caps: formschema.Capabilities{
			Create: &formschema.CreateCap{
				Label:         i18n.L("project_attribution.list.add", "Add credit"),
				FormSchemaURL: fmt.Sprintf("/projects/%s/settings/attributions/form-schema?mode=create", pid),
			},
			Delete: &formschema.DeleteCap{
				URLTemplate: fmt.Sprintf("/projects/%s/settings/attributions/{actor_id}/{kind}/{position}", pid),
			},
		},
		Columns: []formschema.Column{
			{Key: "actor_name", Label: i18n.L("project_attribution.list.actor", "Actor"), Type: "text", Primary: true},
			{
				Key:        "kind",
				Label:      i18n.L("project_attribution.list.kind", "Kind"),
				Type:       "text_badge",
				BadgeStyle: "amber",
				BadgeStyleByValue: map[string]string{
					"author":  "blue",
					"funder":  "green",
					"adopter": "purple",
				},
			},
			{Key: "note", Label: i18n.L("project_attribution.list.note", "Note"), Type: "text"},
		},
		RowActions: []formschema.RowAction{
			{ID: "delete", Icon: "trash", Label: i18n.L("common.remove", "Remove"), Style: "danger"},
		},
		UI: formschema.SchemaUI{
			Languages:   languages,
			PrimaryLang: lang,
		},
	}
}

// BuildSettingsCreateSchema is the create form launched from the "+
// Add credit" action on the settings list. Kind is a fixed select;
// actor_id is a search-select against /actors/options; note is
// freeform.
func BuildSettingsCreateSchema(projectID, lang string, languages []formschema.LanguageInfo) *formschema.FormSchema {
	return &formschema.FormSchema{
		EntityType: "project-attribution",
		Mode:       formschema.ModeCreate,
		Endpoint: &formschema.SchemaEndpoint{
			Method: "POST",
			URL:    fmt.Sprintf("/projects/%s/settings/attributions", projectID),
		},
		Sections: []formschema.Section{
			{
				ID: "details",
				Fields: []formschema.FieldDef{
					{
						Name:     "kind",
						Widget:   formschema.WidgetSelect,
						Required: true,
						Label:    i18n.L("project_attribution.form.kind", "Kind"),
						Help:     i18n.L("project_attribution.form.kind_help", "Authors and funders are credited on the project overview. Kinds are independent of access roles and institutional ownership."),
						Options: []formschema.SelectOption{
							{Value: "author", Label: i18n.L("project_attribution.kind.author", "Author")},
							{Value: "funder", Label: i18n.L("project_attribution.kind.funder", "Funder")},
							{Value: "adopter", Label: i18n.L("project_attribution.kind.adopter", "Adopter")},
						},
					},
					{
						Name:       "actor_id",
						Widget:     formschema.WidgetSearchSelect,
						Required:   true,
						Label:      i18n.L("project_attribution.form.actor", "Actor"),
						Help:       i18n.L("project_attribution.form.actor_help", "Pick the person or institution to credit."),
						EntityType: "actor",
						OptionsURL: fmt.Sprintf("/projects/%s/settings/attributions/options/actors", projectID),
					},
					{
						Name:   "note",
						Widget: formschema.WidgetText,
						Label:  i18n.L("project_attribution.form.note", "Note"),
						Help:   i18n.L("project_attribution.form.note_help", "Optional. Describe what they contributed, e.g. \"lead designer\" or \"co-funded\"."),
					},
				},
			},
		},
		UI: formschema.SchemaUI{
			Languages:      languages,
			PrimaryLang:    lang,
			SubmitLabel:    i18n.L("project_attribution.form.submit_create", "Add credit"),
			CancelLabel:    i18n.L("forms.cancel", "Cancel"),
			SuccessMessage: i18n.L("project_attribution.form.created", "Credit added"),
		},
	}
}

// BuildSettingsEditSchema reuses the create-form layout but switches
// mode and submit URL for note edits. Kind + actor are read-only.
func BuildSettingsEditSchema(projectID, actorID, kind string, position int, note string, actorName string, lang string, languages []formschema.LanguageInfo) *formschema.FormSchema {
	return &formschema.FormSchema{
		EntityType: "project-attribution",
		Mode:       formschema.ModeEdit,
		Endpoint: &formschema.SchemaEndpoint{
			Method: "PATCH",
			URL:    fmt.Sprintf("/projects/%s/settings/attributions/%s/%s/%d", projectID, actorID, kind, position),
		},
		Sections: []formschema.Section{
			{
				ID: "details",
				Fields: []formschema.FieldDef{
					{
						Name:     "actor_label",
						Widget:   formschema.WidgetText,
						Label:    i18n.L("project_attribution.form.actor", "Actor"),
						Readonly: true,
						Value:    actorName,
					},
					{
						Name:     "kind_label",
						Widget:   formschema.WidgetText,
						Label:    i18n.L("project_attribution.form.kind", "Kind"),
						Readonly: true,
						Value:    kind,
					},
					{
						Name:   "note",
						Widget: formschema.WidgetText,
						Label:  i18n.L("project_attribution.form.note", "Note"),
						Help:   i18n.L("project_attribution.form.note_help", "Optional. Describe what they contributed, e.g. \"lead designer\" or \"co-funded\"."),
						Value:  note,
					},
				},
			},
		},
		UI: formschema.SchemaUI{
			Languages:      languages,
			PrimaryLang:    lang,
			SubmitLabel:    i18n.L("forms.save_changes", "Save Changes"),
			CancelLabel:    i18n.L("forms.cancel", "Cancel"),
			SuccessMessage: i18n.L("project_attribution.form.updated", "Credit updated"),
		},
	}
}
