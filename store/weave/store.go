package weave

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/store/postgres"
	"github.com/pletka-io/pletka/store/sqlcgen"
)

// Store owns the pgx-backed query layer for Pletka weave data.
type Store struct {
	pool    *pgxpool.Pool
	queries *sqlcgen.Queries
}

// Open creates a pgx pool from config and wraps it in a Store.
func Open(ctx context.Context, cfg postgres.Config) (*Store, error) {
	pool, err := postgres.OpenPool(ctx, cfg)
	if err != nil {
		return nil, err
	}
	store, err := New(pool)
	if err != nil {
		pool.Close()
		return nil, err
	}
	return store, nil
}

// New wraps an existing pgx pool. The caller keeps ownership of closing it
// through Store.Close.
func New(pool *pgxpool.Pool) (*Store, error) {
	if pool == nil {
		return nil, errors.New("weave store requires a pgx pool")
	}
	return &Store{
		pool:    pool,
		queries: sqlcgen.New(pool),
	}, nil
}

// Pool returns the underlying pgx pool for slice stores that need raw SQL.
func (s *Store) Pool() *pgxpool.Pool {
	if s == nil {
		return nil
	}
	return s.pool
}

// Queries returns the generated sqlc query facade.
func (s *Store) Queries() *sqlcgen.Queries {
	if s == nil {
		return nil
	}
	return s.queries
}

// Ping verifies the backing database is reachable.
func (s *Store) Ping(ctx context.Context) error {
	if s == nil || s.pool == nil {
		return errors.New("weave store is not initialized")
	}
	return s.pool.Ping(ctx)
}

// Close closes the backing pool. It is safe to call on a nil store.
func (s *Store) Close() {
	if s == nil || s.pool == nil {
		return
	}
	s.pool.Close()
}

// InTx runs fn with a sqlc query facade bound to one transaction.
func (s *Store) InTx(ctx context.Context, fn func(*sqlcgen.Queries) error) error {
	if s == nil || s.pool == nil || s.queries == nil {
		return errors.New("weave store is not initialized")
	}
	if fn == nil {
		return errors.New("transaction callback is nil")
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin weave transaction: %w", err)
	}
	defer rollback(ctx, tx)

	if err := fn(s.queries.WithTx(tx)); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit weave transaction: %w", err)
	}
	return nil
}

func rollback(ctx context.Context, tx pgx.Tx) {
	_ = tx.Rollback(ctx)
}
