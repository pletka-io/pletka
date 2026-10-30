// Package readsession owns the bounded, project-specific database lifetime for
// content readers. Request authorization and selection policy remain in auth.
package readsession

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/database/readsql/archive"
	"github.com/pletka-io/pletka/pkg/database/readsql/draft"
	"github.com/pletka-io/pletka/pkg/database/readsql/projectontologies"
	"github.com/pletka-io/pletka/pkg/database/releasearchive"
	"github.com/pletka-io/pletka/pkg/database/releasedeps"
	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
)

// Factory creates read-only repeatable-read sessions. It owns transaction setup;
// readers never receive a pool or transaction with which to open another view.
type Factory struct{ pool *pgxpool.Pool }

// NewFactory constructs a factory over the application pool.
func NewFactory(pool *pgxpool.Pool) *Factory { return &Factory{pool: pool} }

// Session is valid only during Factory.Read's callback. It is not safe for
// concurrent use. Its project and selection cannot be changed by a consumer.
type Session struct {
	projectID  string
	scope      auth.ReadScope
	draft      *draft.Queries
	archive    *archive.Queries
	ontologies *projectontologies.Queries
	lifetime   *lifetime
	source     func(context.Context, string, string) (*Session, error)
}

type lifetime struct{ closed bool }

// ProjectID returns the only project this session's readers may serve.
func (s *Session) ProjectID() string { return s.projectID }

// Scope returns the already-resolved selection, without changing request policy.
func (s *Session) Scope() auth.ReadScope { return s.scope }

// DraftQueries exposes only generated draft SELECTs, and only for a draft session.
func (s *Session) DraftQueries() (draft.Querier, error) {
	if s == nil || s.lifetime == nil || s.lifetime.closed || !s.scope.Valid() || s.draft == nil {
		return nil, fmt.Errorf("draft queries require an open draft session")
	}
	return s.draft, nil
}

// ArchiveQueries exposes only generated archive SELECTs. The generated query
// type has no live-table or write methods, even behind its interface.
func (s *Session) ArchiveQueries() (archive.Querier, error) {
	if s == nil || s.lifetime == nil || s.lifetime.closed || !s.scope.Valid() || s.archive == nil {
		return nil, fmt.Errorf("archive queries require an open release session")
	}
	return s.archive, nil
}

// Read validates a concrete selection, then runs fn against one database
// snapshot. The callback must finish all DB work before returning; rendering,
// network calls and publication belong after this method releases the connection.
func (f *Factory) Read(ctx context.Context, scope auth.ReadScope, projectID string, fn func(*Session) error) (err error) {
	if !scope.Valid() || strings.TrimSpace(projectID) == "" || (scope.IsRelease() && strings.TrimSpace(scope.Version()) == "") {
		return fmt.Errorf("read session requires a project and an explicit draft or nonempty release selection")
	}
	if f == nil || f.pool == nil || fn == nil {
		return fmt.Errorf("read session factory is not configured")
	}
	tx, err := f.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return fmt.Errorf("begin read session: %w", err)
	}
	life := &lifetime{}
	defer func() {
		life.closed = true
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if rollbackErr := tx.Rollback(cleanup); rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) {
			err = errors.Join(err, fmt.Errorf("close read session: %w", rollbackErr))
		}
	}()
	s, err := bind(ctx, sqlcgen.New(tx), tx, life, projectID, scope)
	if err != nil {
		return err
	}
	return fn(s)
}

// Dependency returns a reader session for an entity's captured source release,
// sharing this callback's snapshot and lifetime. It does not grant authorization;
// callers must apply their source-project access policy before serving content.
// Draft dependency policy remains separate; missing released pins never use it.
func (s *Session) Dependency(ctx context.Context, kind, id string) (*Session, error) {
	if s == nil || s.lifetime == nil || s.lifetime.closed || s.source == nil {
		return nil, fmt.Errorf("dependency reader requires an open release session")
	}
	return s.source(ctx, kind, id)
}

func bind(ctx context.Context, q *sqlcgen.Queries, tx pgx.Tx, life *lifetime, projectID string, scope auth.ReadScope) (*Session, error) {
	s := &Session{projectID: projectID, scope: scope, lifetime: life}
	if scope.IsRelease() {
		if err := releasearchive.Require(ctx, q, projectID, scope.Version()); err != nil {
			return nil, err
		}
		s.archive = archive.New(tx)
		s.ontologies = projectontologies.New(tx)
		s.source = func(ctx context.Context, kind, id string) (*Session, error) {
			project, version, err := releasedeps.EntitySource(ctx, q, projectID, scope.Version(), kind, id)
			if err != nil {
				return nil, err
			}
			return bind(ctx, q, tx, life, project, auth.Release(version))
		}
	} else {
		if _, err := q.WeaveGetProjectByID(ctx, projectID); err != nil {
			return nil, fmt.Errorf("open project read session: %w", err)
		}
		s.draft = draft.New(tx)
	}
	return s, nil
}

// NotInReleaseError reports missing content without substituting working rows.
type NotInReleaseError struct{ ProjectID, Version, EntityID string }

func (e *NotInReleaseError) Error() string {
	return fmt.Sprintf("%s is not in release %s@%s", e.EntityID, e.ProjectID, e.Version)
}

// IsNotInRelease selects the canonical API error envelope.
func (e *NotInReleaseError) IsNotInRelease() bool { return true }
