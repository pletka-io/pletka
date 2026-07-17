package ontology

import (
	"fmt"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/formschema"
)

func BuildFamilyLandingPageSchema(model *FamilyLandingPageModel, lang string, languages []formschema.LanguageInfo) *formschema.OntologyPageSchema {
	return buildFamilyLandingPageSchema(model, lang, languages, false)
}

func BuildAdminFamilyLandingPageSchema(model *FamilyLandingPageModel, lang string, languages []formschema.LanguageInfo) *formschema.OntologyPageSchema {
	return buildFamilyLandingPageSchema(model, lang, languages, true)
}

func buildFamilyLandingPageSchema(model *FamilyLandingPageModel, lang string, languages []formschema.LanguageInfo, admin bool) *formschema.OntologyPageSchema {
	cards := make([]formschema.OntologyPageCard, 0, len(model.Families))
	for _, family := range model.Families {
		cards = append(cards, familyPageCard(family, lang, admin))
	}
	return &formschema.OntologyPageSchema{
		Kind:     "ontology-family-landing",
		Title:    lt(lang, "Ontology families"),
		Subtitle: lt(lang, "Browse ontology families first, then drill into base ontologies, extensions, and versions."),
		Breadcrumbs: []formschema.OntologyPageLink{
			{Label: lt(lang, "Ontologies")},
		},
		Sections: []formschema.OntologyPageSection{
			{
				ID:     "summary",
				Widget: "stats-strip",
				Stats: []formschema.OntologyPageStat{
					{Label: lt(lang, "Families"), Value: fmt.Sprintf("%d", model.FamilyCount), Tone: "blue"},
					{Label: lt(lang, "Root families"), Value: fmt.Sprintf("%d", model.RootFamilyCount), Tone: "green"},
					{Label: lt(lang, "Ontologies"), Value: fmt.Sprintf("%d", model.OntologyCount), Tone: "purple"},
				},
			},
			{
				ID:          "families",
				Widget:      "card-list",
				Title:       lt(lang, "Families"),
				Description: lt(lang, "Each family groups related ontology vocabularies and extensions."),
				Cards:       cards,
				EmptyText:   lt(lang, "No ontology families are available."),
			},
		},
		Actions: []formschema.OntologyPageAction{
			{ID: "all", Label: lt(lang, "Browse all ontologies"), Href: "/ontologies/all", Style: "secondary"},
		},
		UI: schemaUI(lang, languages),
	}
}

func BuildAllOntologiesPageSchema(model *AllOntologiesPageModel, lang string, languages []formschema.LanguageInfo) *formschema.OntologyPageSchema {
	return buildAllOntologiesPageSchema(model, lang, languages, false)
}

func BuildAdminAllOntologiesPageSchema(model *AllOntologiesPageModel, lang string, languages []formschema.LanguageInfo) *formschema.OntologyPageSchema {
	return buildAllOntologiesPageSchema(model, lang, languages, true)
}

func buildAllOntologiesPageSchema(model *AllOntologiesPageModel, lang string, languages []formschema.LanguageInfo, admin bool) *formschema.OntologyPageSchema {
	cards := make([]formschema.OntologyPageCard, 0, len(model.Ontologies))
	for _, ontology := range model.Ontologies {
		cards = append(cards, ontologyPageCard(ontology, lang, admin))
	}
	return &formschema.OntologyPageSchema{
		Kind:     "ontology-all",
		Title:    lt(lang, "All ontologies"),
		Subtitle: lt(lang, "Flat browse view across ontology families."),
		Breadcrumbs: []formschema.OntologyPageLink{
			{Label: lt(lang, "Ontologies"), Href: "/ontologies"},
			{Label: lt(lang, "All ontologies")},
		},
		Sections: []formschema.OntologyPageSection{
			{
				ID:        "ontologies",
				Widget:    "card-list",
				Title:     lt(lang, "Ontologies"),
				Cards:     cards,
				EmptyText: lt(lang, "No ontologies are available."),
			},
		},
		UI: schemaUI(lang, languages),
	}
}

func BuildFamilyDetailPageSchema(model *FamilyDetailPageModel, lang string, languages []formschema.LanguageInfo) *formschema.OntologyPageSchema {
	return buildFamilyDetailPageSchema(model, lang, languages, false)
}

func BuildAdminFamilyDetailPageSchema(model *FamilyDetailPageModel, lang string, languages []formschema.LanguageInfo) *formschema.OntologyPageSchema {
	return buildFamilyDetailPageSchema(model, lang, languages, true)
}

