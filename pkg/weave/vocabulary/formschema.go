package vocabulary

import (
	"fmt"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/formschema"
	"github.com/pletka-io/pletka/pkg/frontendrefs"
	"github.com/pletka-io/pletka/pkg/i18n"
)

// BuildConceptListFormSchema returns the metadata form for project-scoped
// controlled lists. Entry curation is handled from the detail view because it
// needs search-and-add interactions against the configured vocabulary source.
func BuildConceptListFormSchema(mode string, existing *ConceptListView, vocabularies []VocabularyView, projectID, lang string, languages []formschema.LanguageInfo) *formschema.FormSchema {
	schema := &formschema.FormSchema{
		EntityType: "concept-list",
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
			URL:    fmt.Sprintf("/api/v2/projects/%s/concept-lists", projectID),
		}
		schema.UI.SubmitLabel = i18n.L("concept_list.form.submit_create", "Create Concept List")
		schema.UI.CancelLabel = i18n.L("forms.cancel", "Cancel")
		schema.UI.SuccessMessage = i18n.L("concept_list.form.created", "Concept list created")
		schema.UI.SuccessRedirectURLTemplate = fmt.Sprintf("/projects/%s/concept-lists/{id}", projectID)
	case formschema.ModeEdit:
		if existing != nil {
			schema.Endpoint = &formschema.SchemaEndpoint{
				Method: "PUT",
				URL:    fmt.Sprintf("/api/v2/projects/%s/concept-lists/%s", projectID, existing.ID),
			}
		}
		schema.UI.SubmitLabel = i18n.L("forms.save_changes", "Save Changes")
		schema.UI.CancelLabel = i18n.L("forms.cancel", "Cancel")
		schema.UI.SuccessMessage = i18n.L("concept_list.form.updated", "Concept list updated")
	}

	schema.Sections = []formschema.Section{
		{
			ID:     "identity",
			Label:  i18n.L("forms.identity", "Identity"),
			Fields: conceptListIdentityFields(mode, existing),
		},
		{
			ID:     "configuration",
			Label:  i18n.L("concept_list.form.configuration", "Configuration"),
			Fields: conceptListConfigurationFields(mode, existing, vocabularies),
		},
	}
	return schema
}

