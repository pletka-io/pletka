package ontologysnapshot

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	readsql "github.com/pletka-io/pletka/pkg/database/readsql/importedontologysnapshot"
)

// ReadImported binds exact imported evidence, independently of local release
// captures. It never substitutes a captured snapshot, or establishes a release
// completeness certificate. Authorization remains the caller's responsibility.
func (f *Factory) ReadImported(ctx context.Context, ref Reference, fn func(Reader) error) (err error) {
	if f == nil || f.pool == nil || ref.Digest() == "" || fn == nil {
		return fmt.Errorf("imported snapshot session requires a reference and callback")
	}
	tx, err := f.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return err
	}
	r := &importedReader{queries: readsql.New(tx), ref: ref}
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