func buildFamilyDetailPageSchema(model *FamilyDetailPageModel, lang string, languages []formschema.LanguageInfo, admin bool) *formschema.OntologyPageSchema {
	childCards := make([]formschema.OntologyPageCard, 0, len(model.ChildFamilies))
	for _, family := range model.ChildFamilies {
		childCards = append(childCards, familyPageCard(family, lang, admin))
	}
	baseCards := make([]formschema.OntologyPageCard, 0, len(model.BaseOntologies))
	for _, ontology := range model.BaseOntologies {
		baseCards = append(baseCards, ontologyPageCard(ontology, lang, admin))
	}
	extensionCards := make([]formschema.OntologyPageCard, 0, len(model.Extensions))
	for _, ontology := range model.Extensions {
		extensionCards = append(extensionCards, ontologyPageCard(ontology, lang, admin))
	}
	sections := []formschema.OntologyPageSection{
		{
			ID:          "overview",
			Widget:      "hero",
			Title:       lt(lang, model.Name),
			Description: lt(lang, model.Description),
			Links: externalLinks(lang, map[string]string{
				"Homepage": model.HomepageURL,
			}),
		},
		{
			ID:     "stats",
			Widget: "stats-strip",
			Stats: []formschema.OntologyPageStat{
				{Label: lt(lang, "Child families"), Value: fmt.Sprintf("%d", len(model.ChildFamilies)), Tone: "blue"},
				{Label: lt(lang, "Base ontologies"), Value: fmt.Sprintf("%d", len(model.BaseOntologies)), Tone: "green"},
				{Label: lt(lang, "Extensions"), Value: fmt.Sprintf("%d", len(model.Extensions)), Tone: "purple"},
			},
		},
		{
			ID:     "tabs",
			Widget: "tabs",
			Tabs: []formschema.OntologyPageTab{
				{ID: "overview", Label: lt(lang, "Overview"), Href: "?tab=overview", Active: model.ActiveTab == "overview"},
				{ID: "bases", Label: lt(lang, "Base ontologies"), Href: "?tab=bases", Active: model.ActiveTab == "bases", Count: len(model.BaseOntologies)},
				{ID: "extensions", Label: lt(lang, "Extensions"), Href: "?tab=extensions", Active: model.ActiveTab == "extensions", Count: len(model.Extensions)},
			},
		},
	}
	switch model.ActiveTab {
	case "bases":
		sections = append(sections, formschema.OntologyPageSection{ID: "base-ontologies", Widget: "card-list", Title: lt(lang, "Base ontologies"), Cards: baseCards, EmptyText: lt(lang, "No base ontologies in this family.")})
	case "extensions":
		sections = append(sections, formschema.OntologyPageSection{ID: "extension-ontologies", Widget: "card-list", Title: lt(lang, "Extensions"), Cards: extensionCards, EmptyText: lt(lang, "No extensions in this family.")})
	default:
		sections = append(sections,
			formschema.OntologyPageSection{ID: "child-families", Widget: "card-list", Title: lt(lang, "Child families"), Cards: childCards, EmptyText: lt(lang, "No child families.")},
			formschema.OntologyPageSection{ID: "base-ontologies", Widget: "card-list", Title: lt(lang, "Base ontologies"), Cards: previewPageCards(baseCards, 3), EmptyText: lt(lang, "No base ontologies in this family.")},
			formschema.OntologyPageSection{ID: "extension-ontologies", Widget: "card-list", Title: lt(lang, "Extensions"), Cards: previewPageCards(extensionCards, 3), EmptyText: lt(lang, "No extensions in this family.")},
		)
	}
	return &formschema.OntologyPageSchema{
		Kind:        "ontology-family",
		Title:       lt(lang, model.Name),
		Subtitle:    lt(lang, "Family overview for related base ontologies and extensions."),
		Breadcrumbs: prependOntologyBreadcrumb(lang, model.Breadcrumbs, model.Name),
		Sections:    sections,
		UI:          schemaUI(lang, languages),
	}
}

func BuildOntologyDetailPageSchema(model *OntologyDetailPageModel, lang string, languages []formschema.LanguageInfo, admin bool) *formschema.OntologyPageSchema {
	familyURL := model.FamilyURL
	if admin && model.FamilyAdminURL != "" {
		familyURL = model.FamilyAdminURL
	}
	sections := []formschema.OntologyPageSection{
		{
			ID:          "overview",
			Widget:      "hero",
			Title:       lt(lang, model.Name),
			Description: lt(lang, model.Description),
			Links: externalLinks(lang, map[string]string{
				"Homepage": model.HomepageURL,
				"Source":   model.SourceURL,
			}),
		},
		{
			ID:     "metadata",
			Widget: "metadata-list",
			Metadata: []formschema.OntologyPageMetadata{
				{Label: lt(lang, "Prefix"), Value: model.Prefix, Code: true},
				{Label: lt(lang, "Namespace"), Value: model.Namespace, Code: true},
				{Label: lt(lang, "Type"), Value: model.OntologyType},
				{Label: lt(lang, "Family"), Value: model.FamilyName, Href: familyURL},
			},
		},
	}
	if model.ExtendsOntology != nil {
		sections = append(sections, formschema.OntologyPageSection{
			ID:     "extends",
			Widget: "callout",
			Tone:   "amber",
			Title:  lt(lang, "Extends base ontology"),
			Cards:  []formschema.OntologyPageCard{ontologyPageCard(*model.ExtendsOntology, lang, admin)},
		})
	}
	if len(model.Extensions) > 0 {
		cards := make([]formschema.OntologyPageCard, 0, len(model.Extensions))
		for _, ontology := range model.Extensions {
			cards = append(cards, ontologyPageCard(ontology, lang, admin))
		}
		sections = append(sections, formschema.OntologyPageSection{
			ID:          "extensions",
			Widget:      "card-list",
			Title:       lt(lang, "Extension ontologies"),
			Description: lt(lang, "Ontologies in this ecosystem that build on this ontology."),
			Cards:       cards,
		})
	}
	versionCards := make([]formschema.OntologyPageCard, 0, len(model.Versions))
	for _, version := range model.Versions {
		versionCards = append(versionCards, versionPageCard(version, lang, admin))
	}
	sections = append(sections, formschema.OntologyPageSection{
		ID:          "versions",
		Widget:      "card-list",
		Title:       lt(lang, "Versions"),
		Description: lt(lang, "Browse imported ontology versions and jump into classes or properties."),
		Cards:       versionCards,
		EmptyText:   lt(lang, "No versions imported."),
	})
	actions := []formschema.OntologyPageAction{}
	if admin {
		ontologyID := ontologyModelID(model)
		editAction := formschema.OntologyPageAction{
			ID:    "edit",
			Label: lt(lang, "Edit ontology"),
			Style: "secondary",
		}
		importAction := formschema.OntologyPageAction{
			ID:    "import",
			Label: lt(lang, "Import version"),
			Style: "primary",
		}
		if ontologyID != "" {
			editAction.FormSchemaURL = "/admin/ontologies/form-schema?mode=edit&entity_id=" + ontologyID
			importAction.Href = "/admin/ontologies/" + ontologyID + "/versions/import/page"
		}
		actions = append(actions,
			editAction,
			importAction,
		)
	}
	return &formschema.OntologyPageSchema{
		Kind:        "ontology-detail",
		Title:       lt(lang, model.Name),
		Subtitle:    lt(lang, model.Namespace),
		Breadcrumbs: linksToSchema(lang, model.Breadcrumbs),
		Sections:    sections,
		Actions:     actions,
		UI:          schemaUI(lang, languages),
	}
}

