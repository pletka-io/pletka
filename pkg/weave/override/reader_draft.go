package override

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/pletka-io/pletka/pkg/database/readsql/draft"
	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/domain"
)

// draftReader holds SELECT-only capabilities for one project and state.
type draftReader struct {
	queries   draft.Querier
	projectID string
}

var _ Reader = (*draftReader)(nil)

func (r *draftReader) ListForEntity(ctx context.Context, entityType, entityID string) ([]domain.FieldOverride, error) {
	rows, err := r.queries.ListEntityOverrides(ctx, draft.ListEntityOverridesParams{ProjectID: r.projectID, EntityType: entityType, EntityID: entityID})
	if err != nil {
		return nil, fmt.Errorf("read overrides: %w", err)
	}
	out := make([]domain.FieldOverride, 0, len(rows))
	for _, row := range rows {
		out = append(out, *rowToOverride(sqlcgen.WeaveFieldOverride(row)))
	}
	return out, nil
}

func (r *draftReader) ListForField(ctx context.Context, fieldID string) ([]domain.FieldOverride, error) {
	rows, err := r.queries.ListFieldOverrides(ctx, draft.ListFieldOverridesParams{ProjectID: r.projectID, FieldID: fieldID})
	if err != nil {
		return nil, fmt.Errorf("read overrides: %w", err)
	}
	out := make([]domain.FieldOverride, 0, len(rows))
	for _, row := range rows {
		out = append(out, *rowToOverride(sqlcgen.WeaveFieldOverride(row)))
	}
	return out, nil
}

func (r *draftReader) ListByType(ctx context.Context, entityType string) ([]domain.FieldOverride, error) {
	rows, err := r.queries.ListTypeOverrides(ctx, draft.ListTypeOverridesParams{ProjectID: r.projectID, EntityType: entityType})
	if err != nil {
		return nil, fmt.Errorf("read overrides: %w", err)
	}
	out := make([]domain.FieldOverride, 0, len(rows))
	for _, row := range rows {
		out = append(out, *rowToOverride(sqlcgen.WeaveFieldOverride(row)))
	}
	return out, nil
}

func (r *draftReader) GetByID(ctx context.Context, id int64) (*domain.FieldOverride, error) {
	row, err := r.queries.GetOverride(ctx, draft.GetOverrideParams{ProjectID: r.projectID, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read override: %w", err)
	}
	return rowToOverride(sqlcgen.WeaveFieldOverride(row)), nil
}
func (r *draftReader) GetBase(ctx context.Context, fieldID string) (*domain.FieldOverride, error) {
	row, err := r.queries.GetBaseOverride(ctx, draft.GetBaseOverrideParams{ProjectID: r.projectID, FieldID: fieldID})
	// A field may legitimately have no base override in this project.
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read base override: %w", err)
	}
	return rowToOverride(sqlcgen.WeaveFieldOverride(row)), nil
}
func (r *draftReader) GetRefs(ctx context.Context, id int64) ([]domain.OverrideRef, error) {
	own, err := r.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if own == nil {
		return nil, errNotFound
	}
	rows, err := r.queries.OverrideRefs(ctx, draft.OverrideRefsParams{ProjectID: r.projectID, ID: id})
	if err != nil {
		return nil, fmt.Errorf("read override refs: %w", err)
	}
	out := make([]domain.OverrideRef, 0, len(rows))
	for _, row := range rows {
		out = append(out, rowToRef(sqlcgen.WeaveOverrideRef(row)))
	}
	return out, nil
}
func (r *draftReader) ListPlacements(ctx context.Context, modelID string) ([]domain.CollectionPlacement, error) {
	rows, err := r.queries.ModelPlacements(ctx, draft.ModelPlacementsParams{ProjectID: r.projectID, ModelID: modelID})
	if err != nil {
		return nil, fmt.Errorf("read model placements: %w", err)
	}
	out := make([]domain.CollectionPlacement, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.CollectionPlacement{ID: row.ID, ProjectID: row.ProjectID, ModelID: row.ModelID, CategoryID: row.CategoryID, CollectionID: row.CollectionID, IsRequired: row.IsRequired, MinOccurs: int(row.MinOccurs), MaxOccurs: int32PtrToIntPtr(row.MaxOccurs), IsHidden: row.IsHidden})
	}
	return out, nil
}
