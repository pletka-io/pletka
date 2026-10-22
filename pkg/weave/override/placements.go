package override

import (
	"context"
	"fmt"

	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/domain"
)

// Collection placements: per-(model, category, collection)
// constraints for a collection group inside a model. Absence of a row means
// the defaults — optional, 0..unbounded, visible — so callers must treat a
// missing placement as domain.CollectionPlacement zero values.

// UpsertPlacement inserts or updates the placement identified by
// (ModelID, CategoryID, CollectionID). ID is written back on success.
func (s *postgresStore) UpsertPlacement(ctx context.Context, p *domain.CollectionPlacement) error {
	row, err := s.queries.WeaveUpsertCollectionPlacement(ctx, sqlcgen.WeaveUpsertCollectionPlacementParams{
		ProjectID:    p.ProjectID,
		ModelID:      p.ModelID,
		CategoryID:   p.CategoryID,
		CollectionID: p.CollectionID,
		IsRequired:   p.IsRequired,
		MinOccurs:    int32(p.MinOccurs),
		MaxOccurs:    intPtrToInt32Ptr(p.MaxOccurs),
		IsHidden:     p.IsHidden,
	})
	if err != nil {
		return fmt.Errorf("upsert collection placement %s/%s/%s: %w",
			p.ModelID, p.CategoryID, p.CollectionID, err)
	}
	p.ID = row.ID
	return nil
}

// ListPlacements returns all live placements for a model, ordered by
// category then collection. Empty slice when the model has none. projectID is
// unused: the live table is keyed by model.
func (r *liveReader) ListPlacements(ctx context.Context, _, modelID string) ([]domain.CollectionPlacement, error) {
	rows, err := r.queries.WeaveListCollectionPlacementsByModel(ctx, modelID)
	if err != nil {
		return nil, fmt.Errorf("list collection placements for %s: %w", modelID, err)
	}
	out := make([]domain.CollectionPlacement, 0, len(rows))
	for _, row := range rows {
		out = append(out, placementFromRow(row))
	}
	return out, nil
}

// ListPlacements returns the placements a model carried at r.version, from
// the archive. A version with none archived reads empty; there is no fallback
// to the live table.
func (r *archiveReader) ListPlacements(ctx context.Context, projectID, modelID string) ([]domain.CollectionPlacement, error) {
	rows, err := r.queries.WeaveListPlacementsAtVersion(ctx, sqlcgen.WeaveListPlacementsAtVersionParams{
		ProjectID:     projectID,
		VersionNumber: r.version,
		ModelID:       modelID,
	})
	if err != nil {
		return nil, fmt.Errorf("list archived collection placements for %s: %w", modelID, err)
	}
	out := make([]domain.CollectionPlacement, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.CollectionPlacement{
			ID:           row.ID,
			ProjectID:    row.ProjectID,
			ModelID:      row.ModelID,
			CategoryID:   row.CategoryID,
			CollectionID: row.CollectionID,
			IsRequired:   row.IsRequired,
			MinOccurs:    int(row.MinOccurs),
			MaxOccurs:    int32PtrToIntPtr(row.MaxOccurs),
			IsHidden:     row.IsHidden,
		})
	}
	return out, nil
}

// DeletePlacement removes the placement identified by the triple. Deleting
// a placement that does not exist is a no-op.
func (s *postgresStore) DeletePlacement(ctx context.Context, modelID, categoryID, collectionID string) error {
	err := s.queries.WeaveDeleteCollectionPlacement(ctx, sqlcgen.WeaveDeleteCollectionPlacementParams{
		ModelID:      modelID,
		CategoryID:   categoryID,
		CollectionID: collectionID,
	})
	if err != nil {
		return fmt.Errorf("delete collection placement %s/%s/%s: %w",
			modelID, categoryID, collectionID, err)
	}
	return nil
}

func placementFromRow(r sqlcgen.WeaveCollectionPlacement) domain.CollectionPlacement {
	return domain.CollectionPlacement{
		ID:           r.ID,
		ProjectID:    r.ProjectID,
		ModelID:      r.ModelID,
		CategoryID:   r.CategoryID,
		CollectionID: r.CollectionID,
		IsRequired:   r.IsRequired,
		MinOccurs:    int(r.MinOccurs),
		MaxOccurs:    int32PtrToIntPtr(r.MaxOccurs),
		IsHidden:     r.IsHidden,
	}
}

func intPtrToInt32Ptr(v *int) *int32 {
	if v == nil {
		return nil
	}
	n := int32(*v)
	return &n
}

func int32PtrToIntPtr(v *int32) *int {
	if v == nil {
		return nil
	}
	n := int(*v)
	return &n
}
