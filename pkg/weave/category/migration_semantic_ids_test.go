//go:build integration

package category

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/pletka-io/pletka/internal/testdb"
)

// TestMigrationRekeysCategoriesBySemanticID seeds a category keyed by a ULID
// (what the category service created before it set ID), with an override, an
// archive row and an adoption pointing at it, runs migration 022's Up SQL and
// checks that the category and every reference now carry the semantic id.
func TestMigrationRekeysCategoriesBySemanticID(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()

	raw, err := os.ReadFile("../../database/migrations/022_category_ids_semantic.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	up, _, ok := strings.Cut(string(raw), "-- +goose Down")
	if !ok {
		t.Fatal("migration has no Down section")
	}

	const (
		ownerID = "tstcat_owner"
		project = "TSTCAT"
		adopter = "TSTCATB"
		oldID   = "01TSTCAT0000000000000000AA"
		newID   = "TSTCAT.CAT.1"
	)
	seed := []string{
		`INSERT INTO weave_actors (id, display_name, slug) VALUES ('` + ownerID + `','TSTCAT Owner','` + ownerID + `') ON CONFLICT (id) DO NOTHING`,
		`INSERT INTO weave_projects (id, owner_id) VALUES ('` + project + `','` + ownerID + `'),('` + adopter + `','` + ownerID + `') ON CONFLICT (id) DO NOTHING`,
		`INSERT INTO weave_categories (id, semantic_id, system_name, project_id) VALUES ('` + oldID + `','` + newID + `','rights','` + project + `')`,
		`INSERT INTO weave_categories_archive (id, semantic_id, system_name, project_id, version_number) VALUES ('` + oldID + `','` + newID + `','rights','` + project + `','1.0.0')`,
		`INSERT INTO weave_field_overrides (field_id, project_id, category_id) VALUES ('TSTCATF.1','` + project + `','` + oldID + `')`,
		`INSERT INTO weave_adoptions (project_id, context_entity_type, context_entity_id, entity_type, source_project_id, source_entity_id)
		 VALUES ('` + adopter + `','project','` + adopter + `','category','` + project + `','` + oldID + `')`,
	}
	t.Cleanup(func() {
		ctx := context.Background()
		for _, q := range []string{
			`DELETE FROM weave_adoptions WHERE project_id = '` + adopter + `'`,
			`DELETE FROM weave_field_overrides WHERE project_id = '` + project + `'`,
			`DELETE FROM weave_categories_archive WHERE project_id = '` + project + `'`,
			`DELETE FROM weave_categories WHERE project_id = '` + project + `'`,
			`DELETE FROM weave_projects WHERE id IN ('` + project + `','` + adopter + `')`,
			`DELETE FROM weave_actors WHERE id = '` + ownerID + `'`,
		} {
			_, _ = pool.Exec(ctx, q)
		}
	})
	for _, q := range seed {
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatalf("seed: %v\n%s", err, q)
		}
	}

	if _, err := pool.Exec(ctx, up); err != nil {
		t.Fatalf("run migration Up: %v", err)
	}

	for name, q := range map[string]string{
		"category":         `SELECT count(*) FROM weave_categories WHERE id = '` + newID + `'`,
		"category archive": `SELECT count(*) FROM weave_categories_archive WHERE id = '` + newID + `'`,
		"override":         `SELECT count(*) FROM weave_field_overrides WHERE project_id = '` + project + `' AND category_id = '` + newID + `'`,
		"adoption":         `SELECT count(*) FROM weave_adoptions WHERE project_id = '` + adopter + `' AND source_entity_id = '` + newID + `'`,
	} {
		var n int
		if err := pool.QueryRow(ctx, q).Scan(&n); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if n != 1 {
			t.Errorf("%s rows keyed %s = %d, want 1", name, newID, n)
		}
	}

	var fk int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM pg_constraint WHERE conname = 'weave_collections_default_category_fkey'`).Scan(&fk); err != nil {
		t.Fatalf("check fk: %v", err)
	}
	if fk != 1 {
		t.Errorf("weave_collections_default_category_fkey count = %d, want 1 (dropped and restored)", fk)
	}
}
