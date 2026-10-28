//go:build integration

package release

import (
	"errors"
	"testing"
	"time"

	"github.com/pletka-io/pletka/internal/testdb"
)

func TestCutPreservesSupplementalContent(t *testing.T) {
	pool := testdb.EmptyPool(t)
	ctx, owner, project := seedZREL(t, pool)
	service := NewService(NewPostgresStore(pool), pool, nil)
	mustExec(t, pool, `INSERT INTO weave_vocabularies(id,project_id,connector_type) VALUES ('capture-vocab','ZREL','local')`)
	mustExec(t, pool, `INSERT INTO weave_vocabulary_entries(id,vocabulary_id,uri,label) VALUES
		('capture-a','capture-vocab','urn:capture:a','{"en":"A"}'),('capture-b','capture-vocab','urn:capture:b','{"en":"B"}')`)
	mustExec(t, pool, `INSERT INTO weave_concept_lists(id,project_id,vocabulary_id,is_closed) VALUES ('capture-list','ZREL','capture-vocab',true)`)
	mustExec(t, pool, `INSERT INTO weave_concept_list_entries(id,concept_list_id,vocabulary_entry_id) VALUES ('capture-member','capture-list','capture-a')`)
	first := time.Date(2025, 2, 3, 4, 5, 6, 123456000, time.UTC)
	second := first.Add(time.Hour)
	mustExec(t, pool, `INSERT INTO weave_concept_broader(id,concept_id,broader_id,scheme_id,created_at)
		VALUES ('capture-edge','capture-a','capture-b','capture-list',$1)`, first)
	// Global edges have no project/scheme ownership and are not part of a
	// scheme release. Keep that deliberate boundary visible in a real cut.
	mustExec(t, pool, `INSERT INTO weave_concept_broader(id,concept_id,broader_id) VALUES ('global-edge','capture-a','capture-b')`)
	for _, cut := range []struct {
		version, entry string
		enforce        bool
		created        time.Time
	}{
		{"9.0.1", "capture-a", true, first},
		{"9.0.2", "capture-b", false, second},
	} {
		mustExec(t, pool, `UPDATE weave_projects SET enforce_concept_lists=$1 WHERE id='ZREL'`, cut.enforce)
		mustExec(t, pool, `UPDATE weave_field_overrides SET set_value_entry_id=$1 WHERE project_id='ZREL'`, cut.entry)
		mustExec(t, pool, `UPDATE weave_concept_broader SET created_at=$1 WHERE id='capture-edge'`, cut.created)
		if _, err := service.Create(releaseAuthCtx(ctx, owner), project, CreateInput{Version: cut.version}); err != nil {
			t.Fatal(err)
		}
	}
	for _, want := range []struct {
		version, entry string
		enforce        bool
		created        time.Time
	}{
		{"9.0.1", "capture-a", true, first}, {"9.0.2", "capture-b", false, second},
	} {
		var enforce bool
		var entry string
		var created time.Time
		mustScan(t, pool, `SELECT p.enforce_concept_lists,o.set_value_entry_id,b.created_at
			FROM weave_projects_archive p JOIN weave_field_overrides_archive o ON o.project_id=p.id AND o.version_number=p.version_number
			JOIN weave_concept_broader_archive b ON b.scheme_id='capture-list' AND b.version_number=p.version_number
			WHERE p.id='ZREL' AND p.version_number=$1`, []any{want.version}, &enforce, &entry, &created)
		if enforce != want.enforce || entry != want.entry || !created.Equal(want.created) {
			t.Errorf("%s content changed: %v %q %s", want.version, enforce, entry, created)
		}
	}
	var global int
	mustScan(t, pool, `SELECT count(*) FROM weave_concept_broader_archive WHERE id='global-edge'`, nil, &global)
	if global != 0 {
		t.Fatal("global discovery edge acquired scheme ownership")
	}
}

func TestFixedEntryRequiresCapturedDependency(t *testing.T) {
	for _, state := range []string{"missing", "unpublished", "published"} {
		t.Run(state, func(t *testing.T) {
			pool := testdb.EmptyPool(t)
			ctx, owner, project := seedZREL(t, pool)
			service := NewService(NewPostgresStore(pool), pool, nil)
			mustExec(t, pool, `INSERT INTO weave_projects(id,owner_id) VALUES ('fixed-source','ZREL_OWNER')`)
			mustExec(t, pool, `INSERT INTO weave_models(id,project_id) VALUES ('fixed-source-model','fixed-source')`)
			if state != "missing" {
				mustExec(t, pool, `INSERT INTO weave_vocabularies(id,project_id,connector_type) VALUES ('fixed-vocab','fixed-source','local')`)
				mustExec(t, pool, `INSERT INTO weave_vocabulary_entries(id,vocabulary_id,uri,label) VALUES ('fixed-entry','fixed-vocab','urn:fixed:entry','{"en":"Before"}')`)
			}
			if state == "published" {
				if _, err := service.Create(releaseAuthCtx(ctx, owner), "fixed-source", CreateInput{Version: "1.0.0"}); err != nil {
					t.Fatal(err)
				}
			}
			mustExec(t, pool, `UPDATE weave_field_overrides SET set_value_entry_id='fixed-entry' WHERE project_id='ZREL'`)
			_, err := service.Create(releaseAuthCtx(ctx, owner), project, CreateInput{Version: "9.0.1"})
			if state != "published" {
				var invalid *ErrValidation
				if !errors.As(err, &invalid) || len(invalid.Fields["dependencies"]) == 0 {
					t.Fatalf("unavailable fixed entry accepted or misreported: %v", err)
				}
				var remaining int
				mustScan(t, pool, `SELECT (SELECT count(*) FROM weave_releases WHERE project_id='ZREL')+
					(SELECT count(*) FROM weave_projects_archive WHERE id='ZREL')+
					(SELECT count(*) FROM weave_field_overrides_archive WHERE project_id='ZREL')+
					(SELECT count(*) FROM weave_change_set WHERE project_id='ZREL')`, nil, &remaining)
				if remaining != 0 {
					t.Fatalf("failed cut left %d rows", remaining)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			mustExec(t, pool, `UPDATE weave_vocabulary_entries SET label='{"en":"After"}' WHERE id='fixed-entry'`)
			if _, err := service.Create(releaseAuthCtx(ctx, owner), "fixed-source", CreateInput{Version: "2.0.0"}); err != nil {
				t.Fatal(err)
			}
			mustExec(t, pool, `DELETE FROM weave_vocabulary_entries WHERE id='fixed-entry'`)
			var source, version, label, entry string
			mustScan(t, pool, `SELECT d.source_project_id,d.source_version,e.label->>'en',o.set_value_entry_id
				FROM weave_release_dependency_entities d
				JOIN weave_vocabulary_entries_archive e ON e.id=d.entity_id AND e.version_number=d.source_version
				JOIN weave_field_overrides_archive o ON o.project_id=d.project_id AND o.version_number=d.version_number
				WHERE d.project_id='ZREL' AND d.version_number='9.0.1' AND d.entity_type='vocabulary_entry' AND d.entity_id='fixed-entry'`,
				nil, &source, &version, &label, &entry)
			if source != "fixed-source" || version != "1.0.0" || label != "Before" || entry != "fixed-entry" {
				t.Fatalf("unstable fixed entry: %s %s %s %s", source, version, label, entry)
			}
		})
	}
}
