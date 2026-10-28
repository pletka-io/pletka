//go:build integration

package release

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/database/readsession"
)

func seedContentChildren(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	for _, sql := range []string{
		`INSERT INTO weave_vocabularies(id,project_id,connector_type) VALUES ('receipt-vocab','ZREL','local')`,
		`INSERT INTO weave_vocabulary_entries(id,vocabulary_id,uri,label) VALUES ('receipt-a','receipt-vocab','urn:a','{"en":"A"}'),('receipt-b','receipt-vocab','urn:b','{"en":"B"}')`,
		`INSERT INTO weave_concept_lists(id,project_id,vocabulary_id) VALUES ('receipt-list','ZREL','receipt-vocab')`,
		`INSERT INTO weave_concept_list_entries(id,concept_list_id,vocabulary_entry_id) VALUES ('receipt-member','receipt-list','receipt-a')`,
		`INSERT INTO weave_concept_broader(id,concept_id,broader_id,scheme_id) VALUES ('receipt-edge','receipt-a','receipt-b','receipt-list')`,
		`INSERT INTO weave_examples(id,project_id,entity_type,entity_id,title,status) VALUES ('receipt-example','ZREL','model','ZRELM.1','{"en":"Example"}','valid')`,
		`INSERT INTO weave_example_values(example_id,field_id,override_id,text_value,value_kind,value_payload) SELECT 'receipt-example','ZRELF1',id,'value','string','{"value":"value"}' FROM weave_field_overrides WHERE project_id='ZREL'`,
		`INSERT INTO weave_override_refs(override_id,ref_type,target_id,semantic_id,position) SELECT id,'concept_list','receipt-list','receipt-list',0 FROM weave_field_overrides WHERE project_id='ZREL'`,
	} {
		mustExec(t, pool, sql)
	}
}

func requireContentRejection(t *testing.T, err error) {
	t.Helper()
	var pgerr *pgconn.PgError
	if !errors.As(err, &pgerr) || (pgerr.Code != "23514" && pgerr.Code != "40001") {
		t.Fatalf("content write escaped guard: %v", err)
	}
}