func conceptListIdentityFields(mode string, existing *ConceptListView) []formschema.FieldDef {
	isEdit := mode == formschema.ModeEdit || mode == formschema.ModeView
	one := 1
	twoHundred := 200

	uiName := formschema.FieldDef{
		Name:     "ui_name",
		Widget:   formschema.WidgetMultilingualText,
		Required: true,
		Readonly: mode == formschema.ModeView,
		Label:    i18n.L("forms.name", "Name"),
		Help:     i18n.L("concept_list.form.name_help", "Display name for this controlled list."),
		Validation: &formschema.ValidationRules{
			MinLength: &one,
			MaxLength: &twoHundred,
		},
	}
	if isEdit && existing != nil {
		uiName.Value = existing.UIName
	}

	description := formschema.FieldDef{
		Name:     "description",
		Widget:   formschema.WidgetMultilingualTextarea,
		Readonly: mode == formschema.ModeView,
		Label:    i18n.L("forms.description", "Description"),
		Help:     i18n.L("concept_list.form.description_help", "Explain how curators should use this controlled list."),
	}
	if isEdit && existing != nil {
		description.Value = existing.Description
	}

	systemName := formschema.FieldDef{
		Name:                 "system_name",
		Widget:               formschema.WidgetSystemNamePreview,
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

	return []formschema.FieldDef{uiName, description, systemName}
}

func conceptListConfigurationFields(mode string, existing *ConceptListView, vocabularies []VocabularyView) []formschema.FieldDef {
	isEdit := mode == formschema.ModeEdit || mode == formschema.ModeView
	vocabOptions := make([]formschema.SelectOption, 0, len(vocabularies))
	defaultVocabularyID := ""
	for _, vocab := range vocabularies {
		label := vocab.UIName
		if len(label) == 0 {
			label = domain.Translations{"en": vocab.SystemName}
		}
		vocabOptions = append(vocabOptions, formschema.SelectOption{
			Value:       vocab.ID,
			Label:       label,
			Description: domain.Translations{"en": vocab.BaseURI},
			Status:      vocab.Status,
		})
		if vocab.ID == "vocab_aat" {
			defaultVocabularyID = vocab.ID
		}
	}

	vocabularyID := defaultVocabularyID
	if isEdit && existing != nil && existing.VocabularyID != nil {
		vocabularyID = *existing.VocabularyID
	}
	listTypeURI := ""
	if isEdit && existing != nil {
		listTypeURI = existing.ListTypeURI
	}

	status := "draft"
	if isEdit && existing != nil && existing.Status != "" {
		status = existing.Status
	}

	return []formschema.FieldDef{
		{
			Name:     "vocabulary_id",
			Widget:   formschema.WidgetSearchSelect,
			Readonly: mode == formschema.ModeView,
			Label:    i18n.L("concept_list.form.source_vocabulary", "Source Vocabulary"),
			Help:     i18n.L("concept_list.form.source_vocabulary_help", "Vocabulary used when curators search for entries to pin into this list."),
			Value:    vocabularyID,
			Options:  vocabOptions,
		},
		{
			Name:              "parent_term_uri",
			Widget:            formschema.WidgetVocabularyEntryPicker,
			Readonly:          mode == formschema.ModeView,
			Label:             i18n.L("concept_list.form.parent_term", "Parent Term"),
			Help:              i18n.L("concept_list.form.parent_term_help", "Optional source vocabulary parent/root term used to constrain search results. Search the selected vocabulary and choose the parent term."),
			Value:             listTypeURI,
			SearchURL:         "/api/v2/vocabularies/{vocabulary_id}/entries/search",
			DependsOn:         []string{"vocabulary_id"},
			HiddenUntilFilled: []string{"vocabulary_id"},
		},
		{
			Name:     "status",
			Widget:   formschema.WidgetSelect,
			Required: true,
			Readonly: mode == formschema.ModeView,
			Label:    i18n.L("common.status", "Status"),
			Value:    status,
			Options: []formschema.SelectOption{
				{Value: "draft", Label: i18n.L("status.draft", "Draft")},
				{Value: "published", Label: i18n.L("status.published", "Published")},
				{Value: "deprecated", Label: i18n.L("status.deprecated", "Deprecated")},
			},
		},
	}
}

func BuildAdminVocabularyEntityListSchema(lang string, languages []formschema.LanguageInfo) *formschema.EntityListSchema {
	return &formschema.EntityListSchema{
		EntityType: "vocabulary",
		Title:      i18n.L("admin.vocabularies.title", "Vocabularies"),
		DataURL:    "/admin/vocabularies/data",
		DataKey:    "items",
		EmptyState: &formschema.EmptyState{
			Icon:    "book-open",
			Title:   i18n.L("admin.vocabularies.empty_title", "No vocabularies"),
			Message: i18n.L("admin.vocabularies.empty_message", "Configure global vocabulary sources such as AAT before projects expose them."),
		},
		Search: &formschema.SearchConfig{
			Placeholder: i18n.L("admin.vocabularies.search_placeholder", "Search vocabularies..."),
			ParamName:   "search",
		},
		Filters: []formschema.FilterConfig{
			{
				Key:       "connector_type",
				Label:     i18n.L("admin.vocabularies.connector", "Connector"),
				ParamName: "connector_type",
				Type:      "select",
				Options: []formschema.FilterOption{
					{Value: "aat", Label: i18n.L("admin.vocabularies.connector_aat", "AAT")},
					{Value: "sparql", Label: i18n.L("admin.vocabularies.connector_sparql", "SPARQL")},
					{Value: "local", Label: i18n.L("admin.vocabularies.connector_local", "Local")},
					{Value: "csv", Label: i18n.L("admin.vocabularies.connector_csv", "CSV")},
				},
			},
			{
				Key:       "status",
				Label:     i18n.L("common.status", "Status"),
				ParamName: "status",
				Type:      "select",
				Options: []formschema.FilterOption{
					{Value: "draft", Label: i18n.L("status.draft", "Draft")},
					{Value: "published", Label: i18n.L("status.published", "Published")},
					{Value: "deprecated", Label: i18n.L("status.deprecated", "Deprecated")},
				},
			},
		},
		SortOptions: []formschema.SortOption{
			{Value: "ui_name", Label: i18n.L("forms.name", "Name")},
			{Value: "system_name", Label: i18n.L("forms.system_name", "System Name")},
			{Value: "connector_type", Label: i18n.L("admin.vocabularies.connector", "Connector")},
			{Value: "entries", Label: i18n.L("concept_list.list.entries", "Entries")},
			{Value: "projects", Label: i18n.L("admin.vocabularies.projects", "Projects")},
		},
		DefaultSort: "ui_name",
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
			TitleField:    "ui_name",
			SubtitleField: "base_uri",
			IdentityFields: []formschema.IdentityField{
				{Key: "semantic_id", Style: "mono"},
				{Key: "system_name", Style: "code"},
			},
			SemanticBadges: []formschema.BadgeConfig{
				{Key: "connector_type", Type: "text", Style: "purple", Label: i18n.L("admin.vocabularies.connector", "Connector")},
				{Key: "entry_count", Type: "count", Style: "green", Label: i18n.L("concept_list.list.entries", "Entries")},
				{Key: "project_count", Type: "count", Style: "blue", Label: i18n.L("admin.vocabularies.projects", "Projects")},
			},
			ProcessBadges: []formschema.BadgeConfig{
				{Key: "status", Type: "status"},
			},
		},
		UI: formschema.SchemaUI{
			Languages:   languages,
			PrimaryLang: lang,
		},
	}
}
