//go:build integration

package weave_test

import (
	"context"
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave"
)

// A receipt written without a source version gets one: the consumer's pinned
// parent release, else the newest live release of the source holding the
// entity. It stays empty when the consumer follows the source's draft on
// purpose or no release holds the entity.
func TestReplaceForContextFillsTheSourceVersion(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	owner := seedTestActor(t, pool, "rsv-owner")
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	t.Cleanup(func() {
		for _, s := range []string{
			`DELETE FROM weave_adoptions WHERE project_id LIKE 'RSV%'`,
			`DELETE FROM weave_project_inheritance WHERE project_id LIKE 'RSV%'`,
			`DELETE FROM weave_fields_archive WHERE project_id LIKE 'RSV%'`,
			`DELETE FROM weave_releases WHERE project_id LIKE 'RSV%'`,
			`DELETE FROM weave_projects WHERE id LIKE 'RSV%'`,
		} {
			_, _ = pool.Exec(context.Background(), s)
		}
	})
	for _, p := range []string{"RSVSRC", "RSVPIN", "RSVDRAFT", "RSVFREE"} {
		exec(`INSERT INTO weave_projects (id, owner_id, system_name) VALUES ($1, $2, lower($1))`, p, owner)
	}
	// Source releases: 1.0.0 holds F1; 2.0.0 holds F1 and F2; 3.0.0 archived holds F3.
	for _, v := range []string{"1.0.0", "2.0.0", "3.0.0"} {
		exec(`INSERT INTO weave_releases (project_id, version, created_by_id, created_at) VALUES ('RSVSRC', $1, $2, now() - (interval '1 day' * (4 - split_part($1, '.', 1)::int)))`, v, owner)
	}
	exec(`UPDATE weave_releases SET archived_at = now() WHERE project_id = 'RSVSRC' AND version = '3.0.0'`)
	for _, r := range [][2]string{{"RSVSRCF.1", "1.0.0"}, {"RSVSRCF.1", "2.0.0"}, {"RSVSRCF.2", "2.0.0"}, {"RSVSRCF.3", "3.0.0"}} {
		exec(`INSERT INTO weave_fields_archive (id, project_id, version_number) VALUES ($1, 'RSVSRC', $2)`, r[0], r[1])
	}
	exec(`INSERT INTO weave_project_inheritance (project_id, parent_project_id, source_mode, source_version) VALUES ('RSVPIN', 'RSVSRC', 'release', '1.0.0')`)
	exec(`INSERT INTO weave_project_inheritance (project_id, parent_project_id, source_mode) VALUES ('RSVDRAFT', 'RSVSRC', 'draft')`)

	store := weave.NewPostgresStore(pool).Adoptions()
	cases := []struct {
		name, project, entity, want string
	}{
		{"pinned parent release holds it", "RSVPIN", "RSVSRCF.1", "1.0.0"},
		{"pinned release lacks it: newest release holding it", "RSVPIN", "RSVSRCF.2", "2.0.0"},
		{"draft-following link stays empty", "RSVDRAFT", "RSVSRCF.1", ""},
		{"no link: newest release holding it", "RSVFREE", "RSVSRCF.1", "2.0.0"},
		{"only an archived release holds it: empty", "RSVFREE", "RSVSRCF.3", ""},
		{"no release holds it: empty", "RSVFREE", "RSVSRCF.9", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := store.ReplaceForContext(ctx, tc.project, "model", tc.project+"M.1", []domain.Adoption{{
				EntityType: "field", SourceProjectID: "RSVSRC", SourceEntityID: tc.entity, CreatedByID: &owner,
			}}); err != nil {
				t.Fatal(err)
			}
			var got string
			if err := pool.QueryRow(ctx, `SELECT COALESCE(source_version, '') FROM weave_adoptions WHERE project_id = $1 AND source_entity_id = $2`, tc.project, tc.entity).Scan(&got); err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("source_version = %q, want %q", got, tc.want)
			}
		})
	}
	// An explicit version is kept as given.
	if err := store.ReplaceForContext(ctx, "RSVFREE", "model", "RSVFREEM.2", []domain.Adoption{{
		EntityType: "field", SourceProjectID: "RSVSRC", SourceEntityID: "RSVSRCF.1", SourceVersion: "1.0.0", CreatedByID: &owner,
	}}); err != nil {
		t.Fatal(err)
	}
	var kept string
	if err := pool.QueryRow(ctx, `SELECT source_version FROM weave_adoptions WHERE project_id = 'RSVFREE' AND context_entity_id = 'RSVFREEM.2'`).Scan(&kept); err != nil || kept != "1.0.0" {
		t.Fatalf("explicit version = %q (%v), want 1.0.0", kept, err)
	}
}

// A context may hold an older receipt without a version next to a new one
// for the same entity with a version; once the old one is filled they are the
// same receipt and must be stored once, not fail on the unique key.
func TestReplaceForContextStoresAFilledDuplicateOnce(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	owner := seedTestActor(t, pool, "rsvdup-owner")
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	t.Cleanup(func() {
		for _, s := range []string{
			`DELETE FROM weave_adoptions WHERE project_id LIKE 'RSD%'`,
			`DELETE FROM weave_fields_archive WHERE project_id LIKE 'RSD%'`,
			`DELETE FROM weave_releases WHERE project_id LIKE 'RSD%'`,
			`DELETE FROM weave_projects WHERE id LIKE 'RSD%'`,
		} {
			_, _ = pool.Exec(context.Background(), s)
		}
	})
	for _, p := range []string{"RSDSRC", "RSDUSE"} {
		exec(`INSERT INTO weave_projects (id, owner_id, system_name) VALUES ($1, $2, lower($1))`, p, owner)
	}
	exec(`INSERT INTO weave_releases (project_id, version, created_by_id) VALUES ('RSDSRC', '2.0.0', $1)`, owner)
	exec(`INSERT INTO weave_fields_archive (id, project_id, version_number) VALUES ('RSDSRCF.1', 'RSDSRC', '2.0.0')`)
	store := weave.NewPostgresStore(pool).Adoptions()
	if err := store.ReplaceForContext(ctx, "RSDUSE", "project", "RSDUSE", []domain.Adoption{
		{EntityType: "field", SourceProjectID: "RSDSRC", SourceEntityID: "RSDSRCF.1", CreatedByID: &owner},
		{EntityType: "field", SourceProjectID: "RSDSRC", SourceEntityID: "RSDSRCF.1", SourceVersion: "2.0.0", CreatedByID: &owner},
	}); err != nil {
		t.Fatalf("replace: %v", err)
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM weave_adoptions WHERE project_id = 'RSDUSE'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("receipts = %d (%v), want 1", n, err)
	}
}
