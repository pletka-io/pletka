package formschema

import (
	"fmt"
	"strings"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/i18n"
)

// SettingsSchema describes the project settings page structure.
// The frontend uses this to render the sidebar navigation and load per-section forms.
type SettingsSchema struct {
	ProjectID   string              `json:"project_id"`
	ProjectName domain.Localizable  `json:"project_name"`
	Sections    []SettingsSection   `json:"sections"`
	Warnings    []SettingsWarning   `json:"warnings,omitempty"`
	Release     *ProjectReleaseView `json:"release,omitempty"`
}

// SettingsSection describes one settings pane in the sidebar.
type SettingsSection struct {
	ID             string             `json:"id"`
	Label          domain.Localizable `json:"label"`
	Icon           string             `json:"icon"`
	Kind           string             `json:"kind,omitempty"` // form, list, composite
	SchemaURL      string             `json:"schema_url,omitempty"`
	Href           string             `json:"href,omitempty"`            // direct link for non-schema panes
	Placeholder    bool               `json:"placeholder,omitempty"`     // true: pane shown in nav but content is a "coming soon" stub
	NeedsAttention bool               `json:"needs_attention,omitempty"` // true: red dot in sidebar — required setup missing
}

// SettingsWarning is a top-of-page banner for required setup that's missing.
// Frontend renders these as persistent (non-dismissible) info banners.
// Definition lives here but rules live in project_warnings.go.
type SettingsWarning struct {
	ID          string             `json:"id"`
	Severity    string             `json:"severity"` // "info", "warning", "error"
	Message     domain.Localizable `json:"message"`
	ActionHref  string             `json:"action_href,omitempty"`
	ActionLabel domain.Localizable `json:"action_label,omitempty"`
}

// BuildSettingsSchema constructs the settings page schema with capability-
// based section filtering. Each section is emitted only when snap.Can()
// grants the minimum capability needed to see it.
//
// setup carries readiness signals (e.g. whether an ontology is configured)
// that drive the top-of-page warning banner and per-section red-dot
// indicators.
func BuildSettingsSchema(project *domain.Project, snap *auth.AuthSnapshot, r auth.Resource, setup ProjectSetupState, activeVersion string) *SettingsSchema {
	projectID := project.ID
	withVersion := func(raw string) string {
		if activeVersion == "" {
			return raw
		}
		return withVersionQuery(raw, activeVersion)
	}

	allSections := []struct {
		section SettingsSection
		gate    auth.Capability
	}{
		{
			section: SettingsSection{
				ID:        "general",
				Label:     i18n.L("project_settings.section.general", "General"),
				Icon:      "cog-6-tooth",
				Kind:      "form",
				SchemaURL: withVersion(fmt.Sprintf("/projects/%s/settings/form-schema/general", projectID)),
			},
			gate: releaseAwareSettingsGate(activeVersion, auth.ProjectEdit),
		},
		{
			section: SettingsSection{
				ID:        "about",
				Label:     i18n.L("project_settings.section.about", "About"),
				Icon:      "information-circle",
				Kind:      "form",
				SchemaURL: withVersion(fmt.Sprintf("/projects/%s/settings/form-schema/about", projectID)),
			},
			gate: releaseAwareSettingsGate(activeVersion, auth.ProjectEdit),
		},
		{
			section: SettingsSection{
				ID:             "ontology",
				Label:          i18n.L("project_settings.section.ontology", "Ontology"),
				Icon:           "academic-cap",
				Kind:           "composite",
				SchemaURL:      withVersion(fmt.Sprintf("/projects/%s/settings/pane-schema/ontology", projectID)),
				NeedsAttention: !setup.HasOntology,
			},
			gate: releaseAwareSettingsGate(activeVersion, auth.ProjectEdit),
		},
		{
			section: SettingsSection{
				ID:        "categories",
				Label:     i18n.L("common.categories", "Categories"),
				Icon:      "tag",
				Kind:      "list",
				SchemaURL: withVersion(fmt.Sprintf("/projects/%s/categories/list-schema", projectID)),
			},
			gate: auth.ProjectRead,
		},
		{
			section: SettingsSection{
				ID:        "members",
				Label:     i18n.L("project_settings.section.members", "Members"),
				Icon:      "users",
				Kind:      "list",
				SchemaURL: withVersion(fmt.Sprintf("/projects/%s/members/list-schema", projectID)),
			},
			gate: auth.ProjectEdit,
		},
		{
			section: SettingsSection{
				ID:        "attributions",
				Label:     i18n.L("project_settings.section.attributions", "Credits"),
				Icon:      "trophy",
				Kind:      "list",
				SchemaURL: withVersion(fmt.Sprintf("/projects/%s/settings/attributions/list-schema", projectID)),
			},
			gate: auth.ProjectRead,
		},
		{
			section: SettingsSection{
				ID:        "namespace-bindings",
				Label:     i18n.L("project_settings.section.namespace_bindings", "Namespace Bindings"),
				Icon:      "at-symbol",
				Kind:      "list",
				SchemaURL: withVersion(fmt.Sprintf("/projects/%s/namespace-bindings/list-schema", projectID)),
			},
			gate: auth.ProjectEdit,
		},
		{
			section: SettingsSection{
				ID:        "vocabularies",
				Label:     i18n.L("project_settings.section.vocabularies", "Vocabularies"),
				Icon:      "book-open",
				Kind:      "form",
				SchemaURL: withVersion(fmt.Sprintf("/projects/%s/settings/form-schema/vocabularies", projectID)),
			},
			gate: releaseAwareSettingsGate(activeVersion, auth.ProjectEdit),
		},
		{
			section: SettingsSection{
				ID:        "autocomplete",
				Label:     i18n.L("project_settings.section.autocomplete", "Autocomplete"),
				Icon:      "magnifying-glass",
				SchemaURL: withVersion(fmt.Sprintf("/projects/%s/settings/form-schema/autocomplete", projectID)),
			},
			gate: auth.ProjectEdit,
		},
		// validation pane removed: its Href pointed at the legacy
		// /projects/{pid}/settings/validation gohtml page. Restore as a
		// slice-served pane when validation gets a schema-driven home.
		{
			section: SettingsSection{
				ID:        "integrations",
				Label:     i18n.L("project_settings.section.integrations", "Integrations"),
				Icon:      "puzzle-piece",
				Kind:      "composite",
				SchemaURL: withVersion(fmt.Sprintf("/projects/%s/integrations/list-schema", projectID)),
			},
			gate: auth.ProjectEdit,
		},
	}

	var sections []SettingsSection
	for _, s := range allSections {
		if activeVersion != "" && s.section.ID != "general" && s.section.ID != "about" && s.section.ID != "ontology" {
			continue
		}
		if snap.Can(s.gate, r, nil) {
			sections = append(sections, s.section)
		}
	}

	return &SettingsSchema{
		ProjectID:   projectID,
		ProjectName: project.UIName,
		Sections:    sections,
		Warnings:    ComputeProjectWarnings(projectID, setup, snap, r),
		Release:     buildProjectReleaseView(projectID, activeVersion, snap.Can(auth.ProjectEdit, r, nil)),
	}
}

