package weave

import (
	"context"
	"time"
)

// DependencySourceMode declares whether an inherited parent is read from draft
// state or a frozen release version.
type DependencySourceMode string

const (
	DependencySourceDraft   DependencySourceMode = "draft"
	DependencySourceRelease DependencySourceMode = "release"
)

// ProjectInheritance is one project-to-parent link in the inheritance graph.
type ProjectInheritance struct {
	ProjectID       string               `json:"project_id"`
	ParentProjectID string               `json:"parent_project_id"`
	IsPrimary       bool                 `json:"is_primary"`
	CanonicalOrder  int                  `json:"canonical_order"`
	AdoptedAt       time.Time            `json:"adopted_at"`
	SourceMode      DependencySourceMode `json:"source_mode"`
	SourceVersion   string               `json:"source_version,omitempty"`
}

// ProjectInheritanceStore accesses the project inheritance graph.
type ProjectInheritanceStore interface {
	List(ctx context.Context, projectID string) ([]ProjectInheritance, error)
	ListVersion(ctx context.Context, projectID, version string) ([]ProjectInheritance, error)
	ListByParent(ctx context.Context, parentID string) ([]ProjectInheritance, error)
	Add(ctx context.Context, link ProjectInheritance) error
	Remove(ctx context.Context, projectID, parentID string) error
	SetPrimary(ctx context.Context, projectID, parentID string) error
	Reorder(ctx context.Context, projectID string, parentIDsInOrder []string) error
}
