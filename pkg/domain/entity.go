package domain

import (
	"fmt"
	"time"
)

// Status is the workflow stage of an Entity. Two valid values: draft and
// published. The retirement axis is carried separately by Entity.Deprecated.
//
// type Status string accepts untyped string constants implicitly, so legacy
// callers that compare against "draft" or assign "published" keep compiling
// without changes. Only typed-string assignments need explicit conversion.
type Status string

const (
	// StatusDraft is the initial workflow stage for a newly created entity.
	StatusDraft Status = "draft"
	// StatusPublished is the active workflow stage. Entities at this stage
	// are visible in pickers (unless deprecated) and may be referenced by
	// overrides.
	StatusPublished Status = "published"
)

// Valid reports whether s is one of the two recognised workflow values.
func (s Status) Valid() bool {
	switch s {
	case StatusDraft, StatusPublished:
		return true
	}
	return false
}

// Entity contains the common identity and metadata fields shared by all
// domain types. Two orthogonal lifecycle axes:
//   - Status: workflow stage (draft → published)
//   - Deprecated: retirement (false = active, true = soft-retired; existing
//     references stay intact, pickers exclude, no new connections allowed)
//
// Hard delete is permitted only when an entity is unreferenced (and, by
// convention, deprecated first).
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
	ProjectID     string       `json:"project_id"`
	Deprecated    bool         `json:"deprecated"`
}

// URI returns the resolvable namespace URI for this entity.
// Pattern: /ns/{projectID}/{semanticID}
func (e *Entity) URI() string {
	if e.ProjectID == "" || e.SemanticID == "" {
		return ""
	}
	return fmt.Sprintf("/ns/%s/%s", e.ProjectID, e.SemanticID)
}
