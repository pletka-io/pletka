package txscope

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Run owns one repeatable-read operation, or a savepoint in an existing owner.
// All reads and stores receive the same bound context. Success never commits an
// enclosing capture transaction. Callbacks must not perform external effects.
func Run(ctx context.Context, pool *pgxpool.Pool, fn func(context.Context) error) (err error) {
	if pool == nil || fn == nil {
		return fmt.Errorf("transaction runner requires a pool and callback")
	}
	_, scoped, err := Current(ctx, pool)
	if err != nil {
		return err
	}
	tx, err := New(pool).BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if closeErr := tx.Rollback(cleanup); closeErr != nil && !errors.Is(closeErr, pgx.ErrTxClosed) {
			err = errors.Join(err, closeErr)
		}
	}()
	if scoped {
		err = fn(ctx)
	} else {
		err = Within(ctx, pool, tx, fn)
	}
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