func BuildVersionDetailPageSchema(model *VersionDetailPageModel, lang string, languages []formschema.LanguageInfo, admin bool) *formschema.OntologyPageSchema {
	version := model.Version
	sections := []formschema.OntologyPageSection{
		{
			ID:          "overview",
			Widget:      "hero",
			Title:       lt(lang, model.Prefix+" "+model.VersionString),
			Description: lt(lang, firstNonEmpty(version.VersionInfo, summarizeText(version.OntologyComment, 320))),
		},
		{
			ID:     "stats",
			Widget: "stats-strip",
			Stats: []formschema.OntologyPageStat{
				{Label: lt(lang, "State"), Value: lifecycleState(version), Tone: lifecycleTone(version)},
				{Label: lt(lang, "Classes"), Value: fmt.Sprintf("%d", version.ClassCount), Tone: "blue"},
				{Label: lt(lang, "Properties"), Value: fmt.Sprintf("%d", version.PropertyCount), Tone: "green"},
				{Label: lt(lang, "Projects"), Value: fmt.Sprintf("%d", version.ProjectCount), Tone: "purple"},
				{Label: lt(lang, "Imports"), Value: fmt.Sprintf("%d", len(version.ImportedOntologies)), Tone: "amber"},
			},
		},
		{
			ID:     "tabs",
			Widget: "tabs",
			Tabs: []formschema.OntologyPageTab{
				{ID: "overview", Label: lt(lang, "Overview"), Href: version.VersionURL, Active: model.ActiveTab == "overview"},
				{ID: "classes", Label: lt(lang, "Classes"), Href: version.ClassesURL, Active: model.ActiveTab == "classes", Count: int(version.ClassCount)},
				{ID: "properties", Label: lt(lang, "Properties"), Href: version.PropertiesURL, Active: model.ActiveTab == "properties", Count: int(version.PropertyCount)},
			},
		},
		{
			ID:     "lifecycle",
			Widget: "metadata-list",
			Title:  lt(lang, "Lifecycle"),
			Metadata: []formschema.OntologyPageMetadata{
				{Label: lt(lang, "State"), Value: lifecycleState(version)},
				{Label: lt(lang, "Project adoption"), Value: versionUsageSummary(version)},
				{Label: lt(lang, "Set active impact"), Value: activeVersionImpact(version, model.ActiveVersionString)},
			},
		},
		{
			ID:     "identity",
			Widget: "metadata-list",
			Title:  lt(lang, "Ontology identity"),
			Metadata: []formschema.OntologyPageMetadata{
				{Label: lt(lang, "Ontology URI"), Value: version.OntologyURI, Code: true},
				{Label: lt(lang, "Version IRI"), Value: version.VersionIRI, Code: true},
				{Label: lt(lang, "Version"), Value: version.VersionString},
				{Label: lt(lang, "Namespace"), Value: model.Namespace, Code: true},
			},
		},
		{
			ID:     "import-metadata",
			Widget: "metadata-list",
			Title:  lt(lang, "Import metadata"),
			Metadata: []formschema.OntologyPageMetadata{
				{Label: lt(lang, "Original filename"), Value: version.OriginalFilename},
				{Label: lt(lang, "File size"), Value: formatBytes(version.FileSize)},
				{Label: lt(lang, "File MD5"), Value: version.FileMD5, Code: true},
				{Label: lt(lang, "Imported"), Value: version.ParsedAt},
				{Label: lt(lang, "Last metadata update"), Value: version.UpdatedAt},
				{Label: lt(lang, "Summary"), Value: versionImportSummary(version)},
			},
		},
		{
			ID:     "compatibility",
			Widget: "metadata-list",
			Title:  lt(lang, "Compatibility"),
			Metadata: []formschema.OntologyPageMetadata{
				{Label: lt(lang, "Summary"), Value: versionCompatibilitySummary(version)},
				{Label: lt(lang, "Compatible base versions"), Value: stringsJoin(version.CompatibleBaseVersions)},
				{Label: lt(lang, "Imported ontologies"), Value: stringsJoin(version.ImportedOntologies)},
			},
		},
		{
			ID:     "browse",
			Widget: "entity-list-link",
			Title:  lt(lang, "Browse vocabulary"),
			Links: []formschema.OntologyPageLink{
				{Label: lt(lang, "Classes"), Href: version.ClassesURL, Icon: "cube"},
				{Label: lt(lang, "Properties"), Href: version.PropertiesURL, Icon: "list"},
			},
		},
	}
	if model.ActiveTab == "classes" {
		sections = append(sections, formschema.OntologyPageSection{
			ID:          "classes-list",
			Widget:      "entity-list",
			Title:       lt(lang, "Classes"),
			Description: lt(lang, "Browse classes imported for this ontology version."),
			SchemaURL:   "/ontologies/" + model.Prefix + "/" + model.VersionString + "/classes/list-schema",
		})
	} else if model.ActiveTab == "properties" {
		sections = append(sections, formschema.OntologyPageSection{
			ID:          "properties-list",
			Widget:      "entity-list",
			Title:       lt(lang, "Properties"),
			Description: lt(lang, "Browse properties imported for this ontology version."),
			SchemaURL:   "/ontologies/" + model.Prefix + "/" + model.VersionString + "/properties/list-schema",
		})
	}
	if version.OntologyComment != "" {
		sections = append(sections, formschema.OntologyPageSection{
			ID:          "comment",
			Widget:      "callout",
			Title:       lt(lang, "Description"),
			Description: lt(lang, version.OntologyComment),
		})
	}
	if admin {
		if len(model.ProjectUsages) > 0 {
			sections = append(sections, formschema.OntologyPageSection{
				ID:          "project-adoption",
				Widget:      "card-list",
				Title:       lt(lang, "Project adoption"),
				Description: lt(lang, projectAdoptionDescription(version, len(model.ProjectUsages))),
				Cards:       projectUsageCards(model.ProjectUsages, lang),
			})
		}
		sections = append(sections, formschema.OntologyPageSection{
			ID:          "import-history",
			Widget:      "card-list",
			Title:       lt(lang, "Import history"),
			Description: lt(lang, "Source-file audit details and importer-managed metadata for this version."),
			Cards:       []formschema.OntologyPageCard{importHistoryCard(version, lang)},
		})
		sections = append(sections, formschema.OntologyPageSection{
			ID:          "usage-impact",
			Widget:      "callout",
			Title:       lt(lang, "Usage impact"),
			Description: lt(lang, versionUsageImpact(version)),
			Tone:        usageImpactTone(version),
		})
	}
	actions := []formschema.OntologyPageAction{}
	if admin {
		if !version.IsActive {
			actions = append(actions, formschema.OntologyPageAction{
				ID:      "set-active",
				Label:   lt(lang, "Set active"),
				Method:  "POST",
				URL:     "/admin/ontologies/" + version.OntologyID + "/versions/" + version.ID + "/set-active",
				Confirm: lt(lang, setActiveConfirm(version, model.ActiveVersionString)),
				Style:   "primary",
			})
		}
		actions = append(actions,
			formschema.OntologyPageAction{
				ID:            "edit",
				Label:         lt(lang, "Edit metadata"),
				FormSchemaURL: "/admin/ontologies/versions/form-schema?mode=edit&entity_id=" + version.ID,
				Style:         "secondary",
			},
			formschema.OntologyPageAction{
				ID:    "import",
				Label: lt(lang, "Import version"),
				Href:  "/admin/ontologies/" + version.OntologyID + "/versions/import/page",
				Style: "secondary",
			},
		)
		if versionCanDelete(version) {
			actions = append(actions, deleteVersionAction(version, lang))
		}
	}
	return &formschema.OntologyPageSchema{
		Kind:        "ontology-version",
		Title:       lt(lang, model.Prefix+" "+model.VersionString),
		Subtitle:    lt(lang, "Version overview"),
		Breadcrumbs: linksToSchema(lang, model.Breadcrumbs),
		Sections:    sections,
		Actions:     actions,
		UI:          schemaUI(lang, languages),
	}
}

