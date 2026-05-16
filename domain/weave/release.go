package weave

import (
	"context"
	"errors"
	"time"
)

// ErrReleaseNotFound is returned when a project release does not exist.
var ErrReleaseNotFound = errors.New("release not found")

// Release is a frozen project version marker.
type Release struct {
	ProjectID   string    `json:"project_id"`
	Version     string    `json:"version"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	CreatedByID string    `json:"created_by_id"`
}

// ReleaseStore reads release metadata for project version selection.
type ReleaseStore interface {
	ListByProject(ctx context.Context, projectID string) ([]Release, error)
	Get(ctx context.Context, projectID, version string) (*Release, error)
}
