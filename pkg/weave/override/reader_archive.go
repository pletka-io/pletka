package override

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/pletka-io/pletka/pkg/database/readsession"
	"github.com/pletka-io/pletka/pkg/database/readsql/archive"
	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/projectsnapshot"
)

// releaseReader holds SELECT-only capabilities for one project and state.
type releaseReader struct {
	queries   archive.Querier
	projectID string
	version   string
}

var _ Reader = (*releaseReader)(nil)

func (r *releaseReader) ListForEntity(ctx context.Context, entityType, entityID string) ([]domain.FieldOverride, error) {
	rows, err := r.queries.ListEntityOverrides(ctx, archive.ListEntityOverridesParams{ProjectID: r.projectID, Version: r.version, EntityType: entityType, EntityID: entityID})
	if err != nil {
		return nil, fmt.Errorf("read overrides: %w", err)
	}
	out := make([]domain.FieldOverride, 0, len(rows))
	for _, row := range rows {
		out = append(out, *rowToArchivedOverride(sqlcgen.WeaveFieldOverridesArchive(row)))
	}
	return out, nil
}

func (r *releaseReader) ListForField(ctx context.Context, fieldID string) ([]domain.FieldOverride, error) {
	rows, err := r.queries.ListFieldOverrides(ctx, archive.ListFieldOverridesParams{ProjectID: r.projectID, Version: r.version, FieldID: fieldID})
	if err != nil {
		return nil, fmt.Errorf("read overrides: %w", err)
	}
	out := make([]domain.FieldOverride, 0, len(rows))
	for _, row := range rows {
		out = append(out, *rowToArchivedOverride(sqlcgen.WeaveFieldOverridesArchive(row)))
	}
	return out, nil
}

func (r *releaseReader) ListByType(ctx context.Context, entityType string) ([]domain.FieldOverride, error) {
	rows, err := r.queries.ListTypeOverrides(ctx, archive.ListTypeOverridesParams{ProjectID: r.projectID, Version: r.version, EntityType: entityType})
	if err != nil {
		return nil, fmt.Errorf("read overrides: %w", err)
	}
	out := make([]domain.FieldOverride, 0, len(rows))
	for _, row := range rows {
		out = append(out, *rowToArchivedOverride(sqlcgen.WeaveFieldOverridesArchive(row)))
	}
	return out, nil
}

func (r *releaseReader) GetByID(ctx context.Context, id int64) (*domain.FieldOverride, error) {
	row, err := r.queries.GetOverride(ctx, archive.GetOverrideParams{ProjectID: r.projectID, ID: id, Version: r.version})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, &readsession.NotInReleaseError{ProjectID: r.projectID, Version: r.version, EntityID: fmt.Sprint(id)}
	}
	if err != nil {
		return nil, fmt.Errorf("read override: %w", err)
	}
	return rowToArchivedOverride(sqlcgen.WeaveFieldOverridesArchive(row)), nil
}
func (r *releaseReader) GetBase(ctx context.Context, fieldID string) (*domain.FieldOverride, error) {
	row, err := r.queries.GetBaseOverride(ctx, archive.GetBaseOverrideParams{ProjectID: r.projectID, FieldID: fieldID, Version: r.version})
	// A field may legitimately have no base override in this project.
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read base override: %w", err)
	}
	return rowToArchivedOverride(sqlcgen.WeaveFieldOverridesArchive(row)), nil
}
func (r *releaseReader) GetRefs(ctx context.Context, id int64) ([]domain.OverrideRef, error) {
	own, err := r.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if own == nil {
		return nil, errNotFound
	}
	rows, err := r.queries.OverrideRefs(ctx, archive.OverrideRefsParams{ProjectID: r.projectID, ID: id, Version: r.version})
	if err != nil {
		return nil, fmt.Errorf("read override refs: %w", err)
	}
	out := make([]domain.OverrideRef, 0, len(rows))
	for _, row := range rows {
		out = append(out, rowToArchivedRef(sqlcgen.WeaveOverrideRefsArchive(row)))
	}
	return out, nil
}
func (r *releaseReader) ListPlacements(ctx context.Context, modelID string) ([]domain.CollectionPlacement, error) {
	rows, err := r.queries.ModelPlacements(ctx, archive.ModelPlacementsParams{ProjectID: r.projectID, ModelID: modelID, Version: r.version})
	if err != nil {
		return nil, fmt.Errorf("read model placements: %w", err)
	}
	out := make([]domain.CollectionPlacement, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.CollectionPlacement{ID: row.ID, ProjectID: row.ProjectID, ModelID: row.ModelID, CategoryID: row.CategoryID, CollectionID: row.CollectionID, IsRequired: row.IsRequired, MinOccurs: int(row.MinOccurs), MaxOccurs: int32PtrToIntPtr(row.MaxOccurs), IsHidden: row.IsHidden})
	}
	return out, nil
}

