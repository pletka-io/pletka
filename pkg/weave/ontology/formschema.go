package ontology

import (
	"strings"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/formschema"
	"github.com/pletka-io/pletka/pkg/frontendrefs"
	"github.com/pletka-io/pletka/pkg/i18n"
)

// Schema URLs (relative to /admin/ontologies/) used by the entity-list
// and entity-form Svelte islands.
const (
	listSchemaURL             = "/admin/ontologies/list-schema"
	entityListSchemaURL       = "/admin/ontologies/entity-list-schema"
	familyListSchemaURL       = "/admin/ontologies/families/list-schema"
	familyEntityListSchemaURL = "/admin/ontologies/families/entity-list-schema"
	versionListSchemaURLTpl   = "/admin/ontologies/{ontologyID}/versions/list-schema"
	ontologyFormSchemaCreate  = "/admin/ontologies/form-schema?mode=create"
	ontologyFormSchemaEditTpl = "/admin/ontologies/form-schema?mode=edit&entity_id={id}"
	familyFormSchemaCreate    = "/admin/ontologies/families/form-schema?mode=create"
	familyFormSchemaEditTpl   = "/admin/ontologies/families/form-schema?mode=edit&entity_id={id}"
	versionFormSchemaEditTpl  = "/admin/ontologies/versions/form-schema?mode=edit&entity_id={id}"
)

// ----------------------------------------------------------------------------
// Family list / form schemas
// ----------------------------------------------------------------------------

// BuildFamilyListSchema is the list view of ontology families. Drives
// the entity-list island for the admin tab.
func BuildFamilyListSchema(lang string, languages []formschema.LanguageInfo) *formschema.ListSchema {
	return &formschema.ListSchema{
		EntityType: "ontology-family",
		EmptyState: &formschema.EmptyState{
			Icon:    "folder",
			Title:   i18n.L("ontology_admin.family.empty_title", "No ontology families"),
			Message: i18n.L("ontology_admin.family.empty_message", "Create one to group related ontologies."),
		},
		DataURL: "/admin/ontologies/families",
		Caps: formschema.Capabilities{
			Create: &formschema.CreateCap{
				Label:         i18n.L("ontology_admin.family.add", "Add family"),
				FormSchemaURL: familyFormSchemaCreate,
			},
			Edit: &formschema.EditCap{
				FormSchemaURLTemplate: familyFormSchemaEditTpl,
			},
			Delete: &formschema.DeleteCap{
				URLTemplate: "/admin/ontologies/families/{id}",
			},
		},
		Columns: []formschema.Column{
			{Key: "name", Label: i18n.L("forms.name", "Name"), Type: "text", Primary: true},
			{Key: "slug", Label: i18n.L("forms.fields.slug", "Slug"), Type: "text"},
			{Key: "icon", Label: i18n.L("ontology_admin.family.icon", "Icon"), Type: "text"},
			{Key: "display_order", Label: i18n.L("ontology_admin.family.order", "Order"), Type: "number"},
		},
	}
}

