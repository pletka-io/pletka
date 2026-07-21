//go:build integration

package publication

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://postgres:pw123@localhost:5433/pletka_weave?sslmode=disable"
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Skipf("database not available: %v", err)
	}
	if err := pool.Ping(context.Background()); err != nil {
		t.Skipf("database not reachable: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return pool
}

// seedPublicationFixture builds a released project TEST_PUB with fields/model in
// each publication state, plus an unreleased project TEST_PUB_NOREL. All
// timestamps are explicit so the compare is deterministic:
//
//	release created_at = 2026-01-01
//	"before"           = 2025-12-01  (published)
//	"after"            = 2026-02-01  (modified / new)
func seedPublicationFixture(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	cleanup := func() {
		for _, tbl := range []string{
			"weave_field_overrides", "weave_fields_archive", "weave_fields",
			"weave_models_archive", "weave_models", "weave_releases", "weave_projects",
		} {
			_, _ = pool.Exec(ctx, "DELETE FROM "+tbl+" WHERE project_id LIKE 'TEST_PUB%'")
		}
		_, _ = pool.Exec(ctx, "DELETE FROM weave_actors WHERE id = 'TEST_PUB_OWNER'")
	}
	cleanup()
	t.Cleanup(cleanup)

	must := func(sql string, args ...any) {
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("seed exec failed: %v\nSQL: %s", err, sql)
		}
	}

	must(`INSERT INTO weave_actors (id, type, display_name, slug, visibility) VALUES ('TEST_PUB_OWNER','organization','Pub Org','pub-org','private') ON CONFLICT DO NOTHING`)
	must(`INSERT INTO weave_projects (id, ui_name, status, owner_id, visibility) VALUES ('TEST_PUB','{"en":"Pub"}','draft','TEST_PUB_OWNER','private') ON CONFLICT DO NOTHING`)
	must(`INSERT INTO weave_projects (id, ui_name, status, owner_id, visibility) VALUES ('TEST_PUB_NOREL','{"en":"NoRel"}','draft','TEST_PUB_OWNER','private') ON CONFLICT DO NOTHING`)
	must(`INSERT INTO weave_releases (project_id, version, title, created_by_id, created_at) VALUES ('TEST_PUB','0.1.0','baseline','TEST_PUB_OWNER','2026-01-01T00:00:00Z') ON CONFLICT DO NOTHING`)

	// Live fields.
	must(`INSERT INTO weave_fields (id, project_id, version_number, updated_at) VALUES
		('F_PUB','TEST_PUB','','2025-12-01T00:00:00Z'),
		('F_MOD','TEST_PUB','','2026-02-01T00:00:00Z'),
		('F_MODOVR','TEST_PUB','','2025-12-01T00:00:00Z'),
		('F_NEW','TEST_PUB','','2026-02-01T00:00:00Z'),
		('F_NOREL','TEST_PUB_NOREL','','2026-02-01T00:00:00Z')`)
	// Archive rows (present in the 0.1.0 release) — all but F_NEW.
	must(`INSERT INTO weave_fields_archive (id, project_id, version_number) VALUES
		('F_PUB','TEST_PUB','0.1.0'),
		('F_MOD','TEST_PUB','0.1.0'),
		('F_MODOVR','TEST_PUB','0.1.0')`)
	// F_MODOVR is unchanged itself but its base override was edited after release.
	must(`INSERT INTO weave_field_overrides (field_id, project_id, entity_type, updated_at) VALUES
		('F_PUB','TEST_PUB','','2025-12-01T00:00:00Z'),
		('F_MODOVR','TEST_PUB','','2026-02-01T00:00:00Z')`)

	// A model modified via a model-context override (exercises entity_type='model').
	must(`INSERT INTO weave_models (id, project_id, version_number, updated_at) VALUES ('M_MOD','TEST_PUB','','2025-12-01T00:00:00Z')`)
	must(`INSERT INTO weave_models_archive (id, project_id, version_number) VALUES ('M_MOD','TEST_PUB','0.1.0')`)
	must(`INSERT INTO weave_field_overrides (field_id, project_id, entity_type, entity_id, updated_at) VALUES ('F_PUB','TEST_PUB','model','M_MOD','2026-02-01T00:00:00Z')`)
}

func TestBatchState(t *testing.T) {
	pool := testPool(t)
	seedPublicationFixture(t, pool)
	r := NewReader(pool)
	ctx := context.Background()

	got, err := r.BatchState(ctx, "TEST_PUB", "field", []string{"F_PUB", "F_MOD", "F_MODOVR", "F_NEW"})
	if err != nil {
		t.Fatalf("BatchState field: %v", err)
	}
	want := map[string]State{"F_PUB": StatePublished, "F_MOD": StateModified, "F_MODOVR": StateModified, "F_NEW": StateNew}
	for id, w := range want {
		if got[id] != w {
			t.Errorf("field %s: got %q want %q", id, got[id], w)
		}
	}

	// No-release project → draft.
	nr, err := r.BatchState(ctx, "TEST_PUB_NOREL", "field", []string{"F_NOREL"})
	if err != nil {
		t.Fatalf("BatchState norel: %v", err)
	}
	if nr["F_NOREL"] != StateDraft {
		t.Errorf("F_NOREL: got %q want draft", nr["F_NOREL"])
	}

	// Model modified via model-context override.
	m, err := r.BatchState(ctx, "TEST_PUB", "model", []string{"M_MOD"})
	if err != nil {
		t.Fatalf("BatchState model: %v", err)
	}
	if m["M_MOD"] != StateModified {
		t.Errorf("M_MOD: got %q want modified", m["M_MOD"])
	}
}

func TestProjectRollup(t *testing.T) {
	pool := testPool(t)
	seedPublicationFixture(t, pool)
	r := NewReader(pool)

	got, err := r.ProjectRollup(context.Background(), "TEST_PUB")
	if err != nil {
		t.Fatalf("ProjectRollup: %v", err)
	}
	// Fields: F_MOD + F_MODOVR modified, F_NEW new. Model: M_MOD modified.
	if got.Modified != 3 {
		t.Errorf("modified: got %d want 3", got.Modified)
	}
	if got.New != 1 {
		t.Errorf("new: got %d want 1", got.New)
	}
	if got.Unreleased() != 4 {
		t.Errorf("unreleased: got %d want 4", got.Unreleased())
	}
}
