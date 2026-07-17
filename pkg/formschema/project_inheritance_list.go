package formschema

import (
	"fmt"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/i18n"
)

// BuildProjectInheritanceListSchema constructs the list schema for the
// ontology parent-management pane in project settings.
func BuildProjectInheritanceListSchema(projectID, lang string, languages []LanguageInfo, activeVersion string) *ListSchema {
	withVersion := func(raw string) string {
		if activeVersion == "" {
			return raw
		}
		return withVersionQuery(raw, activeVersion)
	}

	schema := &ListSchema{
		EntityType: "project-inheritance",
		Title:      i18n.L("project_inheritance.list.title", "Parent projects"),
		EmptyState: &EmptyState{
			Icon:    "link",
			Title:   i18n.L("project_inheritance.list.empty_title", "No parent projects"),
			Message: i18n.L("project_inheritance.list.empty_message", "Add a parent project to inherit ontology context and copy its categories."),
		},
		DataURL: withVersion(fmt.Sprintf("/projects/%s/settings/inheritance", projectID)),
		DataKey: "items",
		Columns: []Column{
			{
				Key:     "label",
				Label:   i18n.L("project_inheritance.list.parent_project", "Parent project"),
				Type:    "text",
				Primary: true,
			},
			{
				Key:       "subtitle",
				Label:     i18n.L("project_inheritance.list.details", "Details"),
				Type:      "text",
				Secondary: true,
			},
			{
				Key:   "source_label",
				Label: i18n.L("project_inheritance.list.source", "Source"),
				Type:  "text_badge",
				BadgeStyleByValue: map[string]string{
					"Draft":  "amber",
					"Pinned": "blue",
				},
				BadgeStyle: "gray",
			},
			{
				Key:   "primary_label",
				Label: i18n.L("project_ontology_version.list.primary", "Primary"),
				Type:  "text_badge",
				BadgeStyleByValue: map[string]string{
					"Primary": "blue",
					"Primair": "blue",
				},
			},
			{
				Key:        "precedence_label",
				Label:      i18n.L("project_inheritance.list.precedence", "Order"),
				Type:       "text_badge",
				BadgeStyle: "gray",
			},
			{
				Key:        "copied_categories_label",
				Label:      i18n.L("project_inheritance.list.copied_categories", "Copied categories"),
				Type:       "text_badge",
				BadgeStyle: "amber",
			},
		},
		UI: SchemaUI{
			Languages:   languages,
			PrimaryLang: lang,
		},
	}

	if activeVersion == "" {
		schema.Caps = Capabilities{
			Reorder: &ReorderCap{
				Enabled:    true,
				URL:        fmt.Sprintf("/projects/%s/settings/inheritance/reorder", projectID),
				OrderField: "parent_project_ids",
			},
			Create: &CreateCap{
				Label:         i18n.L("project_inheritance.list.add", "Add parent"),
				FormSchemaURL: fmt.Sprintf("/projects/%s/settings/form-schema/ontology-parent-add", projectID),
			},
			Edit: &EditCap{
				FormSchemaURLTemplate: fmt.Sprintf("/projects/%s/settings/form-schema/ontology-parent-source?parent_id={id}", projectID),
			},
			Delete: &DeleteCap{
				URLTemplate: fmt.Sprintf("/projects/%s/settings/inheritance/{id}", projectID),
			},
		}
		schema.RowActions = []RowAction{
			{
				ID:    "edit",
				Icon:  "pencil",
				Label: i18n.L("project_inheritance.list.change_source", "Change source"),
			},
			{
				ID:          "set-primary",
				Icon:        "arrow-uturn-up",
				Label:       i18n.L("project_inheritance.list.make_primary", "Make primary"),
				URLTemplate: fmt.Sprintf("/projects/%s/settings/inheritance/{id}/primary", projectID),
				Method:      "POST",
				VisibleWhen: "!is_primary",
			},
			{
				ID:    "delete",
				Icon:  "trash",
				Label: i18n.L("common.remove", "Remove"),
				Style: "danger",
			},
		}
	}

	return schema
}