func BuildFamilyEntityListSchema(lang string, languages []formschema.LanguageInfo) *formschema.EntityListSchema {
	return &formschema.EntityListSchema{
		EntityType:        "ontology-family",
		Title:             i18n.L("ontology_admin.family.title", "Ontology Families"),
		DataURL:           "/admin/ontologies/families/data",
		DataKey:           "items",
		DetailURLTemplate: "/admin/ontologies/families/{id}/page",
		ProjectID:         "",
		EmptyState: &formschema.EmptyState{
			Icon:    "folder",
			Title:   i18n.L("ontology_admin.family.empty_title", "No ontology families"),
			Message: i18n.L("ontology_admin.family.empty_message", "Create one to group related ontologies."),
		},
		Search: &formschema.SearchConfig{
			Placeholder: i18n.L("ontology_admin.family.search_placeholder", "Search families..."),
			ParamName:   "search",
		},
		SortOptions: []formschema.SortOption{
			{Value: "display_order", Label: i18n.L("ontology_admin.family.display_order", "Display order")},
			{Value: "name", Label: i18n.L("forms.name", "Name")},
			{Value: "slug", Label: i18n.L("forms.fields.slug", "Slug")},
			{Value: "ontology_count", Label: i18n.L("ontology_admin.family.ontology_count", "Ontology count")},
		},
		DefaultSort: "display_order",
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
			TitleField:    "name",
			SubtitleField: "parent_family_name",
			IdentityFields: []formschema.IdentityField{
				{Key: "slug", Style: "code"},
			},
			SemanticBadges: []formschema.BadgeConfig{
				{Key: "ontology_count", Type: "count", Style: "blue", Label: i18n.L("ontology_admin.ontologies_label", "Ontologies")},
			},
		},
		Capabilities: &formschema.Capabilities{
			Create: &formschema.CreateCap{
				Label:         i18n.L("ontology_admin.family.add", "Add family"),
				FormSchemaURL: familyFormSchemaCreate,
			},
			Edit: &formschema.EditCap{
				FormSchemaURLTemplate: familyFormSchemaEditTpl,
			},
			Delete: &formschema.DeleteCap{
				URLTemplate: "/admin/ontologies/families/{id}",
			},
		},
		RowActions: []formschema.RowAction{
			{ID: "edit", Icon: "pencil", Label: i18n.L("common.edit", "Edit")},
			{ID: "delete", Icon: "trash", Label: i18n.L("common.remove", "Remove"), Style: "danger"},
		},
		UI: formschema.SchemaUI{
			Languages:   languages,
			PrimaryLang: lang,
		},
	}
}

// BuildFamilyFormSchema is the create / edit form for a family.
//
// `families` is the full set of existing families used to populate the
// `parent_family_id` select. When editing, the family being edited is
// excluded from the options to prevent self-parenting.
func BuildFamilyFormSchema(mode string, existing *domain.OntologyFamily, families []*domain.OntologyFamily, lang string, languages []formschema.LanguageInfo) *formschema.FormSchema {
	schema := &formschema.FormSchema{
		EntityType: "ontology-family",
		Mode:       mode,
		UI: formschema.SchemaUI{
			Languages:   languages,
			PrimaryLang: lang,
		},
	}
	switch mode {
	case formschema.ModeCreate:
		schema.Endpoint = &formschema.SchemaEndpoint{Method: "POST", URL: "/admin/ontologies/families"}
		schema.UI.SubmitLabel = i18n.L("ontology_admin.family.submit_create", "Create family")
	case formschema.ModeEdit:
		if existing != nil {
			schema.Endpoint = &formschema.SchemaEndpoint{Method: "PUT", URL: "/admin/ontologies/families/" + existing.ID}
		}
		schema.UI.SubmitLabel = i18n.L("forms.save_changes", "Save Changes")
	}
	schema.UI.CancelLabel = i18n.L("forms.cancel", "Cancel")

	one := 1
	threeHundred := 300

	slugField := formschema.FieldDef{
		Name:       "slug",
		Widget:     formschema.WidgetText,
		Required:   true,
		Readonly:   mode == formschema.ModeEdit, // slug is the natural key; immutable after creation
		Label:      i18n.L("forms.fields.slug", "Slug"),
		Help:       i18n.L("ontology_admin.family.slug_help", "URL-friendly identifier (lowercase, dashes). Used to derive the deterministic ID."),
		Validation: &formschema.ValidationRules{MinLength: &one, MaxLength: &threeHundred, Pattern: "^[a-z0-9-]+$"},
	}
	nameField := formschema.FieldDef{
		Name:       "name",
		Widget:     formschema.WidgetText,
		Required:   true,
		Label:      i18n.L("forms.name", "Name"),
		Validation: &formschema.ValidationRules{MinLength: &one, MaxLength: &threeHundred},
	}
	descField := formschema.FieldDef{
		Name:   "description",
		Widget: formschema.WidgetMultilingualText,
		Label:  i18n.L("forms.description", "Description"),
	}
	parentField := formschema.FieldDef{
		Name:    "parent_family_id",
		Widget:  formschema.WidgetSelect,
		Label:   i18n.L("ontology_admin.family.parent_family", "Parent family"),
		Help:    i18n.L("ontology_admin.family.parent_family_help", "Optional. Make this family a child of another family."),
		Options: parentFamilyOptions(existing, families),
	}
	homepageField := formschema.FieldDef{Name: "homepage_url", Widget: formschema.WidgetText, Label: i18n.L("ontology_admin.family.homepage_url", "Homepage URL")}
	iconField := formschema.FieldDef{Name: "icon", Widget: formschema.WidgetText, Label: i18n.L("ontology_admin.family.icon", "Icon"), Help: i18n.L("ontology_admin.family.icon_help", "Heroicon name (e.g. 'project-diagram')")}
	displayOrderField := formschema.FieldDef{Name: "display_order", Widget: formschema.WidgetNumber, Label: i18n.L("ontology_admin.family.display_order", "Display order")}

	if existing != nil {
		slugField.Value = existing.Slug
		nameField.Value = existing.Name
		descField.Value = existing.Description
		if existing.ParentFamilyID != nil {
			parentField.Value = *existing.ParentFamilyID
		}
		homepageField.Value = existing.HomepageURL
		iconField.Value = existing.Icon
		displayOrderField.Value = existing.DisplayOrder
	}

	schema.Sections = []formschema.Section{
		{
			ID:     "identity",
			Label:  i18n.L("forms.sections.identity", "Identity"),
			Fields: []formschema.FieldDef{slugField, nameField, descField, parentField},
		},
		{
			ID:     "presentation",
			Label:  i18n.L("ontology_admin.family.section_presentation", "Presentation"),
			Fields: []formschema.FieldDef{homepageField, iconField, displayOrderField},
		},
	}
	return schema
}

