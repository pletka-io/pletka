//go:build integration

package gitmaterializer

import (
	"context"
	"testing"
)

// A tree written before receipts recorded their source release carries
// receipts without a version; restoring it fills them as the store does.
func TestHydrateAdoptionsFillsAMissingSourceVersion(t *testing.T) {
	pool := hydrateTestPool(t)
	ctx := context.Background()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	t.Cleanup(func() {
		for _, s := range []string{
			`DELETE FROM weave_adoptions WHERE project_id = 'RHAUSE'`,
			`DELETE FROM weave_models_archive WHERE project_id = 'RHASRC'`,
			`DELETE FROM weave_releases WHERE project_id = 'RHASRC'`,
			`DELETE FROM weave_projects WHERE id IN ('RHASRC', 'RHAUSE')`,
			`DELETE FROM weave_actors WHERE id = 'RHA_OWNER'`,
		} {
			_, _ = pool.Exec(context.Background(), s)
		}
	})
	exec(`INSERT INTO weave_actors (id, type, display_name, system_name, slug) VALUES ('RHA_OWNER', 'person', 'rha', 'rha', 'rha')`)
	for _, p := range []string{"RHASRC", "RHAUSE"} {
		exec(`INSERT INTO weave_projects (id, owner_id, system_name) VALUES ($1, 'RHA_OWNER', lower($1))`, p)
	}
	exec(`INSERT INTO weave_releases (project_id, version, created_by_id) VALUES ('RHASRC', '1.0.0', 'RHA_OWNER')`)
	exec(`INSERT INTO weave_models_archive (id, project_id, version_number) VALUES ('RHASRCM.1', 'RHASRC', '1.0.0')`)
	set := &AdoptionManifestSet{
		Index: AdoptionsIndexManifest{SchemaVersion: 1, Receipts: []AdoptionsIndexReceiptEntry{{EntityType: "model", EntityID: "RHASRCM.1", File: "model-RHASRCM.1.yaml"}}},
		Receipts: map[string]AdoptionReceiptManifest{
			"model-RHASRCM.1.yaml": {SchemaVersion: 1, Adoption: AdoptionReceiptRecord{
				EntityType: "model", EntityID: "RHASRCM.1",
				Source:   AdoptionReceiptSource{ProjectID: "RHASRC", EntityID: "RHASRCM.1"},
				Contexts: []AdoptionReceiptContext{{ContextEntityType: "project", ContextEntityID: "RHAUSE"}},
			}},
		},
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := NewMaterializer(pool, t.TempDir(), nil).hydrateAdoptionsTx(ctx, tx, "RHAUSE", set); err != nil {
		t.Fatalf("hydrate: %v", err)
	}
	var got string
	if err := tx.QueryRow(ctx, `SELECT COALESCE(source_version, '') FROM weave_adoptions WHERE project_id = 'RHAUSE'`).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != "1.0.0" {
		t.Fatalf("restored source_version = %q, want 1.0.0", got)
	}
}
