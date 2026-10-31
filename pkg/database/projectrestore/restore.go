package projectrestore

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/pkg/database/advisorylock"
	"github.com/pletka-io/pletka/pkg/database/ontologyrelease"
	"github.com/pletka-io/pletka/pkg/database/ontologysnapshot"
	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/projectbundle"
)

// Result describes committed writes. A repeated exact import leaves edited
// working copies intact. DetachedStagingRefs counts instance-local FK attachments
// cleared only in working copies; archives retain their original staging IDs.
type Result struct {
	Committed           bool
	Releases            int
	WorkingCopies       int
	DetachedStagingRefs int64
}

// Restore imports selected immutable histories and creates their working copies
// atomically. Targets must be absent or exact prior imports; existing working
// copies are never overwritten. Root then source locks are acquired on one
// connection before any write, and held until after commit/rollback. Callers must
// authorize the operation and must not already hold a restore lock for this root.
func Restore(ctx context.Context, pool *pgxpool.Pool, imported projectbundle.ImportedProject, catalog projectbundle.Catalog) (result Result, err error) {
	p, err := prepare(imported, catalog)
	if err != nil {
		return result, err
	}
	if pool == nil {
		return result, fmt.Errorf("project import database is unavailable")
	}
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return result, err
	}
	defer conn.Release()
	unlock, err := lockProjects(ctx, conn, p)
	if err != nil {
		return result, err
	}
	defer func() { err = errors.Join(err, unlock()) }()
	tx, err := conn.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return result, err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(cleanup)
	}()
	defer func() {
		var pgerr *pgconn.PgError
		if errors.As(err, &pgerr) && strings.Contains(pgerr.ConstraintName, "conflict") {
			err = &ConflictError{cause: err}
		}
	}()
	pending, err := restore(ctx, tx, p)
	if err != nil {
		return result, err
	}
	if err := tx.Commit(ctx); err != nil {
		return result, err
	}
	pending.Committed = true
	return pending, nil
}

// ConflictError reports a retained identity/content collision, not permission to
// overwrite the existing project, history or ontology artifact.
type ConflictError struct{ cause error }

func (e *ConflictError) Error() string {
	return "project history import conflicts with retained content: " + e.cause.Error()
}
func (e *ConflictError) Unwrap() error { return e.cause }

// ConflictMessage selects the canonical API conflict response.
func (e *ConflictError) ConflictMessage() string { return e.Error() }

func restore(ctx context.Context, tx pgx.Tx, p prepared) (Result, error) {
	result := Result{}
	// Both helpers share this transaction. A later project/receipt failure rolls
	// back newly installed ontology data as well as source/root project data.
	if err := ontologyrelease.RestoreTx(ctx, tx, p.publications); err != nil {
		return result, err
	}
	if err := ontologysnapshot.RestoreTx(ctx, tx, p.legacy); err != nil {
		return result, err
	}
	q := sqlcgen.New(tx)
	created := make(map[string]bool, len(p.projects))
	// Seed absent shared actors from the root context first. Historical source
	// descriptors remain exact and never depend on this mutable actor record.
	shells := append([]project{p.projects[len(p.projects)-1]}, p.projects[:len(p.projects)-1]...)
	for _, project := range shells {
		fresh, err := q.WeavePrepareProjectImport(ctx, project.document)
		if err != nil {
			return result, err
		}
		created[project.id.ProjectID()] = fresh
	}
	for _, project := range p.projects {
		if !project.id.IsDraft() {
			fresh, err := q.WeaveImportProjectRelease(ctx, project.document)
			if err != nil {
				return result, err
			}
			if fresh {
				result.Releases++
			}
		}
		if created[project.id.ProjectID()] {
			detached, err := q.WeaveRestoreProjectWorkspace(ctx, project.document)
			if err != nil {
				return result, err
			}
			result.WorkingCopies++
			result.DetachedStagingRefs += detached
		}
	}
	return result, nil
}

func lockProjects(ctx context.Context, conn *pgxpool.Conn, p prepared) (func() error, error) {
	q := sqlcgen.New(conn)
	previous, err := q.WeaveProjectImportLockTimeout(ctx)
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(p.projects))
	for _, project := range p.projects {
		if project.id.ProjectID() != p.root {
			keys = append(keys, project.id.ProjectID())
		}
	}
	slices.Sort(keys)
	keys = append([]string{p.root}, keys...)
	held := []string{}
	cleanup := func() error {
		var failure error
		for i := len(held) - 1; i >= 0; i-- {
			if err := advisorylock.Release(ctx, conn, advisorylock.Exclusive, held[i], advisorylock.GraceTimeout); err != nil {
				failure = errors.Join(failure, err)
				break
			}
		}
		clean, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if !conn.Conn().IsClosed() {
			failure = errors.Join(failure, q.WeaveSetProjectImportLockTimeout(clean, previous))
		}
		if failure != nil {
			_ = conn.Conn().Close(clean)
		}
		return failure
	}
	if err := q.WeaveSetProjectImportLockTimeout(ctx, "30s"); err != nil {
		return nil, errors.Join(err, cleanup())
	}
	for _, project := range keys {
		key := advisorylock.ProjectLockKey(project)
		if err := advisorylock.Acquire(ctx, conn, advisorylock.Exclusive, key); err != nil {
			return nil, errors.Join(err, cleanup())
		}
		held = append(held, key)
	}
	return cleanup, nil
}