// ContentPlacements returns one deterministic page of lossless owned records.
func (r *releaseReader) ContentPlacements(ctx context.Context, page readsession.Page) ([]projectsnapshot.PlacementRecord, error) {
	if err := page.Validate(); err != nil {
		return nil, err
	}
	rows, err := r.queries.ContentPlacements(ctx, archive.ContentPlacementsParams{Restricted: page.Restricted(), SelectedModel: page.IDs("model"), ProjectID: r.projectID, PageLimit: page.Limit, PageOffset: page.Offset, Version: r.version})
	if err != nil {
		return nil, fmt.Errorf("read placements content: %w", err)
	}
	out := make([]projectsnapshot.PlacementRecord, 0, len(rows))
	for _, row := range rows {
		out = append(out, projectsnapshot.PlacementRecord{
			ID:            row.ID,
			ProjectID:     row.ProjectID,
			ModelID:       row.ModelID,
			CategoryID:    row.CategoryID,
			CollectionID:  row.CollectionID,
			IsRequired:    row.IsRequired,
			MinOccurs:     row.MinOccurs,
			MaxOccurs:     row.MaxOccurs,
			IsHidden:      row.IsHidden,
			CreatedAt:     projectsnapshot.Timestamp(row.CreatedAt),
			UpdatedAt:     projectsnapshot.Timestamp(row.UpdatedAt),
			VersionNumber: row.VersionNumber,
		})
	}
	return out, nil
}

// ContentOverrides returns one deterministic page of lossless owned records.
func (r *releaseReader) ContentOverrides(ctx context.Context, page readsession.Page) ([]projectsnapshot.OverrideRecord, error) {
	if err := page.Validate(); err != nil {
		return nil, err
	}
	rows, err := r.queries.ContentOverrides(ctx, archive.ContentOverridesParams{Restricted: page.Restricted(), SelectedOverride: page.IDs("override"), SelectedField: page.IDs("field"), SelectedModel: page.IDs("model"), SelectedCollection: page.IDs("collection"), ProjectID: r.projectID, PageLimit: page.Limit, PageOffset: page.Offset, Version: r.version})
	if err != nil {
		return nil, fmt.Errorf("read overrides content: %w", err)
	}
	out := make([]projectsnapshot.OverrideRecord, 0, len(rows))
	for _, row := range rows {
		out = append(out, projectsnapshot.OverrideRecord{
			ID:                 row.ID,
			FieldID:            row.FieldID,
			ProjectID:          row.ProjectID,
			EntityType:         row.EntityType,
			EntityID:           row.EntityID,
			Position:           row.Position,
			CollectionOrder:    row.CollectionOrder,
			DisplayName:        row.DisplayName,
			Description:        row.Description,
			CollectionName:     row.CollectionName,
			CategoryID:         row.CategoryID,
			PartOfCollectionID: row.PartOfCollectionID,
			ExpectedValueType:  row.ExpectedValueType,
			SetValue:           row.SetValue,
			IsRequired:         row.IsRequired,
			MinOccurs:          row.MinOccurs,
			MaxOccurs:          row.MaxOccurs,
			IsHidden:           row.IsHidden,
			Visibility:         row.Visibility,
			StagingID:          row.StagingID,
			CreatedAt:          projectsnapshot.Timestamp(row.CreatedAt),
			UpdatedAt:          projectsnapshot.Timestamp(row.UpdatedAt),
			ContentHash:        row.ContentHash,
			VersionNumber:      row.VersionNumber,
			SetValueEntryID:    row.SetValueEntryID,
		})
	}
	return out, nil
}

// ContentOverrideRefs returns one deterministic page of lossless owned records.
func (r *releaseReader) ContentOverrideRefs(ctx context.Context, page readsession.Page) ([]projectsnapshot.OverrideRefRecord, error) {
	if err := page.Validate(); err != nil {
		return nil, err
	}
	rows, err := r.queries.ContentOverrideRefs(ctx, archive.ContentOverrideRefsParams{Restricted: page.Restricted(), SelectedOverride: page.IDs("override"), ProjectID: r.projectID, PageLimit: page.Limit, PageOffset: page.Offset, Version: r.version})
	if err != nil {
		return nil, fmt.Errorf("read overriderefs content: %w", err)
	}
	out := make([]projectsnapshot.OverrideRefRecord, 0, len(rows))
	for _, row := range rows {
		out = append(out, projectsnapshot.OverrideRefRecord{
			OverrideID:    row.OverrideID,
			RefType:       row.RefType,
			TargetID:      row.TargetID,
			SemanticID:    row.SemanticID,
			Position:      row.Position,
			ProjectID:     row.ProjectID,
			VersionNumber: row.VersionNumber,
		})
	}
	return out, nil
}
