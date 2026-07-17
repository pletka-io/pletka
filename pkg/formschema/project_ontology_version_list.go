package formschema

import (
	"fmt"

	"github.com/pletka-io/pletka/pkg/i18n"
)

// BuildProjectOntologyVersionListSchema constructs the list schema for the
// linked-ontology-versions pane. It supports add/edit/delete/stats actions
// but not reorder (versions carry no canonical order) or inline rename (the
// label is owned by the ontology_versions master table, not editable here).
func BuildProjectOntologyVersionListSchema(projectID string, lang string, languages []LanguageInfo) *ListSchema {
	pid := projectID
	return &ListSchema{
		EntityType: "project-ontology-version",
		EmptyState: &EmptyState{
			Icon:    "academic-cap",
			Title:   i18n.L("project_ontology_version.list.empty_title", "No ontologies linked"),
			Message: i18n.L("project_ontology_version.list.empty_message", "Link an ontology version to get started."),
		},
		DataURL: fmt.Sprintf("/projects/%s/project-ontology-versions", pid),
		Caps: Capabilities{
			Create: &CreateCap{
				Label:         i18n.L("project_ontology_version.list.add", "Add ontology"),
				FormSchemaURL: fmt.Sprintf("/projects/%s/form-schema/project-ontology-version?mode=create", pid),
			},
			Edit: &EditCap{
				FormSchemaURLTemplate: fmt.Sprintf("/projects/%s/form-schema/project-ontology-version?mode=edit&entity_id={id}", pid),
			},
			Delete: &DeleteCap{
				URLTemplate: fmt.Sprintf("/projects/%s/project-ontology-versions/{id}", pid),
			},
			Stats: &StatsCap{
				URLTemplate: fmt.Sprintf("/projects/%s/project-ontology-versions/{id}/stats", pid),
			},
		},
		Columns: []Column{
			{
				Key:     "label",
				Label:   i18n.L("project_ontology_version.list.ontology", "Ontology"),
				Type:    "text",
				Primary: true,
			},
			{
				Key:   "version",
				Label: i18n.L("project_ontology_version.list.version", "Version"),
				Type:  "text",
			},
			{
				Key:        "primary",
				Label:      i18n.L("project_ontology_version.list.primary", "Primary"),
				Type:       "badge",
				BadgeStyle: "blue",
				HideZero:   true,
			},
			{
				Key:        "usage_count",
				Label:      i18n.L("project_ontology_version.list.uses", "Uses"),
				Type:       "badge",
				BadgeStyle: "purple",
			},
		},
		RowActions: []RowAction{
			{ID: "edit", Icon: "pencil", Label: i18n.L("common.edit", "Edit")},
			{ID: "stats", Icon: "chart-bar", Label: i18n.L("common.statistics", "Statistics")},
			{ID: "delete", Icon: "trash", Label: i18n.L("project_ontology_version.list.remove", "Remove"), Style: "danger"},
		},
		UI: SchemaUI{
			Languages:   languages,
			PrimaryLang: lang,
		},
	}
}
