package formschema

import (
	"github.com/pletka-io/pletka/pkg/domain"
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


func withVersionQuery(raw, activeVersion string) string {
	if activeVersion == "" {
		return raw
	}
	return addVersionToURL(raw, activeVersion)
}

