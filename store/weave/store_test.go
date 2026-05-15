package weave

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestNewRejectsNilPool(t *testing.T) {
	store, err := New(nil)
	if err == nil {
		t.Fatalf("New(nil) error = nil; want error")
	}
	if store != nil {
		t.Fatalf("New(nil) store = %#v; want nil", store)
	}
}

func TestNewExposesPoolAndQueries(t *testing.T) {
	pool := new(pgxpool.Pool)

	store, err := New(pool)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if store.Pool() != pool {
		t.Fatalf("Pool() did not return original pool")
	}
	if store.Queries() == nil {
		t.Fatalf("Queries() = nil; want query facade")
	}
}

func TestNilStoreGuards(t *testing.T) {
	var store *Store

	if store.Pool() != nil {
		t.Fatalf("nil Pool() != nil")
	}
	if store.Queries() != nil {
		t.Fatalf("nil Queries() != nil")
	}
	if err := store.Ping(context.Background()); err == nil {
		t.Fatalf("nil Ping() error = nil; want error")
	}
	if err := store.InTx(context.Background(), nil); err == nil {
		t.Fatalf("nil InTx() error = nil; want error")
	}
	store.Close()
}