func BuildClassDetailPageSchema(model *ClassDetailPageModel, lang string, languages []formschema.LanguageInfo) *formschema.OntologyPageSchema {
	class := model.Class
	sections := []formschema.OntologyPageSection{
		{
			ID:          "overview",
			Widget:      "hero",
			Title:       lt(lang, class.Qname),
			Description: lt(lang, firstNonEmpty(model.Comment, model.Label)),
			Links: []formschema.OntologyPageLink{
				{Label: lt(lang, "Version overview"), Href: "/ontologies/" + model.Prefix + "/" + model.VersionString},
				{Label: lt(lang, "All classes"), Href: "/ontologies/" + model.Prefix + "/" + model.VersionString + "?tab=classes"},
			},
		},
		{
			ID:     "identity",
			Widget: "metadata-list",
			Title:  lt(lang, "Class identity"),
			Metadata: []formschema.OntologyPageMetadata{
				{Label: lt(lang, "Qname"), Value: class.Qname, Code: true},
				{Label: lt(lang, "Local name"), Value: class.LocalName, Code: true},
				{Label: lt(lang, "URI"), Value: class.URI, Code: true},
				{Label: lt(lang, "Label"), Value: model.Label},
			},
		},
	}
	sections = appendRelationSection(sections, "superclasses", "Sub-class of", model.SuperClasses, lang)
	sections = appendRelationSection(sections, "subclasses", "Direct subclasses", model.SubClasses, lang)
	sections = appendRelationSection(sections, "properties", "Properties with this domain", model.Properties, lang)
	return &formschema.OntologyPageSchema{
		Kind:        "ontology-class",
		Title:       lt(lang, class.Qname),
		Subtitle:    lt(lang, firstNonEmpty(model.Label, class.URI)),
		Breadcrumbs: linksToSchema(lang, model.Breadcrumbs),
		Sections:    sections,
		UI:          schemaUI(lang, languages),
	}
}

