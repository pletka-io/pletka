package projectontologyversion

import (
	"fmt"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/formschema"
	"github.com/pletka-io/pletka/pkg/i18n"
)

// BuildListSchema constructs the list schema served at GET
// /projects/{projectID}/project-ontology-versions/list-schema.
func BuildListSchema(projectID, lang string, languages []formschema.LanguageInfo) *formschema.ListSchema {
	pid := projectID
	return &formschema.ListSchema{
		EntityType: "project-ontology-version",
		EmptyState: &formschema.EmptyState{
			Icon:    "academic-cap",
			Title:   i18n.L("project_ontology_version.list.empty_title", "No ontologies linked"),
			Message: i18n.L("project_ontology_version.list.empty_message", "Link an ontology version to get started."),
		},
		DataURL: fmt.Sprintf("/projects/%s/project-ontology-versions", pid),
		Caps: formschema.Capabilities{
			Create: &formschema.CreateCap{
				Label:         i18n.L("project_ontology_version.list.add", "Add ontology"),
				FormSchemaURL: fmt.Sprintf("/projects/%s/project-ontology-versions/form-schema?mode=create", pid),
			},
			Edit: &formschema.EditCap{
				FormSchemaURLTemplate: fmt.Sprintf("/projects/%s/project-ontology-versions/form-schema?mode=edit&entity_id={id}", pid),
			},
			Delete: &formschema.DeleteCap{
				URLTemplate: fmt.Sprintf("/projects/%s/project-ontology-versions/{id}", pid),
			},
			Stats: &formschema.StatsCap{
				URLTemplate: fmt.Sprintf("/projects/%s/project-ontology-versions/{id}/stats", pid),
			},
		},
		Columns: []formschema.Column{
			{Key: "label", Label: i18n.L("project_ontology_version.list.ontology", "Ontology"), Type: "text", Primary: true},
			{Key: "version", Label: i18n.L("project_ontology_version.list.version", "Version"), Type: "text"},
			{Key: "primary", Label: i18n.L("project_ontology_version.list.primary", "Primary"), Type: "badge", BadgeStyle: "blue", HideZero: true},
			{Key: "usage_count", Label: i18n.L("project_ontology_version.list.uses", "Uses"), Type: "badge", BadgeStyle: "purple"},
		},
		RowActions: []formschema.RowAction{
			{ID: "edit", Icon: "pencil", Label: i18n.L("common.edit", "Edit")},
			{ID: "stats", Icon: "chart-bar", Label: i18n.L("common.statistics", "Statistics")},
			{ID: "delete", Icon: "trash", Label: i18n.L("project_ontology_version.list.remove", "Remove"), Style: "danger"},
		},
		UI: formschema.SchemaUI{
			Languages:   languages,
			PrimaryLang: lang,
		},
	}
}

