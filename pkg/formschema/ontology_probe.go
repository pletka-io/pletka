package formschema

import "github.com/pletka-io/pletka/pkg/i18n"

// BuildOntologyProbeSchema constructs a tiny schema-backed tool form for
// exercising the ontology scope/path widgets under the real FormRenderer path.
func BuildOntologyProbeSchema(lang string, languages []LanguageInfo) *FormSchema {
	return &FormSchema{
		EntityType: "ontology_probe",
		Mode:       ModeEdit,
		Sections: []Section{
			{
				ID:    "options",
				Label: i18n.L("ontology_probe.section_options", "Options"),
				Fields: []FieldDef{
					{
						Name:   "include_parent_projects",
						Widget: WidgetCheckbox,
						Label:  i18n.L("ontology_probe.include_parent_projects", "Include parent projects"),
						Help:   i18n.L("ontology_probe.include_parent_projects_help", "Include ontology versions inherited from parent projects."),
						Value:  true,
					},
					{
						Name:   "include_inverse",
						Widget: WidgetCheckbox,
						Label:  i18n.L("ontology_probe.include_inverse", "Include inverse properties"),
						Help:   i18n.L("ontology_probe.include_inverse_help", "Show inverse ontology properties in suggestions."),
						Value:  false,
					},
				},
			},
			{
				ID:    "ontology",
				Label: i18n.L("ontology_probe.section_path_probe", "Ontology Path Probe"),
				Fields: []FieldDef{
					{
						Name:     "ontology_scope",
						Widget:   WidgetOntologyPath,
						Required: false,
						Label:    i18n.L("common.scope_class", "Scope Class"),
						Help:     i18n.L("ontology_probe.scope_help", "Select a root CRM class to filter properties by domain."),
						Value:    nil,
					},
					{
						Name:     "ontology_path",
						Widget:   WidgetOntologyPath,
						Required: false,
						Label:    i18n.L("field.form.ontology_path", "Ontology Path"),
						Help:     i18n.L("ontology_probe.path_help", "Build a property chain from the selected scope class to a terminal value."),
						Value:    nil,
					},
				},
			},
		},
		UI: SchemaUI{
			Languages:   languages,
			PrimaryLang: lang,
		},
	}
}