func BuildPropertyDetailPageSchema(model *PropertyDetailPageModel, lang string, languages []formschema.LanguageInfo) *formschema.OntologyPageSchema {
	property := model.Property
	flags := propertyFlags(property)
	sections := []formschema.OntologyPageSection{
		{
			ID:          "overview",
			Widget:      "hero",
			Title:       lt(lang, property.Qname),
			Description: lt(lang, firstNonEmpty(model.Comment, model.Label)),
			Links: []formschema.OntologyPageLink{
				{Label: lt(lang, "Version overview"), Href: "/ontologies/" + model.Prefix + "/" + model.VersionString},
				{Label: lt(lang, "All properties"), Href: "/ontologies/" + model.Prefix + "/" + model.VersionString + "/properties"},
			},
		},
		{
			ID:     "identity",
			Widget: "metadata-list",
			Title:  lt(lang, "Property identity"),
			Metadata: []formschema.OntologyPageMetadata{
				{Label: lt(lang, "Qname"), Value: property.Qname, Code: true},
				{Label: lt(lang, "Local name"), Value: property.LocalName, Code: true},
				{Label: lt(lang, "URI"), Value: property.URI, Code: true},
				{Label: lt(lang, "Label"), Value: model.Label},
				{Label: lt(lang, "Type"), Value: property.PropertyType},
				{Label: lt(lang, "Characteristics"), Value: stringsJoin(flags)},
			},
		},
	}
	sections = appendRelationSection(sections, "domain", "Domain", model.Domain, lang)
	sections = appendRelationSection(sections, "range", "Range", model.Range, lang)
	if model.Inverse != nil {
		sections = appendRelationSection(sections, "inverse", "Inverse property", []OntologyRelationLinkModel{*model.Inverse}, lang)
	}
	sections = appendRelationSection(sections, "superproperties", "Sub-property of", model.SuperProperties, lang)
	sections = appendRelationSection(sections, "subproperties", "Direct sub-properties", model.SubProperties, lang)
	return &formschema.OntologyPageSchema{
		Kind:        "ontology-property",
		Title:       lt(lang, property.Qname),
		Subtitle:    lt(lang, firstNonEmpty(model.Label, property.URI)),
		Breadcrumbs: linksToSchema(lang, model.Breadcrumbs),
		Sections:    sections,
		UI:          schemaUI(lang, languages),
	}
}

func BuildAdminImportVersionPageSchema(model *OntologyDetailPageModel, lang string, languages []formschema.LanguageInfo) *formschema.OntologyPageSchema {
	versionCards := make([]formschema.OntologyPageCard, 0, len(model.Versions))
	for _, version := range model.Versions {
		versionCards = append(versionCards, versionPageCard(version, lang, true))
	}
	return &formschema.OntologyPageSchema{
		Kind:     "ontology-import",
		Title:    lt(lang, "Import version"),
		Subtitle: lt(lang, model.Prefix+" version import"),
		Breadcrumbs: []formschema.OntologyPageLink{
			{Label: lt(lang, "Ontologies"), Href: "/admin/ontologies/page"},
			{Label: lt(lang, model.Prefix), Href: "/admin/ontologies/" + model.Ontology.ID + "/page"},
			{Label: lt(lang, "Import version")},
		},
		Sections: []formschema.OntologyPageSection{
			{
				ID:          "import-status",
				Widget:      "callout",
				Title:       lt(lang, "Upload RDF"),
				Description: lt(lang, "Upload an RDF/XML file, inspect the parsed summary, then import it as inactive or make it active immediately."),
				Tone:        "blue",
			},
			{
				ID:     "target",
				Widget: "metadata-list",
				Title:  lt(lang, "Target ontology"),
				Metadata: []formschema.OntologyPageMetadata{
					{Label: lt(lang, "Ontology"), Value: model.Name},
					{Label: lt(lang, "Prefix"), Value: model.Prefix, Code: true},
					{Label: lt(lang, "Ontology ID"), Value: model.Ontology.ID, Code: true},
					{Label: lt(lang, "Namespace"), Value: model.Namespace, Code: true},
				},
			},
			{
				ID:          "existing-versions",
				Widget:      "card-list",
				Title:       lt(lang, "Import history"),
				Description: lt(lang, "Imported versions, source files, compatibility metadata, and current adoption."),
				Cards:       versionCards,
				EmptyText:   lt(lang, "No versions imported."),
			},
		},
		Actions: []formschema.OntologyPageAction{
			{ID: "back", Label: lt(lang, "Back to ontology"), Href: "/admin/ontologies/" + model.Ontology.ID + "/page", Style: "secondary"},
		},
		Import: &formschema.OntologyImportConfig{
			ProbeURL:  "/admin/ontologies/" + model.Ontology.ID + "/versions/import/probe",
			CommitURL: "/admin/ontologies/" + model.Ontology.ID + "/versions/import",
			MaxBytes:  25 << 20,
		},
		UI: schemaUI(lang, languages),
	}
}