func TestContentReceiptAndAllArchiveGuards(t *testing.T) {
	pool := testdb.EmptyPool(t)
	ctx, owner, project := seedZREL(t, pool)
	seedContentChildren(t, pool)
	if _, err := NewService(NewPostgresStore(pool), pool, nil).Create(releaseAuthCtx(ctx, owner), project, CreateInput{Version: "9.0.1"}); err != nil {
		t.Fatal(err)
	}
	var saved *readsession.Session
	var receipt readsession.ContentReceipt
	if err := readsession.NewFactory(pool).Read(ctx, auth.Release("9.0.1"), project, func(s *readsession.Session) error {
		saved = s
		var err error
		receipt, err = s.Content(ctx)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	for _, section := range loadCaptureLedger(t).Sections {
		value, ok := receipt.Section(section.ArchiveTable)
		if !ok || len(value.SHA256) != 64 || value.Rows < 0 {
			t.Fatalf("missing section evidence %s: %+v", section.ArchiveTable, value)
		}
	}
	if section, ok := receipt.Section("weave_collections_archive"); !ok || section.Rows != 0 {
		t.Fatal("empty section is not certified")
	}
	if _, ok := receipt.Section("absent"); ok {
		t.Fatal("absent section confused with empty")
	}
	if _, err := saved.Content(ctx); err == nil {
		t.Fatal("closed session accepted")
	}
	if _, err := (&readsession.Session{}).Content(ctx); err == nil {
		t.Fatal("zero session accepted")
	}
	if err := readsession.NewFactory(pool).Read(ctx, auth.Draft(), project, func(s *readsession.Session) error { _, err := s.Content(ctx); return err }); err == nil {
		t.Fatal("draft receipt accepted")
	}

	var overrideID int64
	mustScan(t, pool, `SELECT id FROM weave_field_overrides WHERE project_id='ZREL'`, nil, &overrideID)
	for _, section := range loadCaptureLedger(t).Sections {
		t.Run(section.ArchiveTable, func(t *testing.T) {
			tab := pgx.Identifier{section.ArchiveTable}.Sanitize()
			body := map[string]any{"id": "99", "project_id": project, "release_project_id": project, "version_number": "9.0.1", "concept_list_id": "receipt-list", "scheme_id": "receipt-list", "example_id": "receipt-example", "vocabulary_id": "receipt-vocab", "override_id": overrideID}
			if section.ArchiveTable == "weave_projects_archive" {
				body["id"] = project
			}
			raw, _ := json.Marshal(body)
			_, err := pool.Exec(ctx, "INSERT INTO "+tab+" SELECT (jsonb_populate_record(NULL::"+tab+",$1)).*", raw)
			requireContentRejection(t, err)
			_, err = pool.Exec(ctx, "TRUNCATE "+tab+" CASCADE")
			requireContentRejection(t, err)
			var count int
			mustScan(t, pool, "SELECT count(*) FROM "+tab+" WHERE version_number='9.0.1'", nil, &count)
			if count == 0 {
				return
			} // Insert guards cover deliberately empty sections too.
			_, err = pool.Exec(ctx, "UPDATE "+tab+" SET version_number=version_number WHERE version_number='9.0.1'")
			requireContentRejection(t, err)
			_, err = pool.Exec(ctx, "DELETE FROM "+tab+" WHERE version_number='9.0.1'")
			requireContentRejection(t, err)
			// Move a valid legacy row INTO an immutable release: check NEW ownership.
			mustExec(t, pool, "INSERT INTO "+tab+" SELECT (jsonb_populate_record(NULL::"+tab+",to_jsonb(a)||'{\"version_number\":\"9.9.9\"}'::jsonb)).* FROM "+tab+" a WHERE version_number='9.0.1' LIMIT 1")
			_, err = pool.Exec(ctx, "UPDATE "+tab+" SET version_number='9.0.1' WHERE version_number='9.9.9'")
			requireContentRejection(t, err)
		})
	}
	for _, sql := range []string{
		`UPDATE weave_releases SET content_capture_version=0 WHERE project_id='ZREL'`,
		`UPDATE weave_releases SET content_capture_receipt='{}' WHERE project_id='ZREL'`,
		`UPDATE weave_releases SET content_capture_xid=NULL WHERE project_id='ZREL'`,
		`UPDATE weave_releases SET project_dependencies_version=0 WHERE project_id='ZREL'`,
		`UPDATE weave_releases SET created_by_id='different' WHERE project_id='ZREL'`,
		`UPDATE weave_releases SET created_at=created_at+interval '1 second' WHERE project_id='ZREL'`,
		`DELETE FROM weave_release_content_fences`, `TRUNCATE weave_release_content_fences`,
		`UPDATE weave_release_content_fences SET entity_id='moved'`,
		`INSERT INTO weave_override_refs_archive(override_id,ref_type,position,project_id,version_number) SELECT id,'concept_list',99,'forged-owner','9.0.1' FROM weave_field_overrides WHERE project_id='ZREL'`,
	} {
		_, err := pool.Exec(ctx, sql)
		requireContentRejection(t, err)
	}
	// Catalog policy remains mutable. Release deletion permits archive cleanup.
	mustExec(t, pool, `UPDATE weave_releases SET title='retitled',archived_at=now() WHERE project_id='ZREL'`)
	mustExec(t, pool, `DELETE FROM weave_releases WHERE project_id='ZREL'`)
	mustExec(t, pool, `DELETE FROM weave_concept_list_entries_archive WHERE concept_list_id='receipt-list'`)
	mustExec(t, pool, `DELETE FROM weave_concept_lists_archive WHERE id='receipt-list'`)
	var fences int
	mustScan(t, pool, `SELECT count(*) FROM weave_release_content_fences WHERE entity_id='receipt-list'`, nil, &fences)
	if fences == 0 {
		t.Fatal("cleanup removed serialization identity")
	}
}

func TestContentComparisonRollsBackCorruptCut(t *testing.T) {
	for _, fault := range []string{
		`UPDATE weave_projects_archive SET enforce_concept_lists=NOT enforce_concept_lists WHERE id='ZREL';`,
		`UPDATE weave_fields_archive SET system_name='changed' WHERE project_id='ZREL';`,
		`DELETE FROM weave_fields_archive WHERE project_id='ZREL';`,
		`INSERT INTO weave_fields_archive(id,project_id,version_number) VALUES ('extra','ZREL',NEW.release_version);`,
		`DELETE FROM weave_concept_list_entries_archive WHERE concept_list_id='receipt-list';`,
		`UPDATE weave_concept_broader_archive SET created_at=created_at+interval '1 second' WHERE id='receipt-edge';`,
	} {
		t.Run(fault, func(t *testing.T) {
			pool := testdb.EmptyPool(t)
			ctx, owner, project := seedZREL(t, pool)
			seedContentChildren(t, pool)
			mustExec(t, pool, `CREATE FUNCTION corrupt_content() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN `+fault+` RETURN NEW; END $$`)
			mustExec(t, pool, `CREATE TRIGGER corrupt_content AFTER INSERT ON weave_change_set FOR EACH ROW WHEN (NEW.kind='release') EXECUTE FUNCTION corrupt_content()`)
			_, err := NewService(NewPostgresStore(pool), pool, nil).Create(releaseAuthCtx(ctx, owner), project, CreateInput{Version: "9.0.1"})
			var pgerr *pgconn.PgError
			if !errors.As(err, &pgerr) || pgerr.ConstraintName != "release_content_incomplete" {
				t.Fatalf("corrupt cut not rejected by content comparison: %v", err)
			}
			var count int
			mustScan(t, pool, `SELECT (SELECT count(*) FROM weave_releases)+(SELECT count(*) FROM weave_change_set)+(SELECT count(*) FROM weave_fields_archive)`, nil, &count)
			if count != 0 {
				t.Fatalf("failed cut retained %d rows", count)
			}
		})
	}
}

func TestContentChildRejectsPreCutSnapshot(t *testing.T) {
	for _, sql := range []string{
		`INSERT INTO weave_concept_list_entries_archive(id,concept_list_id,vocabulary_entry_id,version_number) VALUES ('late','receipt-list','receipt-a','9.0.1')`,
		`INSERT INTO weave_vocabulary_entries_archive(id,vocabulary_id,uri,label,version_number) VALUES ('late','receipt-vocab','urn:late','{}','9.0.1')`,
		`INSERT INTO weave_example_values_archive(example_id,field_id,version_number) VALUES ('receipt-example','field','9.0.1')`,
		`TRUNCATE weave_concept_list_entries_archive`,
	} {
		t.Run(sql, func(t *testing.T) {
			pool := testdb.EmptyPool(t)
			ctx, owner, project := seedZREL(t, pool)
			seedContentChildren(t, pool)
			stale, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
			if err != nil {
				t.Fatal(err)
			}
			defer stale.Rollback(context.Background())
			var count int
			if err := stale.QueryRow(ctx, `SELECT count(*) FROM weave_releases`).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if _, err := NewService(NewPostgresStore(pool), pool, nil).Create(releaseAuthCtx(ctx, owner), project, CreateInput{Version: "9.0.1"}); err != nil {
				t.Fatal(err)
			}
			_, err = stale.Exec(ctx, sql)
			requireContentRejection(t, err)
		})
	}
}

func TestContentParentRejectsPreOrphanSnapshot(t *testing.T) {
	pool := testdb.EmptyPool(t)
	ctx, _, _ := seedZREL(t, pool)
	seedContentChildren(t, pool)
	stale, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		t.Fatal(err)
	}
	defer stale.Rollback(context.Background())
	var count int
	if err := stale.QueryRow(ctx, `SELECT count(*) FROM weave_releases`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	mustExec(t, pool, `INSERT INTO weave_concept_list_entries_archive(id,concept_list_id,vocabulary_entry_id,version_number) VALUES ('orphan','receipt-list','receipt-a','9.0.1')`)
	_, err = stale.Exec(ctx, `INSERT INTO weave_concept_lists_archive(id,project_id,version_number) VALUES ('receipt-list','ZREL','9.0.1')`)
	var pgerr *pgconn.PgError
	if !errors.As(err, &pgerr) || pgerr.Code != "40001" {
		t.Fatalf("stale cut could omit committed child: %v", err)
	}
}

func TestContentDoesNotCertifyOldCuts(t *testing.T) {
	pool := testdb.EmptyPool(t)
	ctx, owner, project := seedZREL(t, pool)
	// Simulate a legacy catalog record; the new transaction cannot recertify it.
	mustExec(t, pool, `INSERT INTO weave_releases(project_id,version,created_by_id,archive_ownership_version) VALUES ('ZREL','9.9.9',$1,1)`, owner)
	err := readsession.NewFactory(pool).Read(ctx, auth.Release("9.9.9"), project, func(s *readsession.Session) error { _, err := s.Content(ctx); return err })
	var missing *readsession.NotInReleaseError
	if !errors.As(err, &missing) {
		t.Fatalf("historical receipt = %v", err)
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	_, err = tx.Exec(ctx, `UPDATE weave_releases SET content_capture_version=1 WHERE project_id='ZREL'`)
	requireContentRejection(t, err)
	if err == nil || !strings.Contains(fmt.Sprint(err), "new repeatable-read cut") {
		t.Fatalf("history was not rejected as an old cut: %v", err)
	}
}

func TestContentPrerequisiteEvidenceFreezesBeforeFinalization(t *testing.T) {
	for _, sql := range []string{
		`INSERT INTO weave_release_project_dependencies(project_id,version_number,source_project_id,source_version) VALUES ('ZREL',NEW.release_version,'forged','1.0.0');`,
		`UPDATE weave_releases SET project_dependencies_version=0 WHERE project_id='ZREL';`,
	} {
		t.Run(sql, func(t *testing.T) {
			pool := testdb.EmptyPool(t)
			ctx, owner, project := seedZREL(t, pool)
			mustExec(t, pool, `CREATE FUNCTION change_evidence() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN `+sql+` RETURN NEW; END $$`)
			mustExec(t, pool, `CREATE TRIGGER change_evidence AFTER INSERT ON weave_change_set FOR EACH ROW WHEN (NEW.kind='release') EXECUTE FUNCTION change_evidence()`)
			_, err := NewService(NewPostgresStore(pool), pool, nil).Create(releaseAuthCtx(ctx, owner), project, CreateInput{Version: "9.0.1"})
			requireContentRejection(t, err)
			var count int
			mustScan(t, pool, `SELECT count(*) FROM weave_releases`, nil, &count)
			if count != 0 {
				t.Fatal("failed evidence mutation retained a partial cut")
			}
		})
	}
}

func TestContentReceiptUsesOneSnapshotDuringLiveEdit(t *testing.T) {
	pool := testdb.EmptyPool(t)
	ctx, owner, project := seedZREL(t, pool)
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	blocker, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Release()
	if _, err := blocker.Exec(ctx, `SELECT pg_advisory_lock(330031)`); err != nil {
		t.Fatal(err)
	}
	defer blocker.Exec(context.Background(), `SELECT pg_advisory_unlock(330031)`)
	mustExec(t, pool, `CREATE FUNCTION pause_content_cut() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN PERFORM pg_advisory_xact_lock(330031); RETURN NEW; END $$`)
	mustExec(t, pool, `CREATE TRIGGER pause_content_cut AFTER INSERT ON weave_change_set FOR EACH ROW WHEN (NEW.kind='release') EXECUTE FUNCTION pause_content_cut()`)
	done := make(chan error, 1)
	go func() {
		_, err := NewService(NewPostgresStore(pool), pool, nil).Create(releaseAuthCtx(ctx, owner), project, CreateInput{Version: "9.0.1"})
		done <- err
	}()
	for {
		var waiting bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_locks WHERE locktype='advisory' AND objid=330031 AND NOT granted)`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		select {
		case err := <-done:
			t.Fatalf("cut finished before coordination: %v", err)
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(10 * time.Millisecond):
		}
	}
	mustExec(t, pool, `UPDATE weave_fields SET system_name='after' WHERE id='ZRELF1'`)
	mustExec(t, pool, `UPDATE weave_projects SET enforce_concept_lists=NOT enforce_concept_lists WHERE id='ZREL'`)
	if _, err := blocker.Exec(ctx, `SELECT pg_advisory_unlock(330031)`); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	var name string
	mustScan(t, pool, `SELECT system_name FROM weave_fields_archive WHERE id='ZRELF1' AND version_number='9.0.1'`, nil, &name)
	if name != "release_probe" {
		t.Fatalf("cut mixed database snapshots: %q", name)
	}
	if err := readsession.NewFactory(pool).Read(ctx, auth.Release("9.0.1"), project, func(s *readsession.Session) error { _, err := s.Content(ctx); return err }); err != nil {
		t.Fatal(err)
	}
}
