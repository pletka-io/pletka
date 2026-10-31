package readsession

import (
	"context"
	"fmt"

	"github.com/pletka-io/pletka/pkg/database/readsql/catalog"
	"github.com/pletka-io/pletka/pkg/projectbundle"
	"github.com/pletka-io/pletka/pkg/projectsnapshot"
)

// Catalog returns the bound project's mutable release headers on this snapshot.
// They are an explicit overlay, never a substitute for captured release content.
func (s *Session) Catalog(ctx context.Context) ([]projectbundle.ReleaseRecord, error) {
	return s.catalogRecords(ctx, "")
}

// ReleaseRecord returns only the concrete release already selected by this
// session. It cannot choose another version or read draft metadata as history.
func (s *Session) ReleaseRecord(ctx context.Context) (projectbundle.ReleaseRecord, error) {
	if err := s.Check(ctx); err != nil {
		return projectbundle.ReleaseRecord{}, err
	}
	if !s.scope.IsRelease() {
		return projectbundle.ReleaseRecord{}, fmt.Errorf("release header requires a concrete release session")
	}
	rows, err := s.catalogRecords(ctx, s.scope.Version())
	if err != nil {
		return projectbundle.ReleaseRecord{}, err
	}
	if len(rows) != 1 {
		return projectbundle.ReleaseRecord{}, &NotInReleaseError{ProjectID: s.projectID, Version: s.scope.Version(), EntityID: "release header"}
	}
	return rows[0], nil
}

func (s *Session) catalogRecords(ctx context.Context, version string) ([]projectbundle.ReleaseRecord, error) {
	if err := s.Check(ctx); err != nil {
		return nil, err
	}
	rows, err := s.catalog.ProjectReleaseCatalog(ctx, catalog.ProjectReleaseCatalogParams{ProjectID: s.projectID, SelectedVersion: version})
	if err != nil {
		return nil, err
	}
	out := make([]projectbundle.ReleaseRecord, 0, len(rows))
	for _, row := range rows {
		r := projectbundle.ReleaseRecord{ProjectID: row.ProjectID, Version: row.Version, Title: row.Title, Description: row.Description, CreatedByID: row.CreatedByID, CreatedAt: projectsnapshot.Timestamp(row.CreatedAt), ArchivedMessage: row.ArchivedMessage}
		if row.ArchivedAt != "" {
			at := projectsnapshot.Timestamp(row.ArchivedAt)
			r.ArchivedAt = &at
		}
		out = append(out, r)
	}
	return out, nil
}
