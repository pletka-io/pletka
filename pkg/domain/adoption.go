package domain

import (
	"context"
	"time"
)

// Adoption is an explicit project-local reuse receipt for an upstream entity.
// In the current storage model it records intent/provenance without yet
// materializing a second local entity row that reuses the same semantic ID.
type Adoption struct {
	ID                int64     `json:"id,omitempty"`
	ProjectID         string    `json:"project_id"`
	ContextEntityType string    `json:"context_entity_type"`
	ContextEntityID   string    `json:"context_entity_id"`
	EntityType        string    `json:"entity_type"`
	SourceProjectID   string    `json:"source_project_id"`
	SourceEntityID    string    `json:"source_entity_id"`
	SourceVersion     string    `json:"source_version,omitempty"`
	AdoptedAt         time.Time `json:"adopted_at,omitempty"`
	CreatedByID       *string   `json:"created_by_id,omitempty"`
	Origin            Origin    `json:"origin,omitempty"`
	VersionNumber     string    `json:"version_number,omitempty"`
}

// AdoptionStore accesses explicit reuse receipts.
type AdoptionStore interface {
	List(ctx context.Context, opts ...QueryOption) ([]Adoption, error)
	ReplaceForContext(ctx context.Context, projectID, contextEntityType, contextEntityID string, adoptions []Adoption) error
}
