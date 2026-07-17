package formschema

import (
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/i18n"
)

// ProjectOverviewSchema is the schema for the overview tab content.
type ProjectOverviewSchema struct {
	Sections []ProjectOverviewSection `json:"sections"`
	UI       SchemaUI                 `json:"ui"`
}

// ProjectOverviewSection is one widget-annotated block in the overview tab.
// Widget types: "readme", "description", "stats-cards", "info-sidebar", "ontologies".
type ProjectOverviewSection struct {
	Widget      string                `json:"widget"`
	Title       domain.Localizable    `json:"title,omitempty"`
	Content     string                `json:"content,omitempty"`      // for "readme" (plain string)
	ContentI18n domain.Translations   `json:"content_i18n,omitempty"` // for "description" (user-data multilingual)
	Items       []ProjectOverviewItem `json:"items,omitempty"`        // for "stats-cards", "info-sidebar", "ontologies"
	Credits     []CreditsGroup        `json:"credits,omitempty"`      // for "credits"
}

// ProjectOverviewItem is one item in a stats-cards or info-sidebar section.
type ProjectOverviewItem struct {
	Key   string             `json:"key,omitempty"`
	Label domain.Localizable `json:"label"`
	Value string             `json:"value,omitempty"` // for info-sidebar
	Count int                `json:"count,omitempty"` // for stats-cards
	Icon  string             `json:"icon,omitempty"`
	Color string             `json:"color,omitempty"` // for stats-cards
	Style string             `json:"style,omitempty"` // "mono" for monospace
	Type  string             `json:"type,omitempty"`  // "date" triggers date formatting

	// Subline renders below the main label/value line. Used by the
	// "ontologies" widget to surface the ontology URI underneath the
	// friendly name + version.
	Subline string `json:"subline,omitempty"`

	// Per-kind usage counters for the "ontologies" widget. Renders as
	// "Classes 4/20 | Properties 10/50" — used count out of total. Set
	// only on ontology rows; other widgets ignore them.
	ClassesUsed     int `json:"classes_used,omitempty"`
	ClassesTotal    int `json:"classes_total,omitempty"`
	PropertiesUsed  int `json:"properties_used,omitempty"`
	PropertiesTotal int `json:"properties_total,omitempty"`

	// URL turns the item's value into an anchor. Used by the
	// "info-sidebar" widget to render quick-link items that jump
	// elsewhere in the app (e.g. profile-page sidebar links into
	// the matching tab). Empty = plain text.
	URL string `json:"url,omitempty"`

	Origin domain.Origin `json:"origin"`
}

// OntologyInfo holds display data for a project's linked ontology version.
type OntologyInfo struct {
	Name           string        `json:"name"`
	Version        string        `json:"version"`
	URI            string        `json:"uri"`
	ClassCount     int           `json:"class_count"`
	PropertyCount  int           `json:"property_count"`
	ClassesUsed    int           `json:"classes_used"`
	PropertiesUsed int           `json:"properties_used"`
	Origin         domain.Origin `json:"origin"`
}

// CreditsGroup is one kind-bucket (Authors / Funders / Adopters) on
// the project overview "credits" section.
type CreditsGroup struct {
	Kind  string             `json:"kind"`
	Label domain.Localizable `json:"label"`
	Items []CreditsItem      `json:"items"`
}

// CreditsItem is a single attribution row rendered in the credits
// section. Note is the freeform "what they did" qualifier.
type CreditsItem struct {
	ActorID   string `json:"actor_id"`
	ActorName string `json:"actor_name"`
	ActorType string `json:"actor_type,omitempty"`
	Note      string `json:"note,omitempty"`
}

// buildCreditsGroups groups attributions by kind in the canonical
// authors → funders → adopters order. Items within each kind keep
// their position-ordered sequence from the reader.
func buildCreditsGroups(attributions []domain.Attribution) []CreditsGroup {
	if len(attributions) == 0 {
		return nil
	}
	buckets := map[string][]CreditsItem{}
	for _, a := range attributions {
		buckets[a.Kind] = append(buckets[a.Kind], CreditsItem{
			ActorID:   a.ActorID,
			ActorName: a.ActorName,
			ActorType: a.ActorType,
			Note:      a.Note,
		})
	}
	defs := []struct {
		Kind  string
		Label domain.Localizable
	}{
		{"author", i18n.L("project_overview.authors", "Authors")},
		{"funder", i18n.L("project_overview.funders", "Funders")},
		{"adopter", i18n.L("project_overview.adopters", "Adopters")},
	}
	out := make([]CreditsGroup, 0, len(defs))
	for _, d := range defs {
		items := buckets[d.Kind]
		if len(items) == 0 {
			continue
		}
		out = append(out, CreditsGroup{Kind: d.Kind, Label: d.Label, Items: items})
	}
	return out
}

