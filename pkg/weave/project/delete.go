package project

import (
	"context"
	"fmt"
	"strings"
)

// DeleteBlockers counts everything outside a project that depends on it.
// A project with any non-zero blocker cannot be deleted — deprecate or
// untangle the dependents first. This mirrors the entity-level delete
// contract (409 in_use), scaled to whole projects: children read the
// parent's live or release-pinned entities, adoption receipts and
// cross-project placements/value-refs point into its entity set.
type DeleteBlockers struct {
	Children           int // weave_project_inheritance rows naming it as parent (draft or release-pinned)
	AdoptionsElsewhere int // other projects' adoption receipts sourced from it
	ExternalPlacements int // its fields placed in other projects' containers
	ExternalValueRefs  int // other projects' override refs targeting its entities
}

// Blocked reports whether any dependency prevents deletion.
func (b DeleteBlockers) Blocked() bool {
	return b.Children > 0 || b.AdoptionsElsewhere > 0 || b.ExternalPlacements > 0 || b.ExternalValueRefs > 0
}

// String renders the non-zero blockers for error messages and CLI output.
func (b DeleteBlockers) String() string {
	var parts []string
	if b.Children > 0 {
		parts = append(parts, fmt.Sprintf("%d child project link(s)", b.Children))
	}
	if b.AdoptionsElsewhere > 0 {
		parts = append(parts, fmt.Sprintf("%d adoption receipt(s) in other projects", b.AdoptionsElsewhere))
	}
	if b.ExternalPlacements > 0 {
		parts = append(parts, fmt.Sprintf("%d placement(s) of its fields in other projects", b.ExternalPlacements))
	}
	if b.ExternalValueRefs > 0 {
		parts = append(parts, fmt.Sprintf("%d value ref(s) from other projects", b.ExternalValueRefs))
	}
	return strings.Join(parts, ", ")
}

// DeleteStats reports what a cascade delete removed, per area.
type DeleteStats struct {
	Fields         int64
	Models         int64
	Collections    int64
	Categories     int64
	Overrides      int64
	OtherRows      int64 // bindings, memberships, staging, counters, refs, change log/sets, archives, …
	ArchiveRows    int64
	ChangeLogRows  int64
	ProjectDeleted bool
}

// ErrDeleteBlocked is returned by Service.Delete when external dependents
// exist. Callers surface the blocker detail and suggest deprecation.
type ErrDeleteBlocked struct {
	ProjectID string
	Blockers  DeleteBlockers
}

func (e *ErrDeleteBlocked) Error() string {
	return fmt.Sprintf("project %s cannot be deleted: %s — deprecate it or untangle the dependents first", e.ProjectID, e.Blockers)
}

// DeleteProject removes a project and every row it owns, after verifying
// nothing outside the project depends on it. The cascade runs in one
// transaction; the materialized git work tree (if any) is the caller's
// concern (the CLI removes it best-effort).
func (s *Service) DeleteProject(ctx context.Context, projectID string) (DeleteStats, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return DeleteStats{}, fmt.Errorf("delete project: id required")
	}
	existing, err := s.store.GetByID(ctx, projectID)
	if err != nil {
		return DeleteStats{}, fmt.Errorf("delete project %s: %w", projectID, err)
	}
	if existing == nil {
		return DeleteStats{}, fmt.Errorf("delete project %s: not found", projectID)
	}
	blockers, err := s.store.DeleteBlockers(ctx, projectID)
	if err != nil {
		return DeleteStats{}, fmt.Errorf("delete project %s: check dependents: %w", projectID, err)
	}
	if blockers.Blocked() {
		return DeleteStats{}, &ErrDeleteBlocked{ProjectID: projectID, Blockers: blockers}
	}
	stats, err := s.store.DeleteCascade(ctx, projectID)
	if err != nil {
		return stats, fmt.Errorf("delete project %s: %w", projectID, err)
	}
	s.log.Info("project deleted", "project_id", projectID,
		"fields", stats.Fields, "models", stats.Models, "collections", stats.Collections,
		"categories", stats.Categories, "overrides", stats.Overrides)
	return stats, nil
}
