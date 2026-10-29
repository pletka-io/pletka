package ontologysnapshot

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	readsql "github.com/pletka-io/pletka/pkg/database/readsql/ontologysnapshot"
)

// Factory constructs snapshot sessions. Authorization remains the caller's job.
type Factory struct{ pool *pgxpool.Pool }

// NewFactory separates connection ownership from the bound reader capability.
func NewFactory(pool *pgxpool.Pool) *Factory { return &Factory{pool: pool} }

// Read validates one exact retained snapshot before entering a read-only callback.
func (f *Factory) Read(ctx context.Context, ref Reference, fn func(Reader) error) (err error) {
	if f == nil || f.pool == nil || ref.digest == "" || fn == nil {
		return fmt.Errorf("legacy snapshot session requires a reference and callback")
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
	if _, err := r.Snapshot(ctx); err != nil {
		return err
	}
	return fn(r)
}