// BuildProjectInheritanceCreateSchema returns the form used to add a new
// parent project to the inheritance graph.
func BuildProjectInheritanceCreateSchema(project *domain.Project, allProjects []*domain.Project, existing []domain.ProjectInheritance, lang string, languages []LanguageInfo) *FormSchema {
	existingParents := make(map[string]bool, len(existing))
	for _, link := range existing {
		existingParents[link.ParentProjectID] = true
	}

	// Restrict the additional-parent picker to Core Weaves
	// (IsCoreWeave=true) PLUS children of any Core Weave already
	// linked on this project. The customer's flow: add a Core
	// Weave first, then come back and add specific children of that
	// Core Weave as secondary parents. Without the children expansion
	// the dropdown stayed empty on the second pass because non-master
	// projects were filtered out unconditionally.
	projectsByID := make(map[string]*domain.Project, len(allProjects))
	for _, p := range allProjects {
		projectsByID[p.ID] = p
	}
	linkedCoreWeaveIDs := make(map[string]bool)
	for _, link := range existing {
		if p, ok := projectsByID[link.ParentProjectID]; ok && p.IsCoreWeave {
			linkedCoreWeaveIDs[p.ID] = true
		}
	}
	projectOptions := make([]SelectOption, 0)
	for _, p := range allProjects {
		if p.ID == project.ID || existingParents[p.ID] {
			continue
		}
		isCoreChild := false
		if p.ParentProjectID != nil && linkedCoreWeaveIDs[*p.ParentProjectID] {
			isCoreChild = true
		}
		if !p.IsCoreWeave && !isCoreChild {
			continue
		}
		label := fmt.Sprintf("%s (%s)", p.UIName.Get("en", p.ID), p.ID)
		if isCoreChild && !p.IsCoreWeave {
			label = fmt.Sprintf("%s (%s) — child of %s", p.UIName.Get("en", p.ID), p.ID, *p.ParentProjectID)
		}
		projectOptions = append(projectOptions, SelectOption{
			Value: p.ID,
			Label: domain.Translations{"en": label},
		})
	}

	defaultPrimary := len(existing) == 0
	return &FormSchema{
		EntityType: "project-inheritance",
		Mode:       ModeCreate,
		Endpoint: &SchemaEndpoint{
			Method: "POST",
			URL:    fmt.Sprintf("/projects/%s/settings/inheritance", project.ID),
		},
		Sections: []Section{
			{
				ID: "parent-inheritance-add",
				Fields: []FieldDef{
					{
						Name:     "parent_project_id",
						Widget:   WidgetSelect,
						Required: true,
						Label:    i18n.L("project_inheritance.list.parent_project", "Parent project"),
						Help:     i18n.L("project_inheritance.form.parent_help", "Adding a parent immediately copies missing categories from that project into this one. Those copied categories stay linked until you structurally edit them locally."),
						Options:  projectOptions,
					},
					{
						Name:   "is_primary",
						Widget: WidgetCheckbox,
						Label:  i18n.L("project_inheritance.form.make_primary", "Make primary parent"),
						Help:   i18n.L("project_inheritance.form.primary_help", "Primary parent takes precedence in inheritance resolution. Drag rows later to set fallback order for the rest."),
						Value:  defaultPrimary,
					},
					{
						Name:   "source_mode",
						Widget: WidgetRadioGroup,
						Label:  i18n.L("project_inheritance.form.source_mode", "Dependency source"),
						Help:   i18n.L("project_inheritance.form.source_mode_help", "Draft follows the parent project live. Pinned release follows one named parent release and is required before cutting a child release."),
						Value:  string(domain.DependencySourceDraft),
						Options: []SelectOption{
							{
								Value: string(domain.DependencySourceDraft),
								Label: i18n.L("project_inheritance.form.source_draft", "Draft"),
							},
							{
								Value: string(domain.DependencySourceRelease),
								Label: i18n.L("project_inheritance.form.source_release", "Pinned release"),
							},
						},
					},
					{
						Name:              "source_version",
						Widget:            WidgetSelect,
						Label:             i18n.L("project_inheritance.form.source_version", "Parent release"),
						Help:              i18n.L("project_inheritance.form.source_version_help", "Choose which named release of the parent project this child should follow."),
						OptionsURL:        fmt.Sprintf("/projects/%s/settings/inheritance/release-options?parent_project_id={parent_project_id}", project.ID),
						DependsOn:         []string{"parent_project_id"},
						HiddenUntilFilled: []string{"parent_project_id"},
						VisibleWhen:       &VisibilityRule{Field: "source_mode", Equals: string(domain.DependencySourceRelease)},
					},
				},
			},
		},
		UI: SchemaUI{
			SubmitLabel:    i18n.L("project_inheritance.form.submit_create", "Add Parent"),
			SuccessMessage: i18n.L("project_inheritance.form.created", "Parent project added"),
			Languages:      languages,
			PrimaryLang:    lang,
		},
	}
}

