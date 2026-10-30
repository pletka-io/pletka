package readsession

import (
	"context"
	"fmt"

	"github.com/pletka-io/pletka/pkg/projectsnapshot"
)

// ContentReader returns lossless owned rows through a bound session.
// It has no independent project, version, transaction or write capability.
type ContentReader interface {
	ContentAdoptions(context.Context, Page) ([]projectsnapshot.AdoptionRecord, error)
	ContentCategories(context.Context, Page) ([]projectsnapshot.CategoryRecord, error)
	ContentForks(context.Context, Page) ([]projectsnapshot.ForkRecord, error)
	ContentFields(context.Context, Page) ([]projectsnapshot.FieldRecord, error)
	ContentModels(context.Context, Page) ([]projectsnapshot.ModelRecord, error)
	ContentNamespaces(context.Context, Page) ([]projectsnapshot.NamespaceRecord, error)
	ContentProjectActors(context.Context, Page) ([]projectsnapshot.ProjectActorRecord, error)
	ContentAttributions(context.Context, Page) ([]projectsnapshot.AttributionRecord, error)
	ContentInheritance(context.Context, Page) ([]projectsnapshot.InheritanceRecord, error)
	ContentOntologyLinks(context.Context, Page) ([]projectsnapshot.OntologyLinkRecord, error)
	ContentProjects(context.Context, Page) ([]projectsnapshot.ProjectRecord, error)
}

// ContentRows binds the transitional readers for unconverted sections.
func (s *Session) ContentRows() (ContentReader, error) {
	if s == nil {
		return nil, fmt.Errorf("content reader requires an open session")
	}
	if s.Scope().IsRelease() {
		q, err := s.ArchiveQueries()
		if err != nil {
			return nil, err
		}
		return &archiveContentReader{queries: q, projectID: s.ProjectID(), version: s.Scope().Version()}, nil
	}
	q, err := s.DraftQueries()
	if err != nil {
		return nil, err
	}
	return &draftContentReader{queries: q, projectID: s.ProjectID()}, nil
}