// parentFamilyOptions builds the select options for the parent_family_id
// field. The leading empty option represents "no parent" (root family).
// When editing, the family being edited is excluded so it can't be its
// own parent.
func parentFamilyOptions(existing *domain.OntologyFamily, families []*domain.OntologyFamily) []formschema.SelectOption {
	opts := make([]formschema.SelectOption, 0, len(families)+1)
	opts = append(opts, formschema.SelectOption{
		Value: "",
		Label: i18n.L("ontology_admin.family.parent_family_none", "— No parent (root family) —"),
	})
	for _, fam := range families {
		if fam == nil {
			continue
		}
		if existing != nil && fam.ID == existing.ID {
			continue
		}
		opts = append(opts, formschema.SelectOption{
			Value: fam.ID,
			Label: domain.Translations{"en": fam.Name},
		})
	}
	return opts
}

// ----------------------------------------------------------------------------
// Ontology list / form schemas
// ----------------------------------------------------------------------------

// BuildOntologyListSchema drives the master ontology admin tab.
func BuildOntologyListSchema(lang string, languages []formschema.LanguageInfo) *formschema.ListSchema {
	return &formschema.ListSchema{
		EntityType: "ontology",
		EmptyState: &formschema.EmptyState{
			Icon:    "academic-cap",
			Title:   i18n.L("ontology_admin.ontology.empty_title", "No ontologies"),
			Message: i18n.L("ontology_admin.ontology.empty_message", "Register an ontology so projects can link versions of it."),
		},
		DataURL: "/admin/ontologies",
		Caps: formschema.Capabilities{
			Create: &formschema.CreateCap{
				Label:         i18n.L("ontology_admin.ontology.add", "Register ontology"),
				FormSchemaURL: ontologyFormSchemaCreate,
			},
			Edit: &formschema.EditCap{
				FormSchemaURLTemplate: ontologyFormSchemaEditTpl,
			},
			Delete: &formschema.DeleteCap{
				URLTemplate: "/admin/ontologies/{id}",
			},
		},
		Columns: []formschema.Column{
			{Key: "name", Label: i18n.L("forms.name", "Name"), Type: "text", Primary: true},
			{Key: "prefix", Label: i18n.L("namespace_binding.list.prefix", "Prefix"), Type: "text"},
			{Key: "namespace", Label: i18n.L("namespace_binding.list.namespace", "Namespace"), Type: "text"},
			{Key: "ontology_type", Label: i18n.L("ontology_admin.ontology.type", "Type"), Type: "badge"},
		},
	}
}

