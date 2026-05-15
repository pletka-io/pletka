package weave

import (
	"context"
	"time"

	"github.com/pletka-io/pletka/domain"
)

// Adoption is an explicit project-local reuse receipt for an upstream entity.
type Adoption struct {
	ID                int64         `json:"id,omitempty"`
	ProjectID         string        `json:"project_id"`
	ContextEntityType string        `json:"context_entity_type"`
	ContextEntityID   string        `json:"context_entity_id"`
	EntityType        string        `json:"entity_type"`
	SourceProjectID   string        `json:"source_project_id"`
	SourceEntityID    string        `json:"source_entity_id"`
	SourceVersion     string        `json:"source_version,omitempty"`
	AdoptedAt         time.Time     `json:"adopted_at,omitempty"`
	CreatedByID       *string       `json:"created_by_id,omitempty"`
	Origin            domain.Origin `json:"origin,omitempty"`
	VersionNumber     string        `json:"version_number,omitempty"`
}

// AdoptionStore accesses explicit reuse receipts.
type AdoptionStore interface {
	List(ctx context.Context, opts ...domain.QueryOption) ([]Adoption, error)
	ReplaceForContext(ctx context.Context, projectID, contextEntityType, contextEntityID string, adoptions []Adoption) error
}

// EntityFork records local divergence from an adopted upstream entity.
type EntityFork struct {
	ID              int64     `json:"id,omitempty"`
	ProjectID       string    `json:"project_id"`
	EntityType      string    `json:"entity_type"`
	ForkEntityID    string    `json:"fork_entity_id"`
	SourceProjectID string    `json:"source_project_id"`
	SourceEntityID  string    `json:"source_entity_id"`
	SourceVersion   string    `json:"source_version,omitempty"`
	ForkedAt        time.Time `json:"forked_at,omitempty"`
	CreatedByID     *string   `json:"created_by_id,omitempty"`
	VersionNumber   string    `json:"version_number,omitempty"`
}

// ForkStore accesses entity fork provenance records.
type ForkStore interface {
	List(ctx context.Context, opts ...domain.QueryOption) ([]EntityFork, error)
	Create(ctx context.Context, fork *EntityFork) error
}
