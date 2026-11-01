package readsession

import (
	"context"

	"github.com/pletka-io/pletka/pkg/database/readsql/archive"
	"github.com/pletka-io/pletka/pkg/database/readsql/draft"
	"github.com/pletka-io/pletka/pkg/domain"
)

// FieldUsage counts contextual overrides in this session's project and selection.
// The caller must validate entity membership through the bound assembler first.
func (s *Session) FieldUsage(ctx context.Context, id string) (domain.FieldUsageCounts, error) {
	if err := s.Check(ctx); err != nil {
		return domain.FieldUsageCounts{}, err
	}
	if s.Scope().IsRelease() {
		q, err := s.ArchiveQueries()
		if err != nil {
			return domain.FieldUsageCounts{}, err
		}
		r, err := q.BrowserFieldUsage(ctx, archive.BrowserFieldUsageParams{ProjectID: s.ProjectID(), Version: s.Scope().Version(), FieldID: id})
		return domain.FieldUsageCounts{ModelsUsing: int(r.ModelsUsing), CollectionsUsing: int(r.CollectionsUsing), OverrideRowCount: int(r.OverrideCount)}, err
	}
	q, err := s.DraftQueries()
	if err != nil {
		return domain.FieldUsageCounts{}, err
	}
	r, err := q.BrowserFieldUsage(ctx, draft.BrowserFieldUsageParams{ProjectID: s.ProjectID(), FieldID: id})
	return domain.FieldUsageCounts{ModelsUsing: int(r.ModelsUsing), CollectionsUsing: int(r.CollectionsUsing), OverrideRowCount: int(r.OverrideCount)}, err
}
