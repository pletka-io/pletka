package readsession

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/pletka-io/pletka/pkg/database/readsql/archive"
	"github.com/pletka-io/pletka/pkg/projectsnapshot"
)

// ImportOrigin describes local storage provenance, separate from the original
// capture contract and frozen content. A re-export must not put this installation's
// import timestamp into the original immutable project tree.
type ImportOrigin struct {
	Imported   bool
	ImportedAt projectsnapshot.Timestamp
}

// Origin distinguishes an imported complete history from a local release cut on
// the same authorized read session. Draft, closed and uncertified scopes fail.
func (s *Session) Origin(ctx context.Context) (ImportOrigin, error) {
	q, err := s.ArchiveQueries()
	if err != nil {
		return ImportOrigin{}, err
	}
	row, err := q.ReleaseImportOrigin(ctx, archive.ReleaseImportOriginParams{ProjectID: s.projectID, Version: s.scope.Version()})
	if errors.Is(err, pgx.ErrNoRows) {
		return ImportOrigin{}, &NotInReleaseError{ProjectID: s.projectID, Version: s.scope.Version(), EntityID: "complete release origin"}
	}
	if err != nil {
		return ImportOrigin{}, err
	}
	return ImportOrigin{Imported: row.Imported, ImportedAt: projectsnapshot.Timestamp(row.ImportedAt)}, nil
}
