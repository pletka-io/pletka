//go:build integration

package draftcapture_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/database"
	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/database/txscope"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/projectbundle"
	"github.com/pletka-io/pletka/pkg/projectsnapshot"
	"github.com/pletka-io/pletka/pkg/service/draftcapture"
	"github.com/pletka-io/pletka/pkg/service/projectread"
	"github.com/pletka-io/pletka/pkg/weave"
	"github.com/pletka-io/pletka/pkg/weave/field"
	"github.com/pletka-io/pletka/pkg/weave/override"
)

func setup(t *testing.T) (*pgxpool.Pool, *draftcapture.Service, draftcapture.Request) {
	t.Helper()
	p := testdb.EmptyPool(t)
	ctx := t.Context()
	_, err := p.Exec(ctx, `INSERT INTO weave_actors(id,type,display_name,slug,visibility) VALUES('dc-owner','organization','Owner','dc-owner','public');
INSERT INTO weave_projects(id,ui_name,status,owner_id,visibility) VALUES('DC','{"en":"Draft capture"}','draft','dc-owner','public');`)
	if err != nil {
		t.Fatal(err)
	}
	s, err := draftcapture.New(p, projectread.DefaultOptions(), func(context.Context, auth.Resource) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	return p, s, draftcapture.Request{ProjectID: "DC", ActorID: "dc-owner", ActorName: "Author", ActorEmail: "author@example.test", Message: "operation"}
}
func bytes(t *testing.T, v projectsnapshot.CompleteProject) projectbundle.Files {
	t.Helper()
	f, e := projectbundle.Encode(v)
	if e != nil {
		t.Fatal(e)
	}
	return f
}

func newField(id string) *domain.Field {
	return &domain.Field{ID: id, ProjectID: "DC", PathElements: []domain.PathElement{}}
}

func TestCaptureRollbackAndDelayedReceipts(t *testing.T) {
	p, s, req := setup(t)
	ctx := t.Context()
	baseline, err := s.Enroll(ctx, req.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	store := field.NewPostgresStore(p)
	runner := weave.NewChangeLogRunner(weave.NewPostgresStore(p))
	first, err := s.Mutate(ctx, req, func(bound context.Context) error {
		return runner.Run(bound, func(bound context.Context, rec domain.ChangeLogRecorder) error {
			f := newField("DCF1")
			if err := store.Create(bound, f); err != nil {
				return err
			}
			return rec.Record(bound, domain.ChangeLogEntry{EntityType: "field", EntityID: f.ID, ProjectID: "DC", Operation: "create", Payload: []byte(`{}`)})
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.Revision() != 1 || cmp.Diff(bytes(t, baseline), bytes(t, first.Before())) != "" {
		t.Fatal("wrong first revision baseline")
	}
	rollback := errors.New("force rollback")
	_, err = s.Mutate(ctx, req, func(bound context.Context) error {
		if err := store.Create(bound, newField("DCF2")); err != nil {
			return err
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatal(err)
	}
	var n int
	if err := p.QueryRow(ctx, `SELECT count(*) FROM weave_fields WHERE id='DCF2'`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("mutation escaped rollback: %d %v", n, err)
	}
	second, err := s.Mutate(ctx, req, func(bound context.Context) error {
		return store.Create(bound, newField("DCF3"))
	})
	if err != nil {
		t.Fatal(err)
	}
	if second.Revision() != 2 {
		t.Fatal("rollback consumed project revision")
	}
	delayed, err := s.Get(ctx, "DC", 1)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(bytes(t, first.After()), bytes(t, delayed.After())); diff != "" {
		t.Fatal(diff)
	}
	if diff := cmp.Diff(bytes(t, first.After()), bytes(t, second.Before())); diff != "" {
		t.Fatal(diff)
	}
	if _, err := p.Exec(ctx, `UPDATE weave_fields SET system_name='bypass' WHERE id='DCF1'`); err == nil {
		t.Fatal("raw write bypassed capture")
	}
	logs, err := sqlcgen.New(p).WeaveListUnprocessedChangeSets(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 0 {
		t.Fatal("legacy writer can consume an enrolled project's log")
	}
}

func TestAuditFailureRollsBackOrdinaryStoreWrite(t *testing.T) {
	p, _, _ := setup(t)
	ctx := t.Context()
	runner := weave.NewChangeLogRunner(weave.NewPostgresStore(p))
	store := field.NewPostgresStore(p)
	recorded := false
	err := runner.Run(ctx, func(bound context.Context, rec domain.ChangeLogRecorder) error {
		if err := store.Create(bound, newField("FAILLOG")); err != nil {
			return err
		}
		recorded = true
		return rec.Record(bound, domain.ChangeLogEntry{Operation: "invalid"})
	})
	if err == nil || !recorded {
		t.Fatalf("did not reach invalid log: %v", err)
	}
	var n int
	if err := p.QueryRow(ctx, `SELECT count(*) FROM weave_fields WHERE id='FAILLOG'`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("content survived failed log: %d %v", n, err)
	}
}

func TestCaptureContextRefreshAndConcurrentOrder(t *testing.T) {
	p, s, req := setup(t)
	ctx := t.Context()
	if _, err := s.Enroll(ctx, "DC"); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Exec(ctx, `UPDATE weave_actors SET display_name='New name' WHERE id='dc-owner'`); err != nil {
		t.Fatal(err)
	}
	called := false
	if _, err := s.Mutate(ctx, req, func(context.Context) error { called = true; return nil }); err == nil || called {
		t.Fatal("context drift was borrowed by edit")
	}
	refresh, err := s.RefreshContext(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if cmp.Diff(bytes(t, refresh.Before()), bytes(t, refresh.After())) == "" {
		t.Fatal("context refresh missing")
	}
	var wg sync.WaitGroup
	results := make(chan draftcapture.Receipt, 2)
	errs := make(chan error, 2)
	for _, id := range []string{"DCC1", "DCC2"} {
		wg.Go(func() {
			r, e := s.Mutate(ctx, req, func(bound context.Context) error {
				return field.NewPostgresStore(p).Create(bound, newField(id))
			})
			results <- r
			errs <- e
		})
	}
	wg.Wait()
	close(results)
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	byRevision := map[int64]draftcapture.Receipt{}
	for r := range results {
		byRevision[r.Revision()] = r
	}
	if len(byRevision) != 2 {
		t.Fatal("concurrent revisions collided")
	}
	if diff := cmp.Diff(bytes(t, byRevision[2].After()), bytes(t, byRevision[3].Before())); diff != "" {
		t.Fatal(diff)
	}
	var escaped context.Context
	if _, err := s.Mutate(ctx, req, func(bound context.Context) error { escaped = bound; return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := txscope.New(p).Exec(escaped, `SELECT 1`); err == nil {
		t.Fatal("escaped transaction context remained usable")
	}
}

func TestCaptureCascadesLosslessEffectsAndPurePatch(t *testing.T) {
	p, s, req := setup(t)
	ctx := t.Context()
	if _, err := s.Enroll(ctx, "DC"); err != nil {
		t.Fatal(err)
	}
	inserted, err := s.Mutate(ctx, req, func(bound context.Context) error {
		_, err := txscope.New(p).Exec(bound, `INSERT INTO weave_models(id,project_id) VALUES('DCM','DC');
INSERT INTO weave_fields(id,project_id,path_elements,ui_name,description) VALUES('DCF','DC','[]',NULL,'null');
INSERT INTO weave_field_overrides(id,field_id,project_id,entity_type,entity_id,category_id,part_of_collection_id) VALUES(8100,'DCF','DC','model','DCM',NULL,NULL);
INSERT INTO weave_examples(id,project_id,entity_type,entity_id) VALUES('DCE','DC','model','DCM');
INSERT INTO weave_example_values(id,example_id,override_id,field_id,value_kind,value_payload,number_value,date_value,slot_path)
VALUES(8100,'DCE',8100,'DCF','integer','null',9007199254740993.123456789,'infinity','root');`)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	patch, err := inserted.Patch()
	if err != nil {
		t.Fatal(err)
	}
	merged, err := projectsnapshot.PatchDraft(inserted.Before(), patch)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(bytes(t, inserted.After()), bytes(t, merged)); diff != "" {
		t.Fatal(diff)
	}
	var sqlNull, jsonNull, numeric, date string
	err = p.QueryRow(ctx, `SELECT after_row->'ui_name',after_row->'description' FROM weave_draft_effects WHERE project_id='DC' AND table_name='weave_fields' AND revision=1`).Scan(&sqlNull, &jsonNull)
	if err != nil || sqlNull != "null" || jsonNull != `"null"` {
		t.Fatalf("null distinction lost: %q %q %v", sqlNull, jsonNull, err)
	}
	err = p.QueryRow(ctx, `SELECT after_row->>'number_value',after_row->>'date_value' FROM weave_draft_effects WHERE project_id='DC' AND table_name='weave_example_values' AND revision=1`).Scan(&numeric, &date)
	if err != nil || numeric != "9007199254740993.123456789" || date != "infinity" {
		t.Fatalf("exact values lost: %s %s %v", numeric, date, err)
	}
	deleted, err := s.Mutate(ctx, req, func(bound context.Context) error {
		_, err := txscope.New(p).Exec(bound, `DELETE FROM weave_examples WHERE id='DCE'`)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	var n int
	if err := p.QueryRow(ctx, `SELECT count(*) FROM weave_draft_effects WHERE project_id='DC' AND revision=2 AND before_row IS NOT NULL AND after_row IS NULL`).Scan(&n); err != nil || n != 2 {
		t.Fatalf("cascade ownership missing: %d %v", n, err)
	}
	if _, err := deleted.Patch(); err != nil {
		t.Fatal(err)
	}
}

func TestCaptureRejectsUnsealedCommitAndStaleEnrollmentSnapshot(t *testing.T) {
	p, s, req := setup(t)
	ctx := t.Context()
	stale, err := p.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		t.Fatal(err)
	}
	defer stale.Rollback(ctx) //nolint:errcheck
	if _, err := stale.Exec(ctx, `SELECT * FROM weave_projects WHERE id='DC'`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Enroll(ctx, "DC"); err != nil {
		t.Fatal(err)
	}
	_, err = stale.Exec(ctx, `UPDATE weave_projects SET system_name='stale bypass' WHERE id='DC'`)
	var pgerr *pgconn.PgError
	if !errors.As(err, &pgerr) || pgerr.Code != "40001" {
		t.Fatalf("old snapshot bypassed enrollment fence: %v", err)
	}
	_ = stale.Rollback(ctx)
	tx, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	_, err = sqlcgen.New(tx).WeaveStartDraftMutation(ctx, sqlcgen.WeaveStartDraftMutationParams{ProjectID: "DC", Purpose: "edit", ActorName: req.ActorName, ActorEmail: req.ActorEmail, Message: req.Message})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO weave_fields(id,project_id,path_elements) VALUES('UNSEALED','DC','[]')`); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err == nil {
		t.Fatal("incomplete receipt committed")
	}
	var n int
	if err := p.QueryRow(ctx, `SELECT count(*) FROM weave_fields WHERE id='UNSEALED'`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("content escaped failed seal: %d %v", n, err)
	}
	if _, err := p.Exec(ctx, `DELETE FROM weave_draft_projects WHERE project_id='DC'`); err == nil {
		t.Fatal("enrollment could be removed to bypass capture")
	}
}

func TestCaptureSingleConnectionLocksAndSameRowHistory(t *testing.T) {
	pool, _, req := setup(t)
	cfg := pool.Config()
	cfg.MaxConns = 1
	cfg.MinConns = 0
	one, err := pgxpool.NewWithConfig(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer one.Close()
	svc, err := draftcapture.New(one, projectread.DefaultOptions(), func(context.Context, auth.Resource) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	if _, err := svc.Enroll(ctx, "DC"); err != nil {
		t.Fatal(err)
	}
	store := field.NewPostgresStore(one)
	locks := override.NewPostgresStore(one)
	first, err := svc.Mutate(ctx, req, func(bound context.Context) error {
		return locks.WithAdvisoryLock(bound, "DC", "field:REPEATED", func(locked context.Context) error { return store.Create(locked, newField("REPEATED")) })
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.Mutate(ctx, req, func(bound context.Context) error {
		_, err := txscope.New(one).Exec(bound, `UPDATE weave_fields SET system_name='later name' WHERE id='REPEATED'`)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(bytes(t, first.After()), bytes(t, second.Before())); diff != "" {
		t.Fatal(diff)
	}
	delayed, err := svc.Get(ctx, "DC", 1)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(bytes(t, first.After()), bytes(t, delayed.After())); diff != "" {
		t.Fatal("later edit stole earlier row: " + diff)
	}
	if _, err := delayed.Patch(); err != nil {
		t.Fatal(err)
	}
	_, err = svc.Mutate(ctx, req, func(bound context.Context) error {
		_, err := txscope.New(pool).Exec(bound, `UPDATE weave_fields SET system_name='wrong pool' WHERE id='REPEATED'`)
		return err
	})
	if err == nil {
		t.Fatal("foreign pool accepted scoped write")
	}
}

func TestCaptureGuardInventoryAndHistoryProtection(t *testing.T) {
	p, s, req := setup(t)
	ctx := t.Context()
	var guards int
	if err := p.QueryRow(ctx, `SELECT count(*) FROM weave_draft_sections_v1() s JOIN pg_trigger t ON t.tgrelid=s.hot_table::regclass WHERE t.tgname IN('guard_draft_capture','capture_draft_effect','guard_draft_truncate') AND t.tgenabled='O'`).Scan(&guards); err != nil || guards != 66 {
		t.Fatalf("capture guards: %d %v", guards, err)
	}
	db := stdlib.OpenDBFromPool(p)
	defer db.Close()
	// An unused installation remains reversible.
	if err := database.MigrateDown(db); err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(db); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Enroll(ctx, "DC"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Mutate(ctx, req, func(bound context.Context) error {
		return field.NewPostgresStore(p).Create(bound, newField("PROTECTED"))
	}); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`DELETE FROM weave_draft_blobs`, `UPDATE weave_draft_snapshots SET manifest='{}'`,
		`DELETE FROM weave_draft_effects`, `DELETE FROM weave_draft_mutations`,
		`TRUNCATE weave_fields CASCADE`, `INSERT INTO weave_draft_row_owners VALUES('weave_examples','bad','DC')`,
		`DELETE FROM weave_draft_project_fences`, `DELETE FROM weave_projects WHERE id='DC'`,
	} {
		if _, err := p.Exec(ctx, statement); err == nil {
			t.Fatalf("history guard accepted %s", statement)
		}
	}
	if err := database.MigrateDown(db); err == nil || !strings.Contains(err.Error(), "cannot downgrade retained atomic draft history") {
		t.Fatalf("capture downgrade guard: %v", err)
	}
	if _, err := s.Get(ctx, "DC", 1); err != nil {
		t.Fatal(err)
	}
}

func TestStaleSnapshotCannotMoveRowOutOfEnrolledProject(t *testing.T) {
	p, s, _ := setup(t)
	ctx := t.Context()
	if _, err := p.Exec(ctx, `INSERT INTO weave_projects(id,status,owner_id) VALUES('OTHER','draft','dc-owner'); INSERT INTO weave_fields(id,project_id,path_elements) VALUES('MOVE','DC','[]')`); err != nil {
		t.Fatal(err)
	}
	stale, err := p.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		t.Fatal(err)
	}
	defer stale.Rollback(ctx) //nolint:errcheck
	if _, err := stale.Exec(ctx, `SELECT * FROM weave_fields WHERE id='MOVE'`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Enroll(ctx, "DC"); err != nil {
		t.Fatal(err)
	}
	_, err = stale.Exec(ctx, `UPDATE weave_fields SET project_id='OTHER' WHERE id='MOVE'`)
	var pgerr *pgconn.PgError
	if !errors.As(err, &pgerr) || pgerr.Code != "40001" {
		t.Fatalf("stale move bypassed capture: %v", err)
	}
}

func TestCaptureLostContextFailsBeforeWaitingOnOwner(t *testing.T) {
	p, s, req := setup(t)
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	if _, err := s.Enroll(ctx, "DC"); err != nil {
		t.Fatal(err)
	}
	_, err := s.Mutate(ctx, req, func(context.Context) error {
		_, err := p.Exec(ctx, `UPDATE weave_projects SET system_name='lost context' WHERE id='DC'`)
		return err
	})
	var pgerr *pgconn.PgError
	if !errors.As(err, &pgerr) || pgerr.ConstraintName != "draft_capture_required" {
		t.Fatalf("unscoped callback did not fail at boundary: %v", err)
	}
}