// BuildCreateForm constructs the "Add ontology version" form for the
// linked-ontology-versions pane. Dependent-field chain:
//
//	ontology_id -> version_id -> extensions
//
// plus is_primary and usage_notes.
//
// Only base ontologies are offered as the first selection; version_id and
// extensions resolve through OptionsURL endpoints with {token} substitution.
func BuildCreateForm(projectID string, baseOntologies []*domain.Ontology, lang string, languages []formschema.LanguageInfo) *formschema.FormSchema {
	pid := projectID
	ontologyOptions := make([]formschema.SelectOption, 0, len(baseOntologies))
	for _, o := range baseOntologies {
		if o == nil {
			continue
		}
		label := o.Name
		if label == "" {
			label = o.ID
		}
		ontologyOptions = append(ontologyOptions, formschema.SelectOption{
			Value: o.ID,
			Label: domain.Translations{"en": label},
		})
	}

	return &formschema.FormSchema{
		EntityType: "project-ontology-version",
		Mode:       formschema.ModeCreate,
		Endpoint: &formschema.SchemaEndpoint{
			Method: "POST",
			URL:    fmt.Sprintf("/projects/%s/project-ontology-versions", pid),
		},
		UI: formschema.SchemaUI{
			Languages:      languages,
			PrimaryLang:    lang,
			SubmitLabel:    i18n.L("ontology.form.submit_create", "Add ontology"),
			CancelLabel:    i18n.L("forms.cancel", "Cancel"),
			SuccessMessage: i18n.L("ontology.form.created", "Ontology version linked successfully"),
		},
		Sections: []formschema.Section{
			{
				ID: "identity",
				Fields: []formschema.FieldDef{
					{
						Name:     "ontology_id",
						Widget:   formschema.WidgetSelect,
						Required: true,
						Label:    i18n.L("ontology.form.ontology", "Ontology"),
						Help:     i18n.L("ontology.form.ontology_help", "Select the base ontology to link"),
						Options:  ontologyOptions,
					},
					{
						Name:              "version_id",
						Widget:            formschema.WidgetSelect,
						Required:          true,
						Label:             i18n.L("ontology.form.version", "Version"),
						Help:              i18n.L("ontology.form.version_help", "Choose which version of the ontology to use"),
						DependsOn:         []string{"ontology_id"},
						OptionsURL:        fmt.Sprintf("/projects/%s/project-ontology-versions/options/versions?base={ontology_id}", pid),
						HiddenUntilFilled: []string{"ontology_id"},
					},
					{
						Name:              "extensions",
						Widget:            formschema.WidgetOntologyTree,
						Label:             i18n.L("ontology.form.extensions", "Extensions"),
						Help:              i18n.L("ontology.form.extensions_help", "Optional extensions compatible with the chosen base version"),
						DependsOn:         []string{"version_id"},
						OptionsURL:        fmt.Sprintf("/projects/%s/project-ontology-versions/options/extensions?base_version={version_id}", pid),
						HiddenUntilFilled: []string{"version_id"},
					},
					{
						Name:   "is_primary",
						Widget: formschema.WidgetCheckbox,
						Label:  i18n.L("ontology.form.primary", "Primary"),
						Help:   i18n.L("ontology.form.primary_help", "Mark this ontology as the primary one for this project"),
						Value:  false,
					},
					{
						Name:   "usage_notes",
						Widget: formschema.WidgetTextarea,
						Label:  i18n.L("ontology.form.usage_notes", "Usage notes"),
						Help:   i18n.L("ontology.form.usage_notes_help", "Optional notes about how this ontology is used in the project"),
					},
				},
			},
		},
	}
}

// BuildEditForm constructs the edit form for an existing
// project-ontology-version link. Only is_primary and usage_notes are
// editable — the ontology version itself is fixed once linked. Submits
// via PATCH.
func BuildEditForm(projectID string, link *domain.ProjectOntologyVersion, lang string, languages []formschema.LanguageInfo) *formschema.FormSchema {
	var (
		isPrimary  bool
		usageNotes string
		versionID  string
	)
	if link != nil {
		isPrimary = link.IsPrimary
		usageNotes = link.UsageNotes
		versionID = link.OntologyVersionID
	}

	return &formschema.FormSchema{
		EntityType: "project-ontology-version",
		Mode:       formschema.ModeEdit,
		Endpoint: &formschema.SchemaEndpoint{
			Method: "PATCH",
			URL:    fmt.Sprintf("/projects/%s/project-ontology-versions/%s", projectID, versionID),
		},
		UI: formschema.SchemaUI{
			Languages:      languages,
			PrimaryLang:    lang,
			SubmitLabel:    i18n.L("forms.save_changes", "Save Changes"),
			CancelLabel:    i18n.L("forms.cancel", "Cancel"),
			SuccessMessage: i18n.L("ontology.form.updated", "Ontology link updated successfully"),
		},
		Sections: []formschema.Section{
			{
				ID: "identity",
				Fields: []formschema.FieldDef{
					{
						Name:   "is_primary",
						Widget: formschema.WidgetCheckbox,
						Label:  i18n.L("ontology.form.primary", "Primary"),
						Help:   i18n.L("ontology.form.primary_help", "Mark this ontology as the primary one for this project"),
						Value:  isPrimary,
					},
					{
						Name:   "usage_notes",
						Widget: formschema.WidgetTextarea,
						Label:  i18n.L("ontology.form.usage_notes", "Usage notes"),
						Help:   i18n.L("ontology.form.usage_notes_help", "Optional notes about how this ontology is used in the project"),
						Value:  usageNotes,
					},
				},
			},
		},
	}
}