// BuildProjectOverviewSchema creates the overview tab content schema.
func BuildProjectOverviewSchema(
	project *domain.Project,
	modelCount, collectionCount, fieldCount, categoryCount int,
	ontologies []OntologyInfo,
	attributions []domain.Attribution,
	lang string,
	languages []LanguageInfo,
) *ProjectOverviewSchema {
	var sections []ProjectOverviewSection

	// Description section — only shown if the project has a non-empty description.
	if project.Description.Get("en", "") != "" {
		sections = append(sections, ProjectOverviewSection{
			Widget:      "description",
			ContentI18n: project.Description,
		})
	}

	sections = append(sections, ProjectOverviewSection{
		Widget: "stats-cards",
		Title:  i18n.L("common.statistics", "Statistics"),
		Items: []ProjectOverviewItem{
			{
				Label: i18n.L("common.models", "Models"),
				Count: modelCount,
				Icon:  "cube",
				Color: "purple",
			},
			{
				Label: i18n.L("common.collections", "Collections"),
				Count: collectionCount,
				Icon:  "collection",
				Color: "green",
			},
			{
				Label: i18n.L("common.fields", "Fields"),
				Count: fieldCount,
				Icon:  "list",
				Color: "blue",
			},
			{
				Label: i18n.L("common.categories", "Categories"),
				Count: categoryCount,
				Icon:  "tag",
				Color: "amber",
			},
		},
	})

	// Ontologies section — show linked ontology versions with per-kind
	// usage. Title is "Deployed Ontologies" because curators
	// read these as ontologies the project has actively deployed, not as
	// stand-alone references. Each row carries a subline with the
	// ontology URI plus separate classes/properties usage counters.
	if len(ontologies) > 0 {
		var ontoItems []ProjectOverviewItem
		for _, o := range ontologies {
			subline := o.URI
			if o.Version != "" {
				if subline == "" {
					subline = "v" + o.Version
				} else {
					subline = "v" + o.Version + " · " + o.URI
				}
			}
			ontoItems = append(ontoItems, ProjectOverviewItem{
				Label:           domain.Translations{"en": o.Name},
				Value:           o.Version,
				Subline:         subline,
				ClassesUsed:     o.ClassesUsed,
				ClassesTotal:    o.ClassCount,
				PropertiesUsed:  o.PropertiesUsed,
				PropertiesTotal: o.PropertyCount,
				Icon:            "cube",
				Color:           "indigo",
				Origin:          o.Origin,
			})
		}
		sections = append(sections, ProjectOverviewSection{
			Widget: "ontologies",
			Title:  i18n.L("project_overview.deployed_ontologies", "Deployed Ontologies"),
			Items:  ontoItems,
		})
	}

	// Credits section — Authors / Funders / Adopters. Kept separate from
	// authorisation (memberships) and institutional ownership
	// (weave_project_actors).
	if creditsGroups := buildCreditsGroups(attributions); len(creditsGroups) > 0 {
		sections = append(sections, ProjectOverviewSection{
			Widget:  "credits",
			Title:   i18n.L("project_overview.credits", "Credits"),
			Credits: creditsGroups,
		})
	}

	// Info sidebar — show what's available on domain.Project.
	infoItems := []ProjectOverviewItem{
		{
			Key:   "id",
			Label: i18n.L("project_overview.project_id", "Project ID"),
			Value: project.ID,
			Style: "mono",
		},
	}

	if project.Namespace != "" {
		infoItems = append(infoItems, ProjectOverviewItem{
			Key:   "namespace",
			Label: i18n.L("project_overview.namespace", "Namespace"),
			Value: project.Namespace,
			Style: "mono",
		})
	}

	if project.ParentProjectID != nil && *project.ParentProjectID != "" {
		infoItems = append(infoItems, ProjectOverviewItem{
			Key:   "parent",
			Label: i18n.L("project_overview.parent_project", "Parent Project"),
			Value: *project.ParentProjectID,
		})
	}

	sections = append(sections, ProjectOverviewSection{
		Widget: "info-sidebar",
		Title:  i18n.L("project_overview.project_details", "Project Details"),
		Items:  infoItems,
	})

	return &ProjectOverviewSchema{
		Sections: sections,
		UI: SchemaUI{
			Languages:   languages,
			PrimaryLang: lang,
		},
	}
}
