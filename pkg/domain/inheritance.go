package domain

import (
	"context"
	"time"
)

type DependencySourceMode string

const (
	DependencySourceDraft   DependencySourceMode = "draft"
	DependencySourceRelease DependencySourceMode = "release"
)

// ProjectInheritance is one project-to-parent link in the inheritance graph.
// A project has at most one primary parent today; additional parents are
// ordered canonically for deterministic traversal.
type ProjectInheritance struct {
	ProjectID       string
	ParentProjectID string
	IsPrimary       bool
	CanonicalOrder  int
	AdoptedAt       time.Time
	SourceMode      DependencySourceMode
	SourceVersion   string
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