func familyPageCard(family FamilyPageCardModel, lang string, admin bool) formschema.OntologyPageCard {
	href := family.URL
	if admin && family.AdminURL != "" {
		href = family.AdminURL
	}
	return formschema.OntologyPageCard{
		ID:          family.ID,
		Title:       lt(lang, family.Name),
		Description: lt(lang, family.Description),
		Href:        href,
		Stats: []formschema.OntologyPageStat{
			{Label: lt(lang, "Ontologies"), Value: fmt.Sprintf("%d", family.OntologyCount)},
			{Label: lt(lang, "Child families"), Value: fmt.Sprintf("%d", family.ChildCount)},
		},
		Actions: externalActions(lang, map[string]string{
			"Homepage": family.HomepageURL,
		}),
	}
}

func ontologyPageCard(ontology OntologyPageCardModel, lang string, admin bool) formschema.OntologyPageCard {
	href := ontology.DetailURL
	if admin && ontology.AdminURL != "" {
		href = ontology.AdminURL
	}
	badges := []formschema.OntologyPageBadge{
		{Label: lt(lang, ontology.OntologyType), Tone: "slate"},
	}
	if ontology.FamilyName != "" {
		badges = append(badges, formschema.OntologyPageBadge{Label: lt(lang, ontology.FamilyName), Tone: "green"})
	}
	if ontology.ExtendsName != "" {
		badges = append(badges, formschema.OntologyPageBadge{Label: lt(lang, "Extends "+ontology.ExtendsName), Tone: "amber"})
	}
	return formschema.OntologyPageCard{
		ID:          ontology.ID,
		Title:       lt(lang, ontology.Prefix),
		Subtitle:    lt(lang, ontology.Name),
		Description: lt(lang, ontology.Description),
		Href:        href,
		Badges:      badges,
		Metadata: []formschema.OntologyPageMetadata{
			{Label: lt(lang, "Namespace"), Value: ontology.Namespace, Code: true},
		},
		Actions: externalActions(lang, map[string]string{
			"Homepage": ontology.HomepageURL,
			"Source":   ontology.SourceURL,
		}),
	}
}

func versionPageCard(version VersionPageCardModel, lang string, admin bool) formschema.OntologyPageCard {
	href := version.VersionURL
	if admin && version.AdminVersionURL != "" {
		href = version.AdminVersionURL
	}
	badges := []formschema.OntologyPageBadge{
		{Label: lt(lang, lifecycleState(version)), Tone: lifecycleTone(version)},
	}
	actions := []formschema.OntologyPageAction{
		{ID: "classes", Label: lt(lang, "Classes"), Href: version.ClassesURL, Style: "primary"},
		{ID: "properties", Label: lt(lang, "Properties"), Href: version.PropertiesURL, Style: "secondary"},
	}
	if admin && !version.IsActive {
		actions = append(actions, formschema.OntologyPageAction{
			ID:      "set-active",
			Label:   lt(lang, "Set active"),
			Method:  "POST",
			URL:     "/admin/ontologies/" + version.OntologyID + "/versions/" + version.ID + "/set-active",
			Confirm: lt(lang, setActiveConfirm(version, "")),
			Style:   "secondary",
		})
	}
	if admin && versionCanDelete(version) {
		actions = append(actions, deleteVersionAction(version, lang))
	}
	return formschema.OntologyPageCard{
		ID:          version.ID,
		Title:       lt(lang, version.VersionString),
		Description: lt(lang, firstNonEmpty(version.VersionInfo, summarizeText(version.OntologyComment, 180), versionImportSummary(version))),
		Href:        href,
		Badges:      badges,
		Stats: []formschema.OntologyPageStat{
			{Label: lt(lang, "Classes"), Value: fmt.Sprintf("%d", version.ClassCount)},
			{Label: lt(lang, "Properties"), Value: fmt.Sprintf("%d", version.PropertyCount)},
			{Label: lt(lang, "Projects"), Value: fmt.Sprintf("%d", version.ProjectCount)},
		},
		Metadata: []formschema.OntologyPageMetadata{
			{Label: lt(lang, "Usage"), Value: versionUsageSummary(version)},
			{Label: lt(lang, "Compatibility"), Value: versionCompatibilitySummary(version)},
			{Label: lt(lang, "Imported"), Value: version.ParsedAt},
			{Label: lt(lang, "Source file"), Value: version.OriginalFilename},
			{Label: lt(lang, "File size"), Value: formatBytes(version.FileSize)},
		},
		Actions: actions,
	}
}

