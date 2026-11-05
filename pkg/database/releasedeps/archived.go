package releasedeps

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
)

// archivedQueries are the SELECTs an archived dependency plan needs. Capture
// supplies *sqlcgen.Queries; tests supply fakes.
type archivedQueries interface {
	WeaveArchivedDependencyReferences(context.Context, sqlcgen.WeaveArchivedDependencyReferencesParams) ([]sqlcgen.WeaveArchivedDependencyReferencesRow, error)
	WeaveArchivedFollowingDependencies(context.Context, sqlcgen.WeaveArchivedFollowingDependenciesParams) ([]string, error)
	WeaveArchivedDependencyEntityOwner(context.Context, sqlcgen.WeaveArchivedDependencyEntityOwnerParams) (string, error)
	WeaveDraftDependencyEntityOwner(context.Context, sqlcgen.WeaveDraftDependencyEntityOwnerParams) (string, error)
	WeaveLatestDependencyRelease(context.Context, string) (string, error)
}

// archivedDraft feeds the planner a release's own archived rows in place of
// the working draft. Entity ownership falls back from live rows to archived
// rows and then to the planner's pinned closure; the latest-release choice is
// the catalog's, as it was at the original cut.
type archivedDraft struct {
	q       archivedQueries
	version string
}

func (a archivedDraft) WeaveDraftFollowingDependencies(ctx context.Context, project string) ([]string, error) {
	return a.q.WeaveArchivedFollowingDependencies(ctx, sqlcgen.WeaveArchivedFollowingDependenciesParams{ProjectID: project, Version: a.version})
}
func (a archivedDraft) WeaveLatestDependencyRelease(ctx context.Context, project string) (string, error) {
	return a.q.WeaveLatestDependencyRelease(ctx, project)
}
func (a archivedDraft) WeaveDraftDependencyEntityOwner(ctx context.Context, p sqlcgen.WeaveDraftDependencyEntityOwnerParams) (string, error) {
	owner, err := a.q.WeaveDraftDependencyEntityOwner(ctx, p)
	if !errors.Is(err, pgx.ErrNoRows) {
		return owner, err
	}
	return a.q.WeaveArchivedDependencyEntityOwner(ctx, sqlcgen.WeaveArchivedDependencyEntityOwnerParams{EntityType: p.EntityType, EntityID: p.EntityID})
}
func (a archivedDraft) WeaveDraftDependencyReferences(ctx context.Context, project string) ([]sqlcgen.WeaveDraftDependencyReferencesRow, error) {
	rows, err := a.q.WeaveArchivedDependencyReferences(ctx, sqlcgen.WeaveArchivedDependencyReferencesParams{ProjectID: project, Version: a.version})
	if err != nil {
		return nil, err
	}
	out := make([]sqlcgen.WeaveDraftDependencyReferencesRow, len(rows))
	for i, row := range rows {
		out[i] = sqlcgen.WeaveDraftDependencyReferencesRow(row)
	}
	return out, nil
}

// CaptureArchived plans and persists a release's dependency pins from its
// archived rows. It runs in the preserve transaction before evidence capture.
func CaptureArchived(ctx context.Context, q *sqlcgen.Queries, projectID, version string) error {
	plan, err := planDependencies(ctx, archivedDraft{q: q, version: version}, q, projectID, version, func(context.Context, string) error { return nil })
	if err != nil {
		return err
	}
	return persist(ctx, q, projectID, version, plan)
}
