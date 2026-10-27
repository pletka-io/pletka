package ontologyrelease

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	readsql "github.com/pletka-io/pletka/pkg/database/readsql/projectontologies"
)

// ReadProject binds a released project's captured ontology inventory. This has
// no draft fallback. Values may escape; the reader cannot outlive its callback.
func (f *Factory) ReadProject(ctx context.Context, project, version string, fn func(ProjectReader) error) (err error) {
	if f == nil || f.pool == nil || strings.TrimSpace(project) == "" || strings.TrimSpace(version) == "" || fn == nil {
		return fmt.Errorf("project ontology session requires a project and concrete release")
	}
	tx, err := f.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return err
	}
	r := &projectReader{queries: readsql.New(tx), project: project, version: version}
	defer func() {
		r.closed = true
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if closeErr := tx.Rollback(cleanup); closeErr != nil && !errors.Is(closeErr, pgx.ErrTxClosed) {
			err = errors.Join(err, closeErr)
		}
	}()
	verified, err := r.queries.ProjectOntologyInventory(ctx, readsql.ProjectOntologyInventoryParams{ProjectID: project, Version: version})
	if errors.Is(err, pgx.ErrNoRows) || err == nil && !verified {
		return &projectMissingError{project: project, version: version}
	}
	if err != nil {
		return err
	}
	return fn(r)
}

type projectMissingError struct{ project, version string }

func (e *projectMissingError) Error() string {
	return fmt.Sprintf("project %s@%s has no verified ontology inventory", e.project, e.version)
}
func (e *projectMissingError) IsNotInRelease() bool { return true }
