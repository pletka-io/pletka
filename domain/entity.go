package domain

import (
	"fmt"
	"time"
)

// Status is the workflow stage of an entity.
type Status string

const (
	StatusDraft     Status = "draft"
	StatusPublished Status = "published"
)

// Valid reports whether s is one of the recognised workflow values.
func (s Status) Valid() bool {
	switch s {
	case StatusDraft, StatusPublished:
		return true
	default:
		return false
	}
}

// Entity contains common identity and metadata fields shared by weave entities.
type Entity struct {
	ID            string       `json:"id"`
	CreatedAt     time.Time    `json:"created_at"`
	UpdatedAt     time.Time    `json:"updated_at"`
	SemanticID    string       `json:"semantic_id,omitempty"`
	SystemName    string       `json:"system_name,omitempty"`
	UIName        Translations `json:"ui_name,omitempty"`
	Description   Translations `json:"description,omitempty"`
	Status        Status       `json:"status"`
	VersionNumber string       `json:"version_number,omitempty"`
	ProjectID     string       `json:"project_id,omitempty"`
	Deprecated    bool         `json:"deprecated"`
}

// URI returns the resolvable namespace URI for this entity.
func (e *Entity) URI() string {
	if e.ProjectID == "" || e.SemanticID == "" {
		return ""
	}
	return fmt.Sprintf("/ns/%s/%s", e.ProjectID, e.SemanticID)
}
