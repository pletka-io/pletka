// Package releasearchive checks the provenance of release-owned archive data.
package releasearchive

import (
	"context"
	"fmt"
	"strings"

	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
)

// UnavailableError identifies release data whose ownership cannot be verified.
type UnavailableError struct {
	ProjectID string
	Version   string
}

func (e *UnavailableError) Error() string {
	return fmt.Sprintf("release %s@%s has no verified archive ownership", e.ProjectID, e.Version)
}

// IsNotInRelease marks unavailable archive data for the canonical API envelope.
func (e *UnavailableError) IsNotInRelease() bool { return true }

// Require verifies ownership without inferring history from current data.
func Require(ctx context.Context, q *sqlcgen.Queries, projectID, version string) error {
	if strings.TrimSpace(projectID) == "" || strings.TrimSpace(version) == "" {
		return &UnavailableError{ProjectID: projectID, Version: version}
	}
	ok, err := q.WeaveReleaseHasArchiveOwnership(ctx, sqlcgen.WeaveReleaseHasArchiveOwnershipParams{ProjectID: projectID, Version: version})
	if err != nil {
		return fmt.Errorf("check release archive ownership: %w", err)
	}
	if !ok {
		return &UnavailableError{ProjectID: projectID, Version: version}
	}
	return nil
}

// Parents returns only explicitly pinned, verified source releases.
func Parents(ctx context.Context, q *sqlcgen.Queries, projectID, version string) ([]sqlcgen.WeaveProjectInheritanceArchive, error) {
	if err := Require(ctx, q, projectID, version); err != nil {
		return nil, err
	}
	rows, err := q.WeaveListReleaseParentTargets(ctx, sqlcgen.WeaveListReleaseParentTargetsParams{ProjectID: projectID, Version: version})
	if err != nil {
		return nil, fmt.Errorf("list release parents: %w", err)
	}
	for _, row := range rows {
		if row.SourceMode != "release" || row.SourceVersion == nil || strings.TrimSpace(*row.SourceVersion) == "" {
			return nil, &UnavailableError{ProjectID: projectID, Version: version}
		}
		if err := Require(ctx, q, row.ParentProjectID, *row.SourceVersion); err != nil {
			return nil, err
		}
	}
	return rows, nil
}
