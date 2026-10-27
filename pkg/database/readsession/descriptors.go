package readsession

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/pletka-io/pletka/pkg/database/readsql/archive"
	"github.com/pletka-io/pletka/pkg/database/releasedescriptor"
)

// Descriptors returns detached identity values for this session's release. Draft
// sessions and closed sessions cannot acquire this capability.
func (s *Session) Descriptors(ctx context.Context) (releasedescriptor.Snapshot, error) {
	q, err := s.ArchiveQueries()
	if err != nil {
		return releasedescriptor.Snapshot{}, err
	}
	data, err := q.ReleaseDescriptors(ctx, archive.ReleaseDescriptorsParams{ProjectID: s.projectID, Version: s.scope.Version()})
	if errors.Is(err, pgx.ErrNoRows) {
		return releasedescriptor.Snapshot{}, &releasedescriptor.UnavailableError{ProjectID: s.projectID, Version: s.scope.Version()}
	}
	if err != nil {
		return releasedescriptor.Snapshot{}, fmt.Errorf("read release descriptors: %w", err)
	}
	return releasedescriptor.Decode(data, s.projectID, s.scope.Version())
}
