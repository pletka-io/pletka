//go:build integration

package override

import (
	"context"
	"testing"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/domain"
)

func TestPlacementEditsAreChangeLogged(t *testing.T) {
	ctx := writeCtx()
	pool := testdb.Pool(t)
	runner := &testChangeLogRunner{pool: pool}
	svc := NewService(NewPostgresStore(pool), nil, runner)
	p := &domain.CollectionPlacement{ProjectID: "LA", ModelID: "TSTPLACE.1", CategoryID: "", CollectionID: "LAC.1"}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM weave_collection_placements WHERE model_id = 'TSTPLACE.1'`)
	})

	entries := func() []sqlcgen.WeaveChangeLog {
		rows, err := sqlcgen.New(pool).WeaveListChangeLogForChangeSet(context.Background(), runner.lastChangeSetID)
		if err != nil {
			t.Fatal(err)
		}
		return rows
	}

	if err := svc.UpsertPlacement(ctx, p); err != nil {
		t.Fatalf("create: %v", err)
	}
	if e := entries(); len(e) != 1 || e[0].EntityType != "collection_placement" || e[0].EntityID != "TSTPLACE.1" || e[0].Operation != "create" {
		t.Fatalf("create entries: %+v", e)
	}
	one := 1
	p.MaxOccurs = &one
	if err := svc.UpsertPlacement(ctx, p); err != nil {
		t.Fatalf("update: %v", err)
	}
	if e := entries(); len(e) != 1 || e[0].Operation != "update" {
		t.Fatalf("update entries: %+v", e)
	}
	if err := svc.DeletePlacement(ctx, "LA", "TSTPLACE.1", "", "LAC.1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if e := entries(); len(e) != 1 || e[0].Operation != "delete" {
		t.Fatalf("delete entries: %+v", e)
	}
}
