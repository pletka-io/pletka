package weave

import (
	"context"
	"testing"
	"time"

	domainweave "github.com/pletka-io/pletka/domain/weave"
	"github.com/pletka-io/pletka/store/sqlcgen"
)

func TestReleaseStoreNilGuard(t *testing.T) {
	ctx := context.Background()
	var store *releaseStore

	if _, err := store.ListByProject(ctx, "TPC"); err == nil {
		t.Fatalf("nil ListByProject() error = nil; want error")
	}
	if _, err := store.Get(ctx, "TPC", "v1"); err == nil {
		t.Fatalf("nil Get() error = nil; want error")
	}
}

func TestReleaseFromRow(t *testing.T) {
	createdAt := time.Date(2026, 5, 16, 10, 0, 0, 0, time.UTC)

	release := releaseFromRow(sqlcgen.WeaveRelease{
		ProjectID:   "TPC",
		Version:     "v1",
		Title:       "First release",
		Description: "Frozen test release",
		CreatedAt:   createdAt,
		CreatedByID: "actor-1",
	})

	if release != (domainweave.Release{
		ProjectID:   "TPC",
		Version:     "v1",
		Title:       "First release",
		Description: "Frozen test release",
		CreatedAt:   createdAt,
		CreatedByID: "actor-1",
	}) {
		t.Fatalf("releaseFromRow() = %#v", release)
	}
}
