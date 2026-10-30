package releasedeps

import (
	"context"
	"fmt"

	archive "github.com/pletka-io/pletka/pkg/database/readsql/dependencyarchive"
	draft "github.com/pletka-io/pletka/pkg/database/readsql/dependencydraft"
	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
)

// PlanDraft selects dependencies using only generated SELECT capabilities. It
// writes no pins, receipts or archive rows and never follows a source draft.
func PlanDraft(ctx context.Context, working draft.Querier, captured archive.Querier, project string, authorize func(context.Context, string) error) (Plan, error) {
	if working == nil || captured == nil {
		return Plan{}, fmt.Errorf("draft dependency queries are required")
	}
	return planDependencies(ctx, draftAdapter{working}, archiveAdapter{captured}, project, "", authorize)
}

type draftAdapter struct{ queries draft.Querier }

func (a draftAdapter) WeaveDraftFollowingDependencies(ctx context.Context, project string) ([]string, error) {
	return a.queries.WeaveDraftFollowingDependencies(ctx, project)
}
func (a draftAdapter) WeaveLatestDependencyRelease(ctx context.Context, project string) (string, error) {
	return a.queries.WeaveLatestDependencyRelease(ctx, project)
}
func (a draftAdapter) WeaveDraftDependencyEntityOwner(ctx context.Context, p sqlcgen.WeaveDraftDependencyEntityOwnerParams) (string, error) {
	return a.queries.WeaveDraftDependencyEntityOwner(ctx, draft.WeaveDraftDependencyEntityOwnerParams(p))
}
func (a draftAdapter) WeaveDraftDependencyReferences(ctx context.Context, project string) ([]sqlcgen.WeaveDraftDependencyReferencesRow, error) {
	rows, err := a.queries.WeaveDraftDependencyReferences(ctx, project)
	if err != nil {
		return nil, err
	}
	out := make([]sqlcgen.WeaveDraftDependencyReferencesRow, len(rows))
	for i, row := range rows {
		out[i] = sqlcgen.WeaveDraftDependencyReferencesRow(row)
	}
	return out, nil
}
