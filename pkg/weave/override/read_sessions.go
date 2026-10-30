package override

import (
	"context"
	"fmt"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/database/readsession"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/projectsnapshot"
)

// Reader serves overrides owned by one bound project and database snapshot.
type Reader interface {
	ContentOverrides(context.Context, readsession.Page) ([]projectsnapshot.OverrideRecord, error)
	ContentOverrideRefs(context.Context, readsession.Page) ([]projectsnapshot.OverrideRefRecord, error)
	ContentPlacements(context.Context, readsession.Page) ([]projectsnapshot.PlacementRecord, error)

	ListForEntity(context.Context, string, string) ([]domain.FieldOverride, error)
	ListForField(context.Context, string) ([]domain.FieldOverride, error)
	ListByType(context.Context, string) ([]domain.FieldOverride, error)
	GetRefs(context.Context, int64) ([]domain.OverrideRef, error)
	GetByID(context.Context, int64) (*domain.FieldOverride, error)
	GetBase(context.Context, string) (*domain.FieldOverride, error)
	ListPlacements(context.Context, string) ([]domain.CollectionPlacement, error)
}

// ReadSessions supplies project-bound overrides within a bounded read lifetime.
type ReadSessions interface {
	Read(context.Context, auth.ReadScope, string, func(Reader) error) error
}

type readSessions struct{ factory *readsession.Factory }

// NewReadSessions adapts the shared transaction factory for override services.
func NewReadSessions(factory *readsession.Factory) ReadSessions {
	return &readSessions{factory: factory}
}

func (f *readSessions) Read(ctx context.Context, scope auth.ReadScope, projectID string, fn func(Reader) error) error {
	return f.factory.Read(ctx, scope, projectID, func(s *readsession.Session) error {
		r, err := BindReader(s)
		if err != nil {
			return err
		}
		return fn(r)
	})
}

// BindReader constructs an override reader from a validated project session.
func BindReader(s *readsession.Session) (Reader, error) {
	if s == nil {
		return nil, fmt.Errorf("override reader requires a session")
	}
	if s.Scope().IsRelease() {
		q, err := s.ArchiveQueries()
		if err != nil {
			return nil, err
		}
		return &releaseReader{queries: q, projectID: s.ProjectID(), version: s.Scope().Version()}, nil
	}
	q, err := s.DraftQueries()
	if err != nil {
		return nil, err
	}
	return &draftReader{queries: q, projectID: s.ProjectID()}, nil
}
