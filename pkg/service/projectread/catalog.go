package projectread

import (
	"context"
	"fmt"

	"github.com/pletka-io/pletka/pkg/database/readsession"
	"github.com/pletka-io/pletka/pkg/projectbundle"
	"github.com/pletka-io/pletka/pkg/projectsnapshot"
)

// Catalog reads the explicit mutable overlay for an already assembled project.
// Call it before closing the same session. Source headers follow the session's
// concrete authorized selections; older root records do not create release bodies.
func Catalog(ctx context.Context, s *readsession.Session, value projectsnapshot.CompleteProject) (projectbundle.Catalog, error) {
	if err := s.Check(ctx); err != nil {
		return projectbundle.Catalog{}, err
	}
	root, err := value.Root()
	if err != nil {
		return projectbundle.Catalog{}, err
	}
	if root.ProjectID() != s.ProjectID() || root.IsDraft() == s.Scope().IsRelease() || root.Version() != s.Scope().Version() {
		return projectbundle.Catalog{}, fmt.Errorf("catalog session differs from assembled selection")
	}
	records, err := s.Catalog(ctx)
	if err != nil {
		return projectbundle.Catalog{}, err
	}
	ids, err := value.Projects()
	if err != nil {
		return projectbundle.Catalog{}, err
	}
	for _, id := range ids {
		if id == root {
			continue
		}
		bound, err := s.ProjectDependency(ctx, id.ProjectID())
		if err != nil {
			return projectbundle.Catalog{}, err
		}
		if id.IsDraft() || bound.Scope().Version() != id.Version() {
			return projectbundle.Catalog{}, fmt.Errorf("catalog source differs from assembled selection")
		}
		record, err := bound.ReleaseRecord(ctx)
		if err != nil {
			return projectbundle.Catalog{}, err
		}
		records = append(records, record)
	}
	return projectbundle.NewCatalog(value, records)
}
