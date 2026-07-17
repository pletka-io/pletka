package formschema

import (
	"strings"

	"github.com/pletka-io/pletka/pkg/domain"
)

// EntityListSchema is the top-level response from the entity list schema API.
// It describes how to render a browse-oriented entity list with search, filters,
// sorting, and rich row layout.
type EntityListSchema struct {
	EntityType        string             `json:"entity_type"`
	Title             domain.Localizable `json:"title"`
	EmptyState        *EmptyState        `json:"empty_state,omitempty"`
	DataURL           string             `json:"data_url"`
	DataKey           string             `json:"data_key"`
	DetailURLTemplate string             `json:"detail_url_template,omitempty"`
	EditURLTemplate   string             `json:"edit_url_template,omitempty"`
	ProjectID         string             `json:"project_id"`

	// Toolbar
	Search      *SearchConfig  `json:"search,omitempty"`
	Filters     []FilterConfig `json:"filters,omitempty"`
	SortOptions []SortOption   `json:"sort_options"`
	DefaultSort string         `json:"default_sort"`

	// Display
	RowWidget    string              `json:"row_widget,omitempty"` // name looked up in frontend registry; empty = "default"
	ViewModes    []ViewMode          `json:"view_modes,omitempty"` // modes supported by the chosen widget
	RowLayout    EntityRowLayout     `json:"row_layout"`
	ColumnHeader *ColumnHeaderConfig `json:"column_header,omitempty"` // rendered once above the rows when present
	Grouping     *GroupConfig        `json:"grouping,omitempty"`
	Pagination   *PaginationConfig   `json:"pagination,omitempty"`
	Editor       *EntityListEditor   `json:"editor,omitempty"`

	// Actions
	Capabilities *Capabilities `json:"capabilities,omitempty"`
	RowActions   []RowAction   `json:"row_actions,omitempty"`

	UI SchemaUI `json:"ui"`
}

// StripEditActions removes mutating affordances while preserving safe read-only
// capabilities such as stats and detail navigation.
func (s *EntityListSchema) StripEditActions() {
	s.EditURLTemplate = ""
	if s.Capabilities != nil {
		s.Capabilities.Reorder = nil
		s.Capabilities.InlineRename = nil
		s.Capabilities.Create = nil
		s.Capabilities.Adopt = nil
		s.Capabilities.Edit = nil
		s.Capabilities.Delete = nil
		if s.Capabilities.Stats == nil {
			s.Capabilities = nil
		}
	}
	if len(s.RowActions) == 0 {
		return
	}
	filtered := make([]RowAction, 0, len(s.RowActions))
	for _, action := range s.RowActions {
		method := strings.ToUpper(strings.TrimSpace(action.Method))
		if action.ID == "stats" || method == httpMethodGet {
			filtered = append(filtered, action)
		}
	}
	s.RowActions = filtered
}

const httpMethodGet = "GET"

// SearchConfig describes the search input for the entity list toolbar.
type SearchConfig struct {
	Placeholder domain.Localizable `json:"placeholder"`
	ParamName   string             `json:"param_name"`
}

// FilterConfig describes a filter in the toolbar or filter drawer.
// Options may be embedded inline (for small, cheap lists) or fetched lazily
// via OptionsURL when the drawer opens (for large lists).
type FilterConfig struct {
	Key         string             `json:"key"`
	Label       domain.Localizable `json:"label"`
	ParamName   string             `json:"param_name"`
	Type        string             `json:"type"` // "select"
	Options     []FilterOption     `json:"options,omitempty"`
	OptionsURL  string             `json:"options_url,omitempty"`  // lazy fetch; GET returns {options: [...]}
	OptionsType string             `json:"options_type,omitempty"` // "checklist" | "typeahead"; default "checklist"
	Multi       bool               `json:"multi,omitempty"`        // when true, selected values are comma-joined on the wire
}

// FilterOption represents a single option in a filter dropdown.
// Default marks this option as pre-selected when the URL carries no
// param for the parent filter — the frontend applies the union of
// every Default=true option as the initial filter value. Used by the
// Origin filter on the entity lists so the curator sees Owned +
// Adapted by default and opts in to Adopted rows.
type FilterOption struct {
	Value   string             `json:"value"`
	Label   domain.Localizable `json:"label"`
	Default bool               `json:"default,omitempty"`
}

// SortOption describes a sort option in the toolbar.
type SortOption struct {
	Value string             `json:"value"`
	Label domain.Localizable `json:"label"`
}

