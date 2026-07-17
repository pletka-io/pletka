package formschema

import "github.com/pletka-io/pletka/pkg/domain"

// OntologyPageSchema is the page-level contract for ontology browse and
// management pages. It is intentionally narrower than ProjectPageSchema:
// ontology pages are read-model views with a small set of reusable widgets,
// not a general workspace shell.
type OntologyPageSchema struct {
	Kind        string                `json:"kind"`
	Title       domain.Localizable    `json:"title"`
	Subtitle    domain.Localizable    `json:"subtitle,omitempty"`
	Breadcrumbs []OntologyPageLink    `json:"breadcrumbs,omitempty"`
	Sections    []OntologyPageSection `json:"sections"`
	Actions     []OntologyPageAction  `json:"actions,omitempty"`
	Import      *OntologyImportConfig `json:"import,omitempty"`
	UI          SchemaUI              `json:"ui"`
}

type OntologyImportConfig struct {
	ProbeURL  string `json:"probe_url"`
	CommitURL string `json:"commit_url"`
	MaxBytes  int64  `json:"max_bytes,omitempty"`
}

type OntologyPageSection struct {
	ID          string                 `json:"id"`
	Widget      string                 `json:"widget"`
	Title       domain.Localizable     `json:"title,omitempty"`
	Description domain.Localizable     `json:"description,omitempty"`
	Tone        string                 `json:"tone,omitempty"`
	Stats       []OntologyPageStat     `json:"stats,omitempty"`
	Metadata    []OntologyPageMetadata `json:"metadata,omitempty"`
	Cards       []OntologyPageCard     `json:"cards,omitempty"`
	Tabs        []OntologyPageTab      `json:"tabs,omitempty"`
	Links       []OntologyPageLink     `json:"links,omitempty"`
	SchemaURL   string                 `json:"schema_url,omitempty"`
	EmptyText   domain.Localizable     `json:"empty_text,omitempty"`
}

type OntologyPageStat struct {
	Label domain.Localizable `json:"label"`
	Value string             `json:"value"`
	Help  domain.Localizable `json:"help,omitempty"`
	Tone  string             `json:"tone,omitempty"`
}

type OntologyPageMetadata struct {
	Label domain.Localizable `json:"label"`
	Value string             `json:"value,omitempty"`
	Href  string             `json:"href,omitempty"`
	Code  bool               `json:"code,omitempty"`
}

type OntologyPageCard struct {
	ID          string                 `json:"id,omitempty"`
	Title       domain.Localizable     `json:"title"`
	Subtitle    domain.Localizable     `json:"subtitle,omitempty"`
	Description domain.Localizable     `json:"description,omitempty"`
	Href        string                 `json:"href,omitempty"`
	Badges      []OntologyPageBadge    `json:"badges,omitempty"`
	Stats       []OntologyPageStat     `json:"stats,omitempty"`
	Metadata    []OntologyPageMetadata `json:"metadata,omitempty"`
	Actions     []OntologyPageAction   `json:"actions,omitempty"`
}

type OntologyPageBadge struct {
	Label domain.Localizable `json:"label"`
	Tone  string             `json:"tone,omitempty"`
}

type OntologyPageTab struct {
	ID     string             `json:"id"`
	Label  domain.Localizable `json:"label"`
	Href   string             `json:"href"`
	Active bool               `json:"active,omitempty"`
	Count  int                `json:"count,omitempty"`
}

type OntologyPageLink struct {
	Label domain.Localizable `json:"label"`
	Href  string             `json:"href,omitempty"`
	Icon  string             `json:"icon,omitempty"`
}

type OntologyPageAction struct {
	ID            string             `json:"id"`
	Label         domain.Localizable `json:"label"`
	Href          string             `json:"href,omitempty"`
	Method        string             `json:"method,omitempty"`
	URL           string             `json:"url,omitempty"`
	FormSchemaURL string             `json:"form_schema_url,omitempty"`
	SuccessHref   string             `json:"success_href,omitempty"`
	Confirm       domain.Localizable `json:"confirm,omitempty"`
	Style         string             `json:"style,omitempty"`
	Disabled      bool               `json:"disabled,omitempty"`
}
