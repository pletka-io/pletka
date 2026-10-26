package collection

import (
	"context"
	"fmt"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/database/readsession"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/override"
)

// Reader serves only its bound project's content in one database snapshot.
// Foreign sources require separate dependency resolution, never a project argument.
type Reader interface {
	Get(context.Context, string) (*domain.Collection, error)
	List(context.Context, ...domain.QueryOption) ([]*domain.Collection, int64, error)
	Usage(context.Context, string) (UsageReport, error)
	ScopeClasses(context.Context) ([]string, error)
	CategoriesUsed(context.Context) ([]CollectionCategoryOption, error)
	CompositionCounts(context.Context, []string) (map[string]domain.CompositionCounts, error)
}

// ReadSessions supplies collection and override readers from the same transaction.
type ReadSessions interface {
	Read(context.Context, auth.ReadScope, string, func(Reader, override.Reader) error) error
}

type readSessions struct{ factory *readsession.Factory }

// NewReadSessions adapts the shared lifetime factory to this slice's read surface.
func NewReadSessions(factory *readsession.Factory) ReadSessions {
	return &readSessions{factory: factory}
}

func (f *readSessions) Read(ctx context.Context, scope auth.ReadScope, projectID string, fn func(Reader, override.Reader) error) error {
	return f.factory.Read(ctx, scope, projectID, func(s *readsession.Session) error {
		r, err := BindReader(s)
		if err != nil {
			return err
		}
		o, err := override.BindReader(s)
		if err != nil {
			return err
		}
		return fn(r, o)
	})
}

// BindReader accepts only a validated session, with no project or database override.
func BindReader(s *readsession.Session) (Reader, error) {
	if s == nil {
		return nil, fmt.Errorf("collection reader requires a session")
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