func BuildOntologyEntityListSchema(lang string, languages []formschema.LanguageInfo, familyOptions []formschema.FilterOption) *formschema.EntityListSchema {
	return &formschema.EntityListSchema{
		EntityType:        "ontology",
		Title:             i18n.L("ontology_admin.ontology.title", "Ontologies"),
		DataURL:           "/admin/ontologies/data",
		DataKey:           "items",
		DetailURLTemplate: "/admin/ontologies/{id}/page",
		ProjectID:         "",
		EmptyState: &formschema.EmptyState{
			Icon:    "academic-cap",
			Title:   i18n.L("ontology_admin.ontology.empty_title", "No ontologies"),
			Message: i18n.L("ontology_admin.ontology.empty_message", "Register an ontology so projects can link versions of it."),
		},
		Search: &formschema.SearchConfig{
			Placeholder: i18n.L("ontology_admin.ontology.search_placeholder", "Search ontologies..."),
			ParamName:   "search",
		},
		Filters: []formschema.FilterConfig{
			{
				Key:       "ontology_type",
				Label:     i18n.L("ontology_admin.ontology.type", "Type"),
				ParamName: "ontology_type",
				Type:      "select",
				Options: []formschema.FilterOption{
					{Value: string(domain.OntologyTypeBase), Label: i18n.L("ontology_admin.ontology.type_base", "Base")},
					{Value: string(domain.OntologyTypeExtension), Label: i18n.L("ontology_admin.ontology.type_extension", "Extension")},
				},
			},
			{
				Key:       "family_id",
				Label:     i18n.L("ontology_admin.ontology.family", "Family"),
				ParamName: "family_id",
				Type:      "select",
				Options:   familyOptions,
			},
		},
		SortOptions: []formschema.SortOption{
			{Value: "prefix", Label: i18n.L("namespace_binding.list.prefix", "Prefix")},
			{Value: "name", Label: i18n.L("forms.name", "Name")},
			{Value: "family", Label: i18n.L("ontology_admin.ontology.family", "Family")},
			{Value: "ontology_type", Label: i18n.L("ontology_admin.ontology.type", "Type")},
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
			TitleField:    "name",
			SubtitleField: "namespace",
			IdentityFields: []formschema.IdentityField{
				{Key: "prefix", Style: "mono"},
			},
			SemanticBadges: []formschema.BadgeConfig{
				{Key: "ontology_type", Type: "text", Style: "purple"},
				{Key: "family_name", Type: "text", Style: "blue", HideEmpty: true},
			},
		},
		Capabilities: &formschema.Capabilities{
			Create: &formschema.CreateCap{
				Label:         i18n.L("ontology_admin.ontology.add", "Register ontology"),
				FormSchemaURL: ontologyFormSchemaCreate,
			},
			Edit: &formschema.EditCap{
				FormSchemaURLTemplate: ontologyFormSchemaEditTpl,
			},
			Delete: &formschema.DeleteCap{
				URLTemplate: "/admin/ontologies/{id}",
			},
		},
		RowActions: []formschema.RowAction{
			{ID: "edit", Icon: "pencil", Label: i18n.L("common.edit", "Edit")},
			{ID: "delete", Icon: "trash", Label: i18n.L("common.remove", "Remove"), Style: "danger"},
		},
		UI: formschema.SchemaUI{
			Languages:   languages,
			PrimaryLang: lang,
		},
	}
}

