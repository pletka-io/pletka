package vocabulary

import (
	"context"
	"fmt"

	"github.com/pletka-io/pletka/pkg/database/readsession"
	"github.com/pletka-io/pletka/pkg/projectsnapshot"
)

// ContentReader returns lossless owned rows through a bound session.
// It has no independent project, version, transaction or write capability.
type ContentReader interface {
	ContentVocabularies(context.Context, readsession.Page) ([]projectsnapshot.VocabularyRecord, error)
	ContentVocabularyEntries(context.Context, readsession.Page) ([]projectsnapshot.VocabularyEntryRecord, error)
	ContentConceptLists(context.Context, readsession.Page) ([]projectsnapshot.ConceptListRecord, error)
	ContentConceptEntries(context.Context, readsession.Page) ([]projectsnapshot.ConceptEntryRecord, error)
	ContentBroader(context.Context, readsession.Page) ([]projectsnapshot.BroaderRecord, error)
}

// BindContentReader binds the vocabulary slice’s lossless row facet to one session.
func BindContentReader(s *readsession.Session) (ContentReader, error) {
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
