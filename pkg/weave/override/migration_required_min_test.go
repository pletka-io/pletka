//go:build integration

package override

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/pletka-io/pletka/internal/testdb"
)

// TestMigrationSyncsRequiredAndMin seeds overrides and placements where
// Required and Min disagree, runs migration 024's Up SQL and checks that
// every row now has Required <=> Min >= 1, the stricter side winning.
func TestMigrationSyncsRequiredAndMin(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()

	raw, err := os.ReadFile("../../database/migrations/024_required_min_sync.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	up, _, ok := strings.Cut(string(raw), "-- +goose Down")
	if !ok {
		t.Fatal("migration has no Down section")
	}

	const project = "TSTRQMIN"
	seed := []string{
		`INSERT INTO weave_field_overrides (field_id, project_id, entity_type, entity_id, is_required, min_occurs) VALUES
			('TSTRQMIN.F.1','` + project + `','model','TSTRQMIN.M.1', false, 1),
			('TSTRQMIN.F.2','` + project + `','model','TSTRQMIN.M.1', true, 0),
			('TSTRQMIN.F.3','` + project + `','model','TSTRQMIN.M.1', NULL, NULL),
			('TSTRQMIN.F.4','` + project + `','model','TSTRQMIN.M.1', false, 3)`,
		`INSERT INTO weave_collection_placements (project_id, model_id, category_id, collection_id, is_required, min_occurs) VALUES
			('` + project + `','TSTRQMIN.M.1','','TSTRQMIN.C.1', false, 2),
			('` + project + `','TSTRQMIN.M.1','','TSTRQMIN.C.2', true, 0),
			('` + project + `','TSTRQMIN.M.1','','TSTRQMIN.C.3', false, 0)`,
	}
	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = pool.Exec(ctx, `DELETE FROM weave_field_overrides WHERE project_id = '`+project+`'`)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_collection_placements WHERE project_id = '`+project+`'`)
	})
	for _, q := range seed {
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatalf("seed: %v\n%s", err, q)
		}
	}

	if _, err := pool.Exec(ctx, up); err != nil {
		t.Fatalf("run migration Up: %v", err)
	}

	rows, err := pool.Query(ctx, `
		SELECT field_id, COALESCE(is_required, false), COALESCE(min_occurs, 0) FROM weave_field_overrides WHERE project_id = $1
		UNION ALL
		SELECT collection_id, is_required, min_occurs FROM weave_collection_placements WHERE project_id = $1
		ORDER BY 1`, project)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var id string
		var required bool
		var minOccurs int
		if err := rows.Scan(&id, &required, &minOccurs); err != nil {
			t.Fatalf("scan: %v", err)
		}
		r := "optional"
		if required {
			r = "required"
		}
		got = append(got, id+" "+r+" "+string(rune('0'+minOccurs)))
	}
	want := []string{
		"TSTRQMIN.C.1 required 2",
		"TSTRQMIN.C.2 required 1",
		"TSTRQMIN.C.3 optional 0",
		"TSTRQMIN.F.1 required 1",
		"TSTRQMIN.F.2 required 1",
		"TSTRQMIN.F.3 optional 0",
		"TSTRQMIN.F.4 required 3",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("after migration:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}
