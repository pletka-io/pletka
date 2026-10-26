package ontologyrelease

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	readsql "github.com/pletka-io/pletka/pkg/database/readsql/ontologyrelease"
)

// Factory constructs sealed-release readers. Authorization belongs to its caller;
// a valid reference and an available release never grant access by themselves.
type Factory struct{ pool *pgxpool.Pool }

// NewFactory constructs binding infrastructure, separate from the write store.
func NewFactory(pool *pgxpool.Pool) *Factory { return &Factory{pool: pool} }

// Read validates the sealed release before entering a bounded read-only snapshot.
func (f *Factory) Read(ctx context.Context, ref Reference, fn func(Reader) error) (err error) {
	if f == nil || f.pool == nil || (ref.body.OntologyID() == "" || ref.body.VersionID() == "") || fn == nil {
		return fmt.Errorf("ontology release session requires a validated reference and callback")
	}
	tx, err := f.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return err
	}
	r := &reader{queries: readsql.New(tx), ref: ref}
	defer func() {
		r.closed = true
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if closeErr := tx.Rollback(cleanup); closeErr != nil && !errors.Is(closeErr, pgx.ErrTxClosed) {
			err = errors.Join(err, closeErr)
		}
	}()
	if _, err := r.Artifact(ctx); err != nil {
		return err
	}
	return fn(r)
}