func importHistoryCard(version VersionPageCardModel, lang string) formschema.OntologyPageCard {
	return formschema.OntologyPageCard{
		ID:          "import-history-" + version.ID,
		Title:       lt(lang, firstNonEmpty(version.OriginalFilename, "Source file")),
		Description: lt(lang, versionImportSummary(version)),
		Badges: []formschema.OntologyPageBadge{
			{Label: lt(lang, lifecycleState(version)), Tone: lifecycleTone(version)},
		},
		Stats: []formschema.OntologyPageStat{
			{Label: lt(lang, "Classes"), Value: fmt.Sprintf("%d", version.ClassCount)},
			{Label: lt(lang, "Properties"), Value: fmt.Sprintf("%d", version.PropertyCount)},
			{Label: lt(lang, "Projects"), Value: fmt.Sprintf("%d", version.ProjectCount)},
		},
		Metadata: []formschema.OntologyPageMetadata{
			{Label: lt(lang, "Imported"), Value: version.ParsedAt},
			{Label: lt(lang, "Created"), Value: version.CreatedAt},
			{Label: lt(lang, "Updated"), Value: version.UpdatedAt},
			{Label: lt(lang, "File size"), Value: formatBytes(version.FileSize)},
			{Label: lt(lang, "File MD5"), Value: version.FileMD5, Code: true},
			{Label: lt(lang, "Compatible base versions"), Value: stringsJoin(version.CompatibleBaseVersions)},
			{Label: lt(lang, "Imported ontologies"), Value: stringsJoin(version.ImportedOntologies)},
			{Label: lt(lang, "Importer metadata"), Value: version.OntologyMetadataSummary},
		},
	}
}

func projectUsageCards(usages []VersionProjectUsageModel, lang string) []formschema.OntologyPageCard {
	cards := make([]formschema.OntologyPageCard, 0, len(usages))
	for _, usage := range usages {
		badges := []formschema.OntologyPageBadge{{Label: lt(lang, "Linked"), Tone: "slate"}}
		if usage.IsPrimary {
			badges = append(badges, formschema.OntologyPageBadge{Label: lt(lang, "Primary"), Tone: "green"})
		}
		cards = append(cards, formschema.OntologyPageCard{
			ID:       "project-usage-" + usage.ProjectID,
			Title:    lt(lang, firstNonEmpty(usage.ProjectName, usage.ProjectID)),
			Subtitle: lt(lang, usage.ProjectID),
			Href:     usage.ProjectURL,
			Badges:   badges,
			Metadata: []formschema.OntologyPageMetadata{
				{Label: lt(lang, "Linked since"), Value: usage.AddedAt},
				{Label: lt(lang, "Role"), Value: projectUsageRole(usage)},
			},
		})
	}
	return cards
}

func appendRelationSection(sections []formschema.OntologyPageSection, id, title string, links []OntologyRelationLinkModel, lang string) []formschema.OntologyPageSection {
	cards := relationCards(links, lang)
	if len(cards) == 0 {
		return sections
	}
	return append(sections, formschema.OntologyPageSection{
		ID:     id,
		Widget: "card-list",
		Title:  lt(lang, title),
		Cards:  cards,
	})
}

func relationCards(links []OntologyRelationLinkModel, lang string) []formschema.OntologyPageCard {
	cards := make([]formschema.OntologyPageCard, 0, len(links))
	for _, link := range links {
		if link.Qname == "" {
			continue
		}
		cards = append(cards, formschema.OntologyPageCard{
			ID:          stringsID(link.Kind + "-" + link.Qname),
			Title:       lt(lang, link.Qname),
			Description: lt(lang, link.Label),
			Href:        link.Href,
			Badges: []formschema.OntologyPageBadge{
				{Label: lt(lang, relationKindLabel(link.Kind)), Tone: "slate"},
			},
		})
	}
	return cards
}

func relationKindLabel(kind string) string {
	switch kind {
	case "class", "classes":
		return "Class"
	case "property", "properties":
		return "Property"
	default:
		return "Relation"
	}
}

func propertyFlags(property *domain.OntologyProperty) []string {
	if property == nil {
		return nil
	}
	flags := []string{}
	if property.IsFunctional {
		flags = append(flags, "Functional")
	}
	if property.IsInverseFunctional {
		flags = append(flags, "Inverse functional")
	}
	if property.IsTransitive {
		flags = append(flags, "Transitive")
	}
	if property.IsSymmetric {
		flags = append(flags, "Symmetric")
	}
	if property.IsAsymmetric {
		flags = append(flags, "Asymmetric")
	}
	if property.IsReflexive {
		flags = append(flags, "Reflexive")
	}
	if property.IsIrreflexive {
		flags = append(flags, "Irreflexive")
	}
	return flags
}

func projectUsageRole(usage VersionProjectUsageModel) string {
	if usage.IsPrimary {
		return "Primary ontology version"
	}
	return "Additional ontology version"
}

func projectAdoptionDescription(version VersionPageCardModel, shown int) string {
	if version.ProjectCount <= int64(shown) {
		return versionUsageImpact(version)
	}
	return fmt.Sprintf("%s Showing %d linked projects.", versionUsageImpact(version), shown)
}

func lifecycleState(version VersionPageCardModel) string {
	if version.LifecycleState != "" {
		return version.LifecycleState
	}
	if version.IsActive {
		return "Active"
	}
	return "Inactive"
}

func lifecycleTone(version VersionPageCardModel) string {
	if version.LifecycleTone != "" {
		return version.LifecycleTone
	}
	if version.IsActive {
		return "green"
	}
	return "slate"
}

func versionUsageSummary(version VersionPageCardModel) string {
	if version.UsageSummary != "" {
		return version.UsageSummary
	}
	return usageSummary(version.ProjectCount)
}