// BuildGeneralSettingsSchema constructs the form schema for the General settings pane.
// canFlagCoreWeave gates the super-admin-only is_core_weave toggle.
// Callers pass the caller's snapshot.IsSuperAdmin to decide whether to emit the field.
func BuildGeneralSettingsSchema(project *domain.Project, canFlagCoreWeave bool, lang string, languages []LanguageInfo) *FormSchema {
	visibility := project.Visibility
	if visibility == "" {
		visibility = "private"
	}

	curation := Section{
		ID:    "curation",
		Label: i18n.L("project_settings.section.curation", "Curation"),
		Fields: []FieldDef{
			{
				Name:     "is_core_weave",
				Widget:   WidgetCheckbox,
				Required: false,
				Label:    i18n.L("project_settings.fields.is_core_weave", "Core Weave"),
				Help:     i18n.L("project_settings.fields.is_core_weave_help", "Mark this project as a top-level Core Weave so it surfaces as a starting point in child projects' parent picker. Super-admin only."),
				Value:    project.IsCoreWeave,
			},
		},
	}

	schema := &FormSchema{
		EntityType: "project_settings",
		Mode:       ModeEdit,
		Endpoint: &SchemaEndpoint{
			Method: "PUT",
			URL:    fmt.Sprintf("/projects/%s/settings/general", project.ID),
		},
		Sections: []Section{
			{
				ID:    "identity",
				Label: i18n.L("project_settings.section.identity", "Project Identity"),
				Fields: []FieldDef{
					{
						Name:     "ui_name",
						Widget:   WidgetMultilingualText,
						Required: true,
						Label:    i18n.L("project_settings.fields.project_name", "Project Name"),
						Help:     i18n.L("project_settings.fields.project_name_help", "Display name for this project"),
						Value:    project.UIName,
					},
					{
						Name:   "description",
						Widget: WidgetMultilingualTextarea,
						Label:  i18n.L("forms.description", "Description"),
						Help:   i18n.L("project.form.description_help", "Brief description of this project"),
						Value:  project.Description,
					},
					{
						Name:     "system_name",
						Widget:   WidgetSystemNamePreview,
						Readonly: true,
						Label:    i18n.L("forms.system_name", "System Name"),
						Help:     i18n.L("project_settings.fields.system_name_help", "Immutable identifier, cannot be changed"),
						Value:    project.SystemName,
					},
				},
			},
			{
				ID:    "visibility",
				Label: i18n.L("project_settings.section.visibility", "Visibility"),
				Fields: []FieldDef{
					{
						Name:     "visibility",
						Widget:   WidgetRadioGroup,
						Required: true,
						Label:    i18n.L("project_settings.fields.visibility", "Project visibility"),
						Help:     i18n.L("project_settings.fields.visibility_help", "Public projects can be discovered and read by anonymous visitors. Private projects are only visible to project members and privileged users."),
						Value:    visibility,
						Options: []SelectOption{
							{
								Value:       "private",
								Label:       i18n.L("project_settings.visibility.private", "Private"),
								Description: i18n.L("project_settings.visibility.private_help", "Only members and privileged users can view this project."),
							},
							{
								Value:       "internal",
								Label:       i18n.L("project_settings.visibility.internal", "Internal"),
								Description: i18n.L("project_settings.visibility.internal_help", "Visible to anyone in the owning institution. Hidden from anonymous visitors."),
							},
							{
								Value:       "public",
								Label:       i18n.L("project_settings.visibility.public", "Public"),
								Description: i18n.L("project_settings.visibility.public_help", "Anyone can discover and read this project."),
							},
						},
					},
				},
			},
		},
		UI: SchemaUI{
			SubmitLabel:    i18n.L("forms.save_changes", "Save Changes"),
			SuccessMessage: i18n.L("project_settings.saved", "Project settings updated"),
			Languages:      languages,
			PrimaryLang:    lang,
		},
	}

	if canFlagCoreWeave {
		schema.Sections = append(schema.Sections, curation)
	}
	return schema
}