// BuildProjectInheritanceSourceSchema returns the edit form used to switch a
// parent edge between live draft and a pinned named parent release.
func BuildProjectInheritanceSourceSchema(projectID string, link domain.ProjectInheritance, parent *domain.Project, releaseOptions []SelectOption, lang string, languages []LanguageInfo) *FormSchema {
	label := link.ParentProjectID
	if parent != nil {
		label = parent.UIName.Get("en", parent.ID)
	}
	sourceMode := string(link.SourceMode)
	if sourceMode == "" {
		sourceMode = string(domain.DependencySourceDraft)
	}
	return &FormSchema{
		EntityType: "project-inheritance-source",
		Mode:       ModeEdit,
		Endpoint: &SchemaEndpoint{
			Method: "PUT",
			URL:    fmt.Sprintf("/projects/%s/settings/inheritance/%s/source", projectID, link.ParentProjectID),
		},
		Sections: []Section{
			{
				ID: "parent-inheritance-source",
				Fields: []FieldDef{
					{
						Name:     "parent_label",
						Widget:   WidgetText,
						Readonly: true,
						Label:    i18n.L("project_inheritance.list.parent_project", "Parent project"),
						Value:    label,
					},
					{
						Name:   "source_mode",
						Widget: WidgetRadioGroup,
						Label:  i18n.L("project_inheritance.form.source_mode", "Dependency source"),
						Help:   i18n.L("project_inheritance.form.source_mode_help", "Draft follows the parent project live. Pinned release follows one named parent release and is required before cutting a child release."),
						Value:  sourceMode,
						Options: []SelectOption{
							{
								Value: string(domain.DependencySourceDraft),
								Label: i18n.L("project_inheritance.form.source_draft", "Draft"),
							},
							{
								Value: string(domain.DependencySourceRelease),
								Label: i18n.L("project_inheritance.form.source_release", "Pinned release"),
							},
						},
					},
					{
						Name:        "source_version",
						Widget:      WidgetSelect,
						Label:       i18n.L("project_inheritance.form.source_version", "Parent release"),
						Help:        i18n.L("project_inheritance.form.source_version_help", "Choose which named release of the parent project this child should follow."),
						Value:       link.SourceVersion,
						Options:     releaseOptions,
						VisibleWhen: &VisibilityRule{Field: "source_mode", Equals: string(domain.DependencySourceRelease)},
					},
				},
			},
		},
		UI: SchemaUI{
			SubmitLabel:    i18n.L("project_inheritance.form.submit_update_source", "Save source"),
			SuccessMessage: i18n.L("project_inheritance.form.updated_source", "Parent dependency source updated"),
			Languages:      languages,
			PrimaryLang:    lang,
		},
	}
}