func versionCompatibilitySummary(version VersionPageCardModel) string {
	if version.CompatibilitySummary != "" {
		return version.CompatibilitySummary
	}
	return compatibilitySummary(version.CompatibleBaseVersions)
}

func versionImportSummary(version VersionPageCardModel) string {
	if version.ImportSummary != "" {
		return version.ImportSummary
	}
	return importSummary(version.OriginalFilename, version.ParsedAt)
}

func activeVersionImpact(version VersionPageCardModel, currentActive string) string {
	if version.IsActive {
		return "Already the active version for new project selections"
	}
	if currentActive != "" {
		return fmt.Sprintf("Setting this active replaces %s as the default for new project selections. Existing project links are not migrated.", currentActive)
	}
	return "Setting this active changes the default version for new project selections"
}

func setActiveConfirm(version VersionPageCardModel, currentActive string) string {
	if currentActive != "" {
		return fmt.Sprintf("Set %s as the active version? New project selections will default to this version instead of %s. Existing project links are not migrated.", version.VersionString, currentActive)
	}
	return fmt.Sprintf("Set %s as the active version? New project selections will default to this version. Existing project links are not migrated.", version.VersionString)
}

func versionUsageImpact(version VersionPageCardModel) string {
	if version.ProjectCount == 1 {
		return "1 project currently links to this version. Deleting is blocked while that link exists."
	}
	if version.ProjectCount > 1 {
		return fmt.Sprintf("%d projects currently link to this version. Deleting is blocked while those links exist.", version.ProjectCount)
	}
	return "No projects currently link to this version. Delete guards can allow removal when other references are clear."
}

func usageImpactTone(version VersionPageCardModel) string {
	if version.ProjectCount > 0 {
		return "amber"
	}
	return "green"
}

func versionCanDelete(version VersionPageCardModel) bool {
	return !version.IsActive && version.ProjectCount == 0
}

func ontologyModelID(model *OntologyDetailPageModel) string {
	if model == nil || model.Ontology == nil {
		return ""
	}
	return model.Ontology.ID
}

func deleteVersionAction(version VersionPageCardModel, lang string) formschema.OntologyPageAction {
	successHref := version.AdminOntologyURL
	if successHref == "" {
		successHref = "/admin#ontologies"
	}
	return formschema.OntologyPageAction{
		ID:          "delete",
		Label:       lt(lang, "Delete version"),
		Method:      "DELETE",
		URL:         "/admin/ontologies/versions/" + version.ID,
		SuccessHref: successHref,
		Confirm:     lt(lang, "Delete this ontology version? This cannot be undone."),
		Style:       "danger",
	}
}

func schemaUI(lang string, languages []formschema.LanguageInfo) formschema.SchemaUI {
	return formschema.SchemaUI{Languages: languages, PrimaryLang: lang}
}

func lt(lang, value string) domain.Localizable {
	if value == "" {
		return nil
	}
	if lang == "" {
		lang = "en"
	}
	return domain.Translations{lang: value, "en": value}
}

func linksToSchema(lang string, links []OntologyPageLinkModel) []formschema.OntologyPageLink {
	out := make([]formschema.OntologyPageLink, 0, len(links))
	for _, link := range links {
		out = append(out, formschema.OntologyPageLink{Label: lt(lang, link.Label), Href: link.Href})
	}
	return out
}

func prependOntologyBreadcrumb(lang string, chain []OntologyPageLinkModel, current string) []formschema.OntologyPageLink {
	links := []OntologyPageLinkModel{{Label: "Ontologies", Href: "/ontologies"}}
	links = append(links, chain...)
	links = append(links, OntologyPageLinkModel{Label: current})
	return linksToSchema(lang, links)
}

func externalLinks(lang string, in map[string]string) []formschema.OntologyPageLink {
	out := []formschema.OntologyPageLink{}
	for _, label := range []string{"Homepage", "Source"} {
		if href := in[label]; href != "" {
			out = append(out, formschema.OntologyPageLink{Label: lt(lang, label), Href: href})
		}
	}
	return out
}

func externalActions(lang string, in map[string]string) []formschema.OntologyPageAction {
	out := []formschema.OntologyPageAction{}
	for _, label := range []string{"Homepage", "Source"} {
		if href := in[label]; href != "" {
			out = append(out, formschema.OntologyPageAction{ID: stringsID(label), Label: lt(lang, label), Href: href, Style: "secondary"})
		}
	}
	return out
}

func previewPageCards(cards []formschema.OntologyPageCard, limit int) []formschema.OntologyPageCard {
	if len(cards) <= limit {
		return cards
	}
	return cards[:limit]
}

func formatBytes(n int64) string {
	if n <= 0 {
		return ""
	}
	return fmt.Sprintf("%d bytes", n)
}

func stringsJoin(values []string) string {
	if len(values) == 0 {
		return ""
	}
	out := ""
	for i, value := range values {
		if i > 0 {
			out += ", "
		}
		out += value
	}
	return out
}

func stringsID(value string) string {
	out := ""
	for _, r := range value {
		if r >= 'A' && r <= 'Z' {
			r += 'a' - 'A'
		}
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			out += string(r)
			continue
		}
		if out != "" && out[len(out)-1] != '-' {
			out += "-"
		}
	}
	if out == "" {
		return "action"
	}
	return out
}