// EntityRowLayout describes the 3-line row layout generically.
// BylineField and CountColumns are optional inputs read by widgets that
// support them (e.g. the editorial widget on the projects list).
type EntityRowLayout struct {
	TitleField     string          `json:"title_field"`
	SubtitleField  string          `json:"subtitle_field"`
	BylineField    string          `json:"byline_field,omitempty"`
	IdentityFields []IdentityField `json:"identity_fields"`
	SemanticBadges []BadgeConfig   `json:"semantic_badges"`
	ProcessBadges  []BadgeConfig   `json:"process_badges"`
	CountColumns   []CountColumn   `json:"count_columns,omitempty"`
	// PathField names the row item's field holding the []PathElement ontology
	// path. When set, the default row renders it as a class/property pill chain
	// on its own line (detailed view mode only) — the field-specific rendering
	// that used to live in the separate FieldCard widget. Empty = no path row.
	PathField string `json:"path_field,omitempty"`
	// LeadBox renders a single field as a prominent box at the START
	// of the title line, before the entity name. Used for the ontology
	// scope class on model / collection / field rows so the class is
	// the first thing the eye lands on (as a second-line
	// badge it read like the field's value range). Nil = no lead box.
	LeadBox *LeadBoxConfig `json:"lead_box,omitempty"`
}

// LeadBoxConfig describes the leading box rendered before the title.
type LeadBoxConfig struct {
	Key   string `json:"key"`             // item field to display
	Style string `json:"style,omitempty"` // badge palette key, e.g. "purple"
}

// CountColumn declares one labelled numeric cell in a count-grid row layout.
// Widgets render the value at item[Key], with Label as a column header.
type CountColumn struct {
	Key             string             `json:"key"`
	Label           domain.Localizable `json:"label"`
	ZeroPlaceholder string             `json:"zero_placeholder,omitempty"` // e.g. "—"; default: render the literal 0
}

// ColumnHeaderConfig declares a header strip rendered once above the rows by
// EntityListView when the active widget benefits from one. Labels for the
// count columns are sourced from RowLayout.CountColumns; this struct controls
// whether to render and what to call the title/subtitle group on the left.
type ColumnHeaderConfig struct {
	Show       bool               `json:"show"`
	IDLabel    domain.Localizable `json:"id_label,omitempty"`
	TitleLabel domain.Localizable `json:"title_label,omitempty"` // label for the title/byline column
}

// IdentityField describes an identity element on the title line.
type IdentityField struct {
	Key   string `json:"key"`
	Style string `json:"style"` // "mono", "code"
}

// BadgeConfig describes a badge rendered based on item data.
type BadgeConfig struct {
	Key       string             `json:"key"`
	Type      string             `json:"type"`  // "text", "status", "ownership", "count"
	Style     string             `json:"style"` // "purple", "blue", "green", "gray", "amber"
	Label     domain.Localizable `json:"label,omitempty"`
	HideEmpty bool               `json:"hide_empty,omitempty"`
}

// GroupConfig describes grouping options for the entity list.
type GroupConfig struct {
	Options    []GroupOption `json:"options"`
	DefaultKey string        `json:"default_key"`
	ParamName  string        `json:"param_name"`
}

// GroupOption represents a single grouping option.
type GroupOption struct {
	Value string             `json:"value"`
	Label domain.Localizable `json:"label"`
}

// PaginationConfig describes pagination for the entity list.
type PaginationConfig struct {
	PageSize        int    `json:"page_size"`
	ParamName       string `json:"param_name"`
	PerPageName     string `json:"per_page_name"`
	PageSizeOptions []int  `json:"page_size_options,omitempty"`
}

// ViewMode is one display variant for a list. When Widget is set, selecting
// the mode swaps the active row widget; otherwise the mode is just a hint
// passed to the current widget (e.g. "compact" vs "detailed" within one widget).
type ViewMode struct {
	ID      string             `json:"id"` // "editorial", "compact", "detailed", etc.
	Label   domain.Localizable `json:"label"`
	Default bool               `json:"default,omitempty"`
	Widget  string             `json:"widget,omitempty"` // optional: overrides EntityListSchema.RowWidget when active
}

// EntityListEditor names a custom editor widget for create/edit flows that
// cannot be represented by a plain FormSchema.
type EntityListEditor struct {
	Widget string `json:"widget"`
	// Endpoints carries the API URLs (or {id}-style url templates) the
	// editor widget calls, so the frontend never constructs an API URL
	// itself (schema-driven API rule). Keys are widget-specific.
	Endpoints map[string]string `json:"endpoints,omitempty"`
}
