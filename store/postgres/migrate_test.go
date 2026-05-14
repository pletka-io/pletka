package postgres

import (
	"math"
	"testing"

	"github.com/pressly/goose/v3"

	"github.com/pletka-io/pletka/store/migrations"
)

func TestEmbeddedMigrationsCollect(t *testing.T) {
	goose.SetBaseFS(migrations.FS)

	migs, err := goose.CollectMigrations(migrationsDir, 0, math.MaxInt64)
	if err != nil {
		t.Fatalf("CollectMigrations() error = %v", err)
	}
	if len(migs) != 1 {
		t.Fatalf("migration count = %d; want 1", len(migs))
	}
	if got := migs[0].Version; got != 1 {
		t.Fatalf("migration version = %d; want 1", got)
	}
}
