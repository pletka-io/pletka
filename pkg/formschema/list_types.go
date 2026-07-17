package formschema

import "github.com/pletka-io/pletka/pkg/domain"

// ListSchema is the top-level response from the list schema API.
type ListSchema struct {
	EntityType string             `json:"entity_type"`
	Title      domain.Localizable `json:"title"`
	EmptyState *EmptyState        `json:"empty_state,omitempty"`
	DataURL    string             `json:"data_url"`
	DataKey    string             `json:"data_key"`
	Caps       Capabilities       `json:"capabilities"`
	Columns    []Column           `json:"columns"`
	RowActions []RowAction        `json:"row_actions"`
	UI         SchemaUI           `json:"ui"`
	// PerRowReadonlyField names a boolean field on each row. Rows whose value
	// for that field is truthy hide their inline row actions. Used by panes
	// where some rows are system-owned and non-mutable (namespace bindings).
	PerRowReadonlyField string `json:"per_row_readonly_field,omitempty"`
}

// EmptyState defines what to show when the list has no items.
type EmptyState struct {
	Icon    string             `json:"icon"`
	Title   domain.Localizable `json:"title"`
	Message domain.Localizable `json:"message"`
}

// Capabilities describes what operations the list supports.
type Capabilities struct {
	Reorder      *ReorderCap      `json:"reorder,omitempty"`
	InlineRename *InlineRenameCap `json:"inline_rename,omitempty"`
	Create       *CreateCap       `json:"create,omitempty"`
	Adopt        *AdoptCap        `json:"adopt,omitempty"`
	Edit         *EditCap         `json:"edit,omitempty"`
	Delete       *DeleteCap       `json:"delete,omitempty"`
	Stats        *StatsCap        `json:"stats,omitempty"`
}

// ReorderCap describes drag-and-drop reorder support.
type ReorderCap struct {
	Enabled    bool   `json:"enabled"`
	URL        string `json:"url"`
	OrderField string `json:"order_field"`
}

// InlineRenameCap describes inline rename support.
type InlineRenameCap struct {
	Enabled         bool   `json:"enabled"`
	Field           string `json:"field"`
	SaveURLTemplate string `json:"save_url_template"`
}

// CreateCap describes create support.
type CreateCap struct {
	Label         domain.Localizable `json:"label"`
	FormSchemaURL string             `json:"form_schema_url"`
}

// AdoptCap describes "adopt existing pattern from ancestor project"
// support. The frontend renders an Adopt button alongside Create and
// opens a side-panel picker that lists candidate entities from the
// project's ancestor chain.
type AdoptCap struct {
	// Label for the Adopt button.
	Label domain.Localizable `json:"label"`
	// OptionsURL lists adoptable candidates (ancestor entities not yet
	// local or adopted). Returns []{value,label,source_project_id}.
	OptionsURL string `json:"options_url"`
	// URL accepts a POST {source_project_id, source_entity_id} to
	// append an adoption row for the current project.
	URL string `json:"url"`
	// DestinationLabel is the "Adopting into {project}" line shown above
	// the search box on the picker — UX review item 4 of the
	// adopt/adapt rollout. Keeps the destination context visible so
	// curators don't lose track of where the receipt will land.
	DestinationLabel domain.Localizable `json:"destination_label,omitempty"`
}

// EditCap describes edit support.
type EditCap struct {
	FormSchemaURLTemplate string `json:"form_schema_url_template"`
}

// DeleteCap describes delete support.
type DeleteCap struct {
	URLTemplate  string       `json:"url_template"`
	Reassignment *ReassignCap `json:"reassignment,omitempty"`
}

// ReassignCap describes delete-with-reassignment support.
type ReassignCap struct {
	Enabled     bool               `json:"enabled"`
	EntityLabel domain.Localizable `json:"entity_label"`
	CountField  string             `json:"count_field"`
	OptionsFrom string             `json:"options_from"` // "siblings"
}

// StatsCap describes stats modal support.
type StatsCap struct {
	URLTemplate string `json:"url_template"`
}

// Column defines a column in the list.
type Column struct {
	Key        string             `json:"key"`
	Label      domain.Localizable `json:"label,omitempty"`
	Type       string             `json:"type"` // "translation", "badge", "text_badge", "computed_badge", "text"
	Primary    bool               `json:"primary,omitempty"`
	Secondary  bool               `json:"secondary,omitempty"`
	Truncate   bool               `json:"truncate,omitempty"`
	BadgeStyle string             `json:"badge_style,omitempty"` // "blue", "purple", "green", "gray"
	ZeroStyle  string             `json:"zero_style,omitempty"`
	HideZero   bool               `json:"hide_zero,omitempty"`
	Compute    string             `json:"compute,omitempty"` // e.g., "model_field_count + collection_field_count"
	// BadgeStyleByValue maps the column's string value to a badge style
	// (used by type="text_badge"). Falls back to BadgeStyle when the
	// value isn't in the map. Example: {"owner":"amber","viewer":"gray"}.
	BadgeStyleByValue map[string]string `json:"badge_style_by_value,omitempty"`
}

// RowAction defines an action button on each row.
type RowAction struct {
	ID    string             `json:"id"` // "edit", "stats", "delete", "deprecate", "activate", ...
	Icon  string             `json:"icon"`
	Label domain.Localizable `json:"label"`
	Style string             `json:"style,omitempty"` // "danger"

	// URLTemplate is an optional explicit URL for actions that don't have a
	// standard mapping baked into the frontend. Substitute {id} with the
	// row's ID. When empty, the frontend uses its built-in default for the
	// action ID (e.g., delete uses Caps.Delete.URLTemplate).
	URLTemplate string `json:"url_template,omitempty"`

	// Method is the HTTP verb to use when invoking URLTemplate. Defaults
	// vary by action ID — lifecycle actions are typically POST.
	Method string `json:"method,omitempty"`

	// VisibleWhen is a simple boolean expression evaluated against the row
	// payload to gate visibility. Supported syntax:
	//   "fieldname"     — truthy
	//   "!fieldname"    — falsy
	// Empty means always visible.
	VisibleWhen string `json:"visible_when,omitempty"`
}

// Group is one section of a grouped list data response. The list-schema
// endpoint does not emit Groups directly; instead, the data endpoint chooses
// between returning a flat `items: [...]` array (default) or a `groups: [...]`
// array (for nested / inherited / per-base listings).
type Group struct {
	ID          string             `json:"id"`
	Label       domain.Localizable `json:"label"`
	Subtitle    domain.Localizable `json:"subtitle,omitempty"`
	Badges      []Badge            `json:"badges,omitempty"`
	Collapsible bool               `json:"collapsible"`
	Collapsed   bool               `json:"collapsed"`
	ReadOnly    bool               `json:"read_only,omitempty"`
	Actions     []RowAction        `json:"actions,omitempty"`
	Items       []map[string]any   `json:"items"`
}

// Badge is a small coloured label rendered in a group header (e.g. "Primary",
// "Inherited from LA"). Tone drives the colour.
type Badge struct {
	Label string `json:"label"`
	Tone  string `json:"tone,omitempty"` // "primary" | "neutral" | "warning"
}
