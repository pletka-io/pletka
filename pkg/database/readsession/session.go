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
	"github.com/pletka-io/pletka/pkg/database/releasearchive"
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
	projectID string
	scope     auth.ReadScope
	draft     *draft.Queries
	archive   *archive.Queries
	closed    bool
}

// ProjectID returns the only project this session's readers may serve.
func (s *Session) ProjectID() string { return s.projectID }

// Scope returns the already-resolved selection, without changing request policy.
func (s *Session) Scope() auth.ReadScope { return s.scope }

// DraftQueries exposes only generated draft SELECTs, and only for a draft session.
func (s *Session) DraftQueries() (draft.Querier, error) {
	if s == nil || s.closed || !s.scope.Valid() || s.draft == nil {
		return nil, fmt.Errorf("draft queries require an open draft session")
	}
	return s.draft, nil
}

// ArchiveQueries exposes only generated archive SELECTs. The generated query
// type has no live-table or write methods, even behind its interface.
func (s *Session) ArchiveQueries() (archive.Querier, error) {
	if s == nil || s.closed || !s.scope.Valid() || s.archive == nil {
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
	s := &Session{projectID: projectID, scope: scope}
	defer func() {
		s.closed = true
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if rollbackErr := tx.Rollback(cleanup); rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) {
			err = errors.Join(err, fmt.Errorf("close read session: %w", rollbackErr))
		}
	}()
	q := sqlcgen.New(tx)
	if scope.IsRelease() {
		if err := releasearchive.Require(ctx, q, projectID, scope.Version()); err != nil {
			return err
		}
		s.archive = archive.New(tx)
	} else {
		if _, err := q.WeaveGetProjectByID(ctx, projectID); err != nil {
			return fmt.Errorf("open project read session: %w", err)
		}
		s.draft = draft.New(tx)
	}
	return fn(s)
}

// NotInReleaseError reports missing content without substituting working rows.
type NotInReleaseError struct{ ProjectID, Version, EntityID string }

func (e *NotInReleaseError) Error() string {
	return fmt.Sprintf("%s is not in release %s@%s", e.EntityID, e.ProjectID, e.Version)
}

// IsNotInRelease selects the canonical API error envelope.
func (e *NotInReleaseError) IsNotInRelease() bool { return true }
