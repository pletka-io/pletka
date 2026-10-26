//go:build integration

package release

import (
	"context"
	"testing"
	"time"
)

func TestCreateUsesOneDatabaseSnapshot(t *testing.T) {
	pool := releaseTestPool(t)
	base, ownerID, projectID := seedZREL(t, pool)
	ctx, cancel := context.WithTimeout(base, 20*time.Second)
	defer cancel()
	mustExec(t, pool, `CREATE FUNCTION pause_release_cut() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN
		IF NEW.project_id = 'ZREL' THEN PERFORM pg_advisory_xact_lock(719823); END IF;
		RETURN NEW;
		END $$`)
	t.Cleanup(func() {
		mustExec(t, pool, `DROP TRIGGER IF EXISTS pause_release_cut ON weave_categories_archive`)
		mustExec(t, pool, `DROP FUNCTION pause_release_cut()`)
	})
	mustExec(t, pool, `CREATE TRIGGER pause_release_cut AFTER INSERT ON weave_categories_archive
		FOR EACH ROW EXECUTE FUNCTION pause_release_cut()`)
	blocker, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Release()
	if _, err := blocker.Exec(ctx, `SELECT pg_advisory_lock(719823)`); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		defer close(result)
		_, cutErr := NewService(NewPostgresStore(pool), pool, nil).Create(releaseAuthCtx(ctx, ownerID), projectID, CreateInput{Version: "9.0.1"})
		result <- cutErr
	}()
	defer func() {
		cancel()
		cleanup, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		if _, err := blocker.Exec(cleanup, `SELECT pg_advisory_unlock(719823)`); err != nil {
			t.Error(err)
		}
		select {
		case <-result:
		case <-cleanup.Done():
			t.Error("release cut did not stop")
		}
	}()
	for {
		var waiting bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_locks WHERE locktype='advisory' AND objid=719823 AND NOT granted)`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		select {
		case err := <-result:
			t.Fatalf("cut finished before synchronization: %v", err)
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(time.Millisecond):
		}
	}
	edit, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer edit.Rollback(context.Background())
	if _, err := edit.Exec(ctx, `UPDATE weave_projects SET ui_name='{"en":"After"}' WHERE id=$1`, projectID); err != nil {
		t.Fatal(err)
	}
	if _, err := edit.Exec(ctx, `UPDATE weave_fields SET ui_name='{"en":"After"}' WHERE id='ZRELF1'`); err != nil {
		t.Fatal(err)
	}
	if err := edit.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := blocker.Exec(ctx, `SELECT pg_advisory_unlock(719823)`); err != nil {
		t.Fatal(err)
	}
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	var projectTitle, fieldTitle, liveProjectTitle, liveFieldTitle string
	mustScan(t, pool, `SELECT ui_name->>'en' FROM weave_projects_archive WHERE id='ZREL' AND version_number='9.0.1'`, nil, &projectTitle)
	mustScan(t, pool, `SELECT ui_name->>'en' FROM weave_fields_archive WHERE id='ZRELF1' AND version_number='9.0.1'`, nil, &fieldTitle)
	mustScan(t, pool, `SELECT ui_name->>'en' FROM weave_projects WHERE id='ZREL'`, nil, &liveProjectTitle)
	mustScan(t, pool, `SELECT ui_name->>'en' FROM weave_fields WHERE id='ZRELF1'`, nil, &liveFieldTitle)
	if projectTitle != "Release Probe" || fieldTitle != "Release Probe" || liveProjectTitle != "After" || liveFieldTitle != "After" {
		t.Fatalf("archive=(%q, %q), live=(%q, %q); want consistent pre-edit archive and committed edit", projectTitle, fieldTitle, liveProjectTitle, liveFieldTitle)
	}
}
