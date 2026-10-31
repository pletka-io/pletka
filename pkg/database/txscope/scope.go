// Package txscope lets store adapters join an infrastructure-owned transaction.
// It does not open transactions implicitly or select a project/actor policy.
package txscope

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type key struct{}
type scope struct {
	pool   *pgxpool.Pool
	tx     pgx.Tx
	closed atomic.Bool
}

// Within binds tx for the callback only. The caller owns commit/rollback. A
// different pool or nested owner fails; nested stores use Begin savepoints.
func Within(ctx context.Context, pool *pgxpool.Pool, tx pgx.Tx, fn func(context.Context) error) error {
	if pool == nil || tx == nil || fn == nil {
		return fmt.Errorf("transaction scope is not configured")
	}
	if ctx.Value(key{}) != nil {
		return fmt.Errorf("transaction scope already has an owner")
	}
	s := &scope{pool: pool, tx: tx}
	defer func() { s.closed.Store(true) }()
	return fn(context.WithValue(ctx, key{}, s))
}

// Current is reserved for transaction-owning infrastructure and read-session
// binding. Store adapters use New; they never receive the parent's commit handle.
func Current(ctx context.Context, pool *pgxpool.Pool) (pgx.Tx, bool, error) {
	s, ok := ctx.Value(key{}).(*scope)
	if !ok {
		return nil, false, nil
	}
	if s.closed.Load() || s.pool != pool {
		return nil, true, fmt.Errorf("transaction scope is closed or belongs to another pool")
	}
	return s.tx, true, nil
}

// DB routes sqlc and direct pgx calls through the context's transaction. Without
// a scope it preserves ordinary pool behavior. It exposes no Conn/Acquire escape.
type DB struct{ pool *pgxpool.Pool }

// New returns a context-aware adapter for this pool.
func New(pool *pgxpool.Pool) *DB { return &DB{pool: pool} }

// Exec executes on the scoped transaction, or the pool when unscoped.
func (d *DB) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	tx, scoped, err := Current(ctx, d.pool)
	if err != nil {
		return pgconn.CommandTag{}, err
	}
	if scoped {
		return tx.Exec(ctx, sql, args...)
	}
	return d.pool.Exec(ctx, sql, args...)
}

// Query queries on the scoped transaction, or the pool when unscoped.
func (d *DB) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	tx, scoped, err := Current(ctx, d.pool)
	if err != nil {
		return nil, err
	}
	if scoped {
		return tx.Query(ctx, sql, args...)
	}
	return d.pool.Query(ctx, sql, args...)
}

type errorRow struct{ err error }

func (r errorRow) Scan(...any) error { return r.err }

// QueryRow queries one row using the scoped transaction when present.
func (d *DB) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	tx, scoped, err := Current(ctx, d.pool)
	if err != nil {
		return errorRow{err}
	}
	if scoped {
		return tx.QueryRow(ctx, sql, args...)
	}
	return d.pool.QueryRow(ctx, sql, args...)
}

// Begin starts a child savepoint when scoped, otherwise a pool transaction.
func (d *DB) Begin(ctx context.Context) (pgx.Tx, error) { return d.BeginTx(ctx, pgx.TxOptions{}) }

// BeginTx starts a pool transaction; scoped calls require zero options and use a savepoint.
func (d *DB) BeginTx(ctx context.Context, opts pgx.TxOptions) (pgx.Tx, error) {
	tx, scoped, err := Current(ctx, d.pool)
	if err != nil {
		return nil, err
	}
	if !scoped {
		return d.pool.BeginTx(ctx, opts)
	}
	if opts != (pgx.TxOptions{}) {
		return nil, fmt.Errorf("nested store cannot change transaction options")
	}
	return tx.Begin(ctx)
}