// BuildVocabularySettingsSchema builds the project's vocabulary settings form:
// exposed source vocabularies, concept-list enforcement, and the concept
// namespace override used for RDF/SKOS export (F4, #3599).
func BuildVocabularySettingsSchema(projectID string, vocabularyOptions []SelectOption, selectedVocabularyIDs []string, enforceConceptLists bool, conceptNamespace string, lang string, languages []LanguageInfo) *FormSchema {
	return &FormSchema{
		EntityType: "project_settings",
		Mode:       ModeEdit,
		Endpoint: &SchemaEndpoint{
			Method: "PUT",
			URL:    fmt.Sprintf("/projects/%s/settings/vocabularies", projectID),
		},
		Sections: []Section{
			{
				ID:    "exposure",
				Label: i18n.L("project_settings.vocabularies.exposure", "Vocabulary Exposure"),
				Fields: []FieldDef{
					{
						Name:    "vocabulary_ids",
						Widget:  WidgetPillMultiSelect,
						Label:   i18n.L("project_settings.vocabularies.available", "Exposed vocabularies"),
						Help:    i18n.L("project_settings.vocabularies.available_help", "Choose which global vocabulary sources this project can use for concept lists and concept pickers."),
						Value:   selectedVocabularyIDs,
						Options: vocabularyOptions,
					},
					{
						Name:   "enforce_concept_lists",
						Widget: WidgetCheckbox,
						Label:  i18n.L("project_settings.vocabularies.enforce", "Enforce concept lists"),
						Help:   i18n.L("project_settings.vocabularies.enforce_help", "When enabled, concept values should come from the concept lists configured for each field. Keep off while auditing legacy fields."),
						Value:  enforceConceptLists,
					},
					{
						Name:   "concept_namespace",
						Widget: WidgetText,
						Label:  i18n.L("project_settings.vocabularies.namespace", "Concept namespace"),
						Help:   i18n.L("project_settings.vocabularies.namespace_help", "Base IRI that this project's local concept identifiers expand to in RDF/SKOS exports. Leave blank for the platform default (https://vocab.pletka.io/)."),
						Value:  conceptNamespace,
					},
				},
			},
		},
		UI: SchemaUI{
			SubmitLabel:    i18n.L("forms.save_changes", "Save Changes"),
			SuccessMessage: i18n.L("project_settings.vocabularies.saved", "Vocabulary settings updated"),
			Languages:      languages,
			PrimaryLang:    lang,
		},
	}
}