// BuildOntologyFormSchema is the create / edit form for an ontology.
func BuildOntologyFormSchema(mode string, existing *domain.Ontology, lang string, languages []formschema.LanguageInfo) *formschema.FormSchema {
	schema := &formschema.FormSchema{
		EntityType: "ontology",
		Mode:       mode,
		UI: formschema.SchemaUI{
			Languages:   languages,
			PrimaryLang: lang,
		},
	}
	switch mode {
	case formschema.ModeCreate:
		schema.Endpoint = &formschema.SchemaEndpoint{Method: "POST", URL: "/admin/ontologies"}
		schema.UI.SubmitLabel = i18n.L("ontology_admin.ontology.add", "Register ontology")
	case formschema.ModeEdit:
		if existing != nil {
			schema.Endpoint = &formschema.SchemaEndpoint{Method: "PUT", URL: "/admin/ontologies/" + existing.ID}
		}
		schema.UI.SubmitLabel = i18n.L("forms.save_changes", "Save Changes")
	}
	schema.UI.CancelLabel = i18n.L("forms.cancel", "Cancel")

	one := 1
	threeHundred := 300

	prefixField := formschema.FieldDef{
		Name:       "prefix",
		Widget:     formschema.WidgetText,
		Required:   true,
		Readonly:   mode == formschema.ModeEdit, // prefix is the natural key; immutable
		Label:      i18n.L("namespace_binding.list.prefix", "Prefix"),
		Help:       i18n.L("ontology_admin.ontology.prefix_help", "Short namespace prefix (e.g. 'crm', 'skos')."),
		Validation: &formschema.ValidationRules{MinLength: &one, MaxLength: &threeHundred, Pattern: "^[a-z][a-z0-9_-]*$"},
	}
	namespaceField := formschema.FieldDef{
		Name:     "namespace",
		Widget:   formschema.WidgetText,
		Required: true,
		Label:    i18n.L("ontology_admin.ontology.namespace_iri", "Namespace IRI"),
		Help:     i18n.L("ontology_admin.ontology.namespace_iri_help", "Full IRI prefix used in RDF (e.g. 'http://www.cidoc-crm.org/cidoc-crm/')."),
	}
	nameField := formschema.FieldDef{
		Name:     "name",
		Widget:   formschema.WidgetText,
		Required: true,
		Label:    i18n.L("ontology_admin.ontology.display_name", "Display name"),
	}
	descField := formschema.FieldDef{Name: "description", Widget: formschema.WidgetMultilingualText, Label: i18n.L("forms.description", "Description")}
	familyField := formschema.FieldDef{
		Name:       "family_id",
		Widget:     formschema.WidgetSelect,
		Label:      i18n.L("ontology_admin.ontology.family", "Family"),
		OptionsURL: "/admin/ontologies/families/options",
	}
	typeField := formschema.FieldDef{
		Name:     "ontology_type",
		Widget:   formschema.WidgetSelect,
		Required: true,
		Label:    i18n.L("ontology_admin.ontology.type", "Type"),
		Options: []formschema.SelectOption{
			{Value: string(domain.OntologyTypeBase), Label: i18n.L("ontology_admin.ontology.type_base", "Base")},
			{Value: string(domain.OntologyTypeExtension), Label: i18n.L("ontology_admin.ontology.type_extension", "Extension")},
		},
	}
	extendsField := formschema.FieldDef{
		Name:       "extends_ontology_id",
		Widget:     formschema.WidgetSelect,
		Label:      i18n.L("ontology_admin.ontology.extends", "Extends ontology"),
		Help:       i18n.L("ontology_admin.ontology.extends_help", "Required when type is 'extension'."),
		OptionsURL: "/admin/ontologies/options?ontology_type=base",
	}
	homepageField := formschema.FieldDef{Name: "homepage_url", Widget: formschema.WidgetText, Label: i18n.L("ontology_admin.family.homepage_url", "Homepage URL")}
	sourceField := formschema.FieldDef{Name: "source_url", Widget: formschema.WidgetText, Label: i18n.L("ontology_admin.ontology.source_url", "Source URL")}

	if existing != nil {
		prefixField.Value = existing.Prefix
		namespaceField.Value = existing.Namespace
		nameField.Value = existing.Name
		descField.Value = existing.Description
		familyField.Value = existing.FamilyID
		typeField.Value = string(existing.OntologyType)
		extendsField.Value = existing.ExtendsOntologyID
		homepageField.Value = existing.HomepageURL
		sourceField.Value = existing.SourceURL
	} else {
		typeField.Value = string(domain.OntologyTypeBase)
	}

	schema.Sections = []formschema.Section{
		{
			ID:     "identity",
			Label:  i18n.L("forms.sections.identity", "Identity"),
			Fields: []formschema.FieldDef{prefixField, namespaceField, nameField, descField, familyField},
		},
		{
			ID:     "type",
			Label:  i18n.L("ontology_admin.ontology.type", "Type"),
			Fields: []formschema.FieldDef{typeField, extendsField},
		},
		{
			ID:     "links",
			Label:  i18n.L("ontology_admin.ontology.section_links", "External links"),
			Fields: []formschema.FieldDef{homepageField, sourceField},
		},
	}
	return schema
}

