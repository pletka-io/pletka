package weave

import (
	"context"

	"github.com/pletka-io/pletka/domain"
)

// Project represents a workspace that owns semantic patterns.
type Project struct {
	domain.Entity

	Namespace       string              `json:"namespace,omitempty"`
	ParentProjectID *string             `json:"parent_project_id,omitempty"`
	StagingID       *int64              `json:"staging_id,omitempty"`
	OwnerID         string              `json:"owner_id,omitempty"`
	CreatedByID     *string             `json:"created_by_id,omitempty"`
	Visibility      string              `json:"visibility,omitempty"`
	License         string              `json:"license,omitempty"`
	README          domain.Translations `json:"readme,omitempty"`
	Topics          []string            `json:"topics,omitempty"`
	BaseURL         string              `json:"base_url,omitempty"`
	IsMasterWeave   bool                `json:"is_master_weave,omitempty"`
}

// ProjectStore reads project metadata for materialization and routing.
type ProjectStore interface {
	GetByID(ctx context.Context, id string) (*Project, error)
	GetByIDVersion(ctx context.Context, id, version string) (*Project, error)
}