// BuildAboutSettingsSchema constructs the form schema for the About
// section: license, README, topics, base URL. Topics ships as a
// comma-separated text input for v1 — a chip widget is a follow-up.
func BuildAboutSettingsSchema(project *domain.Project, lang string, languages []LanguageInfo) *FormSchema {
	topicsValue := ""
	if len(project.Topics) > 0 {
		topicsValue = strings.Join(project.Topics, ", ")
	}

	return &FormSchema{
		EntityType: "project_settings",
		Mode:       ModeEdit,
		Endpoint: &SchemaEndpoint{
			Method: "PUT",
			URL:    fmt.Sprintf("/projects/%s/settings/about", project.ID),
		},
		Sections: []Section{
			{
				ID:    "metadata",
				Label: i18n.L("project_settings.section.metadata", "Project Metadata"),
				Fields: []FieldDef{
					{
						Name:   "license",
						Widget: WidgetText,
						Label:  i18n.L("project_settings.fields.license", "License"),
						Help:   i18n.L("project_settings.fields.license_help", "SPDX identifier (e.g. CC-BY-4.0, MIT) declaring how the project's patterns can be reused."),
						Value:  project.License,
					},
					{
						Name:   "topics",
						Widget: WidgetText,
						Label:  i18n.L("project_settings.fields.topics", "Topics"),
						Help:   i18n.L("project_settings.fields.topics_help", "Comma-separated keyword tags for filtering and discovery (e.g. 'heritage, photography, archive')."),
						Value:  topicsValue,
					},
					{
						Name:   "readme",
						Widget: WidgetMultilingualTextarea,
						Label:  i18n.L("project_settings.fields.readme", "README"),
						Help:   i18n.L("project_settings.fields.readme_help", "Long-form description of the project shown on its home page."),
						Value:  project.README,
					},
				},
			},
			{
				ID:    "uris",
				Label: i18n.L("project_settings.section.uris", "Canonical URIs"),
				Fields: []FieldDef{
					{
						Name:   "base_url",
						Widget: WidgetText,
						Label:  i18n.L("project_settings.fields.base_url", "Base URL"),
						Help:   i18n.L("project_settings.fields.base_url_help", "Override the default Pletka entity URI base for this project's RDF output. Changing this after publishing breaks RDF URIs already referenced elsewhere."),
						Value:  project.BaseURL,
					},
				},
			},
		},
		UI: SchemaUI{
			SubmitLabel:    i18n.L("project_settings.save_about", "Save About"),
			SuccessMessage: i18n.L("project_settings.about_saved", "About section updated"),
			Languages:      languages,
			PrimaryLang:    lang,
		},
	}
}