// ----------------------------------------------------------------------------
// Version list / form schemas
// ----------------------------------------------------------------------------

// BuildVersionListSchema drives the per-ontology versions tab. Mounted
// under /admin/ontologies/{ontologyID}/versions.
func BuildVersionListSchema(ontologyID, lang string, languages []formschema.LanguageInfo) *formschema.ListSchema {
	return &formschema.ListSchema{
		EntityType: "ontology-version",
		EmptyState: &formschema.EmptyState{
			Icon:    "clock",
			Title:   i18n.L("ontology_admin.version.empty_title", "No versions imported"),
			Message: i18n.L("ontology_admin.version.empty_message", "Open the ontology management page and use Import version to upload RDF."),
		},
		DataURL: "/admin/ontologies/" + ontologyID + "/versions",
		Caps: formschema.Capabilities{
			Edit: &formschema.EditCap{
				FormSchemaURLTemplate: versionFormSchemaEditTpl,
			},
			Delete: &formschema.DeleteCap{
				URLTemplate: "/admin/ontologies/versions/{id}",
			},
		},
		Columns: []formschema.Column{
			{Key: "version.version_string", Label: i18n.L("ontology_admin.version.version_label", "Version"), Type: "text", Primary: true},
			{Key: "version.is_active", Label: i18n.L("ontology_admin.version.active", "Active"), Type: "badge", BadgeStyle: "green", HideZero: true},
			{Key: "version.class_count", Label: i18n.L("ontology_admin.version.classes", "Classes"), Type: "number"},
			{Key: "version.property_count", Label: i18n.L("ontology_admin.version.properties", "Properties"), Type: "number"},
			{Key: "project_count", Label: i18n.L("common.projects", "Projects"), Type: "badge", BadgeStyle: "purple"},
		},
	}
}

