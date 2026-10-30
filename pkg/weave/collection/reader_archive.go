package collection

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

func (r *releaseReader) Get(ctx context.Context, id string) (*domain.Collection, error) {
	row, err := r.queries.GetCollection(ctx, archive.GetCollectionParams{ProjectID: r.projectID, ID: id, Version: r.version})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, &readsession.NotInReleaseError{ProjectID: r.projectID, Version: r.version, EntityID: fmt.Sprint(id)}
	}
	if err != nil {
		return nil, fmt.Errorf("get collection: %w", err)
	}
	return archiveRowToCollection(sqlcgen.WeaveCollectionsArchive(row)), nil
}
func (r *releaseReader) List(ctx context.Context, opts ...domain.QueryOption) ([]*domain.Collection, int64, error) {
	cfg, err := boundListOptions(r.projectID, r.version, opts)
	if err != nil {
		return nil, 0, err
	}
	scopeClass, _ := cfg.Filters["scope_class"].(string)
	categoryID, _ := cfg.Filters["category_id"].(string)
	status, _ := cfg.Filters["status"].(string)
	rows, err := r.queries.ListCollections(ctx, archive.ListCollectionsParams{ProjectID: r.projectID, Version: r.version, Search: cfg.Search, ScopeClass: scopeClass, CategoryID: categoryID, Status: status, SortBy: cfg.OrderBy, SortDesc: cfg.OrderDesc, ResultLimit: cfg.limit, ResultOffset: cfg.offset})
	if err != nil {
		return nil, 0, fmt.Errorf("list collections: %w", err)
	}
	count, err := r.queries.CountCollections(ctx, archive.CountCollectionsParams{ProjectID: r.projectID, Version: r.version, Search: cfg.Search, ScopeClass: scopeClass, CategoryID: categoryID, Status: status})
	if err != nil {
		return nil, 0, fmt.Errorf("count collections: %w", err)
	}
	out := make([]*domain.Collection, 0, len(rows))
	for _, row := range rows {
		out = append(out, archiveRowToCollection(sqlcgen.WeaveCollectionsArchive(row)))
	}
	return out, count, nil
}
func (r *releaseReader) Usage(ctx context.Context, id string) (UsageReport, error) {
	count, err := r.queries.CollectionUsageCount(ctx, archive.CollectionUsageCountParams{ProjectID: r.projectID, ID: id, Version: r.version})
	if err != nil {
		return UsageReport{}, fmt.Errorf("collection usage count: %w", err)
	}
	out := UsageReport{FieldCount: count}
	if count == 0 {
		return out, nil
	}
	rows, err := r.queries.CollectionUsageSamples(ctx, archive.CollectionUsageSamplesParams{ProjectID: r.projectID, ID: id, Version: r.version})
	if err != nil {
		return UsageReport{}, fmt.Errorf("collection usage samples: %w", err)
	}
	for _, row := range rows {
		out.FieldSamples = append(out.FieldSamples, FieldRef{FieldID: row.FieldID, FieldSemanticID: row.FieldSemanticID, FieldName: row.FieldName})
	}
	return out, nil
}
func (r *releaseReader) ScopeClasses(ctx context.Context) ([]string, error) {
	return r.queries.CollectionScopeClasses(ctx, archive.CollectionScopeClassesParams{ProjectID: r.projectID, Version: r.version})
}
func (r *releaseReader) CategoriesUsed(ctx context.Context) ([]CollectionCategoryOption, error) {
	rows, err := r.queries.CollectionCategories(ctx, archive.CollectionCategoriesParams{ProjectID: r.projectID, Version: r.version})
	if err != nil {
		return nil, fmt.Errorf("collection categories: %w", err)
	}
	out := make([]CollectionCategoryOption, 0, len(rows))
	for _, row := range rows {
		out = append(out, CollectionCategoryOption{ID: row.ID, UIName: unmarshalTranslations(row.UiName), CanonicalOrder: int(row.CanonicalOrder)})
	}
	return out, nil
}
func (r *releaseReader) CompositionCounts(ctx context.Context, ids []string) (map[string]domain.CompositionCounts, error) {
	rows, err := r.queries.CollectionCounts(ctx, archive.CollectionCountsParams{ProjectID: r.projectID, Ids: ids, Version: r.version})
	if err != nil {
		return nil, fmt.Errorf("collection composition counts: %w", err)
	}
	out := make(map[string]domain.CompositionCounts, len(rows))
	for _, row := range rows {
		out[row.EntityID] = domain.CompositionCounts{FieldCount: int(row.FieldCount)}
	}
	return out, nil
}

// ContentCollections returns one deterministic page of lossless owned records.
func (r *releaseReader) ContentCollections(ctx context.Context, page readsession.Page) ([]projectsnapshot.CollectionRecord, error) {
	if err := page.Validate(); err != nil {
		return nil, err
	}
	rows, err := r.queries.ContentCollections(ctx, archive.ContentCollectionsParams{ProjectID: r.projectID, PageLimit: page.Limit, PageOffset: page.Offset, Version: r.version})
	if err != nil {
		return nil, fmt.Errorf("read collections content: %w", err)
	}
	out := make([]projectsnapshot.CollectionRecord, 0, len(rows))
	for _, row := range rows {
		out = append(out, projectsnapshot.CollectionRecord{
			ID:                       row.ID,
			CreatedAt:                projectsnapshot.Timestamp(row.CreatedAt),
			UpdatedAt:                projectsnapshot.Timestamp(row.UpdatedAt),
			SystemName:               row.SystemName,
			UIName:                   row.UiName,
			Description:              row.Description,
			Status:                   row.Status,
			ProjectID:                row.ProjectID,
			OntologyScope:            row.OntologyScope,
			CollectionNumber:         row.CollectionNumber,
			CanonicalCollectionOrder: row.CanonicalCollectionOrder,
			StagingID:                row.StagingID,
			Deprecated:               row.Deprecated,
			DefaultCategoryID:        row.DefaultCategoryID,
			VersionNumber:            row.VersionNumber,
		})
	}
	return out, nil
}
