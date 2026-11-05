//go:build integration

package testdb_test

import (
	"testing"

	"github.com/pletka-io/pletka/internal/testdb"
)

func TestArchiveDraftCopiesEveryRegistrySection(t *testing.T) {
	p := testdb.EmptyPool(t)
	ctx := t.Context()
	for _, sql := range []string{
		`INSERT INTO weave_actors(id,type,slug,display_name) VALUES ('ad-owner','organization','ad-owner','Owner')`,
		`INSERT INTO weave_projects(id,owner_id,ui_name) VALUES ('ad','ad-owner','{"en":"Draft"}')`,
		`INSERT INTO weave_models(id,project_id) VALUES ('ad-m','ad')`,
		`INSERT INTO weave_fields(id,project_id) VALUES ('ad-f','ad')`,
		`INSERT INTO weave_field_overrides(field_id,project_id,entity_type,entity_id) VALUES ('ad-f','ad','model','ad-m')`,
		`INSERT INTO weave_releases(project_id,version,created_by_id) VALUES ('ad','0.1.0','ad-owner')`,
		`INSERT INTO weave_projects_archive(id,owner_id,version_number,ui_name) VALUES ('ad','ad-owner','0.1.0','{"en":"Kept"}')`,
	} {
		if _, err := p.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	testdb.ArchiveDraft(t, p, "ad", "0.1.0")
	var models, fields, overrides int
	var title string
	row := p.QueryRow(ctx, `SELECT (SELECT count(*) FROM weave_models_archive WHERE project_id='ad' AND version_number='0.1.0'),
		(SELECT count(*) FROM weave_fields_archive WHERE project_id='ad' AND version_number='0.1.0'),
		(SELECT count(*) FROM weave_field_overrides_archive WHERE project_id='ad' AND version_number='0.1.0'),
		(SELECT ui_name->>'en' FROM weave_projects_archive WHERE id='ad' AND version_number='0.1.0')`)
	if err := row.Scan(&models, &fields, &overrides, &title); err != nil {
		t.Fatal(err)
	}
	if models != 1 || fields != 1 || overrides != 1 {
		t.Fatalf("archived models=%d fields=%d overrides=%d", models, fields, overrides)
	}
	if title != "Kept" {
		t.Fatalf("pre-existing archive row replaced: %q", title)
	}
	testdb.ArchiveDraft(t, p, "ad", "0.1.0") // idempotent
}
