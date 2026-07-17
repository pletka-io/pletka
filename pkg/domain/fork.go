package domain

import (
	"context"
	"time"
)

// EntityFork records local divergence from an adopted upstream entity.
// The forked local entity gets a new project-based semantic ID while
// provenance points back to the adopted source.
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

type ForkStore interface {
	List(ctx context.Context, opts ...QueryOption) ([]EntityFork, error)
	Create(ctx context.Context, fork *EntityFork) error
}