// BuildVersionFormSchema is the metadata-edit form for a version.
// Create flow lives in RDF import; this form covers post-import metadata
// tweaks (compatibility, label, comment, import notes, etc.).
func BuildVersionFormSchema(existing *domain.OntologyVersion, lang string, languages []formschema.LanguageInfo) *formschema.FormSchema {
	schema := &formschema.FormSchema{
		EntityType: "ontology-version",
		Mode:       formschema.ModeEdit,
		UI: formschema.SchemaUI{
			Languages:   languages,
			PrimaryLang: lang,
			SubmitLabel: i18n.L("forms.save_changes", "Save Changes"),
			CancelLabel: i18n.L("forms.cancel", "Cancel"),
		},
	}
	if existing != nil {
		schema.Endpoint = &formschema.SchemaEndpoint{
			Method: "PUT",
			URL:    "/admin/ontologies/versions/" + existing.ID,
		}
	}

	one := 1
	thirty := 30

	versionStringField := formschema.FieldDef{
		Name:       "version_string",
		Widget:     formschema.WidgetText,
		Required:   true,
		Label:      i18n.L("ontology_admin.version.version_string", "Version string"),
		Help:       i18n.L("ontology_admin.version.version_string_help", "Semver-style version (e.g. '7.1.3')."),
		Validation: &formschema.ValidationRules{MinLength: &one, MaxLength: &thirty},
	}
	uriField := formschema.FieldDef{
		Name:   "ontology_uri",
		Widget: formschema.WidgetText,
		Label:  i18n.L("ontology_admin.version.ontology_uri", "Ontology URI"),
	}
	versionIRIField := formschema.FieldDef{
		Name:   "version_iri",
		Widget: formschema.WidgetText,
		Label:  i18n.L("ontology_admin.version.version_iri", "Version IRI"),
	}
	versionInfoField := formschema.FieldDef{
		Name:   "version_info",
		Widget: formschema.WidgetMultilingualTextarea,
		Label:  i18n.L("ontology_admin.version.version_info", "Version notes"),
		Help:   i18n.L("ontology_admin.version.version_info_help", "Admin-facing notes about this ontology version."),
	}
	compatibleField := formschema.FieldDef{
		Name:   "compatible_base_versions_text",
		Widget: formschema.WidgetTextarea,
		Label:  i18n.L("ontology_admin.version.compatible_base_versions", "Compatible base versions"),
		Help:   i18n.L("ontology_admin.version.compatible_base_versions_help", "Comma or newline separated base ontology versions this import is compatible with."),
	}
	importedOntologiesField := formschema.FieldDef{
		Name:   "imported_ontologies_text",
		Widget: formschema.WidgetTextarea,
		Label:  i18n.L("ontology_admin.version.imported_ontologies", "Imported ontologies"),
		Help:   i18n.L("ontology_admin.version.imported_ontologies_help", "Comma or newline separated ontology imports declared by the source file."),
	}
	labelField := formschema.FieldDef{Name: "ontology_label", Widget: formschema.WidgetMultilingualText, Label: i18n.L("ontology_admin.version.label", "Label")}
	commentField := formschema.FieldDef{Name: "ontology_comment", Widget: formschema.WidgetMultilingualText, Label: i18n.L("ontology_admin.version.comment", "Comment")}
	originalFilenameField := formschema.FieldDef{Name: "original_filename", Widget: formschema.WidgetText, Readonly: true, Label: i18n.L("ontology_admin.version.original_filename", "Original filename")}
	fileMD5Field := formschema.FieldDef{Name: "file_md5", Widget: formschema.WidgetText, Readonly: true, Label: i18n.L("ontology_admin.version.file_md5", "File MD5")}

	if existing != nil {
		versionStringField.Value = existing.VersionString
		uriField.Value = existing.OntologyURI
		versionIRIField.Value = existing.VersionIRI
		versionInfoField.Value = existing.VersionInfo
		compatibleField.Value = strings.Join(existing.CompatibleBaseVersions, "\n")
		importedOntologiesField.Value = strings.Join(existing.ImportedOntologies, "\n")
		labelField.Value = existing.OntologyLabel
		commentField.Value = existing.OntologyComment
		originalFilenameField.Value = existing.OriginalFilename
		fileMD5Field.Value = existing.FileMD5
	}

	schema.Sections = []formschema.Section{
		{
			ID:     "identity",
			Label:  i18n.L("forms.sections.identity", "Identity"),
			Fields: []formschema.FieldDef{versionStringField, uriField, versionIRIField, versionInfoField},
		},
		{
			ID:     "documentation",
			Label:  i18n.L("ontology_admin.version.section_documentation", "Documentation"),
			Fields: []formschema.FieldDef{labelField, commentField},
		},
		{
			ID:     "compatibility",
			Label:  i18n.L("ontology_admin.version.section_compatibility", "Compatibility"),
			Fields: []formschema.FieldDef{compatibleField, importedOntologiesField},
		},
		{
			ID:        "source",
			Label:     i18n.L("ontology_admin.version.section_source", "Source file"),
			Collapsed: true,
			Fields:    []formschema.FieldDef{originalFilenameField, fileMD5Field},
		},
	}
	return schema
}