// BuildOntologySettingsSchema constructs the form schema for the parent-project
// inheritance panel. Other ontology-related controls (linked versions, namespace
// bindings) render via separate list schemas embedded in the composite pane.
//
// The parent picker lists only Core Weaves (IsCoreWeave=true). Legacy
// non-master parents already wired into a project keep their dropdown
// entry, marked "Legacy", so curators can see what the value points
// at and unset it; new selections are constrained to Core Weaves.
//
// existingChildParentIDs are the project's current secondary inheritances
// whose parent is a child of the currently-selected primary parent — they
// pre-populate the "Also inherit from child weaves" multi-select.
func BuildOntologySettingsSchema(project *domain.Project, allProjects []*domain.Project, existingChildParentIDs []string, lang string, languages []LanguageInfo) *FormSchema {
	var parentProjectID string
	if project.ParentProjectID != nil {
		parentProjectID = *project.ParentProjectID
	}

	coreOptions := []SelectOption{}
	var legacyEntry *SelectOption
	for _, p := range allProjects {
		if p.ID == project.ID {
			continue
		}
		label := fmt.Sprintf("%s (%s)", p.UIName.Get("en", p.ID), p.ID)
		opt := SelectOption{Value: p.ID, Label: domain.Translations{"en": label}}
		switch {
		case p.IsCoreWeave:
			coreOptions = append(coreOptions, opt)
		case p.ID == parentProjectID:
			// Surface the current non-Core parent as a single "Legacy"
			// entry so curators can still see and unset it. New parent
			// selections are constrained to Core Weaves.
			legacy := opt
			legacy.Label = domain.Translations{"en": "Legacy: " + label}
			legacyEntry = &legacy
		}
	}
	projectOptions := make([]SelectOption, 0, 2+len(coreOptions))
	projectOptions = append(projectOptions, SelectOption{
		Value: "",
		Label: i18n.L("project_settings.parent.none", "None (no parent)"),
	})
	projectOptions = append(projectOptions, coreOptions...)
	if legacyEntry != nil {
		projectOptions = append(projectOptions, *legacyEntry)
	}

	return &FormSchema{
		EntityType: "project_settings",
		Mode:       ModeEdit,
		Endpoint: &SchemaEndpoint{
			Method: "PUT",
			URL:    fmt.Sprintf("/projects/%s/settings/ontology", project.ID),
		},
		Sections: []Section{
			{
				ID: "parent-inheritance",
				Fields: []FieldDef{
					{
						Name:    "parent_project_id",
						Widget:  WidgetSelect,
						Label:   i18n.L("project_settings.parent.inherit_from", "Core Weave parent"),
						Help:    i18n.L("project_settings.parent.inherit_help", "Optionally inherit ontology configuration from a Core Weave. The parent's linked ontologies appear here as read-only entries, so this project can build on the same vocabulary without re-configuring it. Leave empty to start fresh."),
						Value:   parentProjectID,
						Options: projectOptions,
					},
					{
						Name:              "additional_child_parents",
						Widget:            WidgetPillMultiSelect,
						Label:             i18n.L("project_settings.parent.child_weaves", "Also inherit from child weaves"),
						Help:              i18n.L("project_settings.parent.child_weaves_help", "Optionally inherit from specific child weaves of the chosen Core Weave."),
						EntityType:        "project",
						OptionsURL:        fmt.Sprintf("/projects/%s/settings/ontology/child-weave-options?parent_project_id={parent_project_id}", project.ID),
						DependsOn:         []string{"parent_project_id"},
						HiddenUntilFilled: []string{"parent_project_id"},
						Value:             existingChildParentIDs,
					},
				},
			},
		},
		UI: SchemaUI{
			SubmitLabel:    i18n.L("forms.save_changes", "Save Changes"),
			SuccessMessage: i18n.L("project_settings.parent.updated", "Parent project updated"),
			Languages:      languages,
			PrimaryLang:    lang,
		},
	}
}

// BuildOntologyPaneSchema constructs the composite pane envelope for the
// ontology settings section. Parent inheritance is managed through a
// list manager pane so projects can add/remove/reorder multiple parents
// against weave_project_inheritance while linked ontologies render in
// their dedicated panel.
func BuildOntologyPaneSchema(projectID, activeVersion string) *CompositePaneSchema {
	withVersion := func(raw string) string {
		if activeVersion == "" {
			return raw
		}
		return withVersionQuery(raw, activeVersion)
	}
	return &CompositePaneSchema{
		Kind:  "composite-pane",
		Title: i18n.L("project_settings.section.ontology", "Ontology"),
		Panels: []CompositePanel{
			{
				ID:        "parent-inheritance",
				Label:     i18n.L("project_settings.ontology.parent_panel", "Parent projects"),
				Kind:      "list",
				SchemaURL: withVersion(fmt.Sprintf("/projects/%s/settings/list-schema/ontology-parents", projectID)),
			},
			{
				ID:    "linked-ontologies",
				Label: i18n.L("project_settings.ontology.linked_panel", "Linked ontologies"),
				// Kind dispatches the panel to a dedicated Svelte
				// component (LinkedOntologiesPanel) that renders the
				// grouped base + extensions UI. SchemaURL points at the
				// PaneView endpoint the component fetches on mount.
				Kind:      "linked-ontologies",
				SchemaURL: withVersion(fmt.Sprintf("/projects/%s/project-ontology-versions/pane", projectID)),
			},
		},
	}
}

func releaseAwareSettingsGate(activeVersion string, draftCapability auth.Capability) auth.Capability {
	if activeVersion != "" {
		return auth.ProjectRead
	}
	return draftCapability
}

func buildProjectReleaseView(projectID, activeVersion string, canEdit bool) *ProjectReleaseView {
	if activeVersion == "" {
		return nil
	}
	view := &ProjectReleaseView{
		Version: activeVersion,
		Label: i18n.LF("release.viewing_version", "Viewing release {version}",
			map[string]string{"version": activeVersion}),
	}
	if canEdit {
		view.DraftURL = fmt.Sprintf("/projects/%s/settings", projectID)
	}
	return view
}

func withVersionQuery(raw, activeVersion string) string {
	if activeVersion == "" {
		return raw
	}
	return addVersionToURL(raw, activeVersion)
}
