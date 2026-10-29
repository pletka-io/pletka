//go:build integration

package release

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/database/rebuildlease"
)

func seedRebuild(t *testing.T) (context.Context, *pgxpool.Pool, *Service) {
	t.Helper()
	pool := testdb.EmptyPool(t)
	ctx, owner, _ := seedZREL(t, pool)
	ctx = releaseAuthCtx(ctx, owner)
	mustExec(t, pool, `INSERT INTO weave_releases(project_id,version,title,created_by_id) VALUES ('ZREL','9.0.1','Legacy',$1)`, owner)
	mustExec(t, pool, `INSERT INTO weave_projects_archive(id,owner_id,version_number,ui_name) VALUES ('ZREL',$1,'9.0.1','{"en":"Old archive"}')`, owner)
	return ctx, pool, NewService(NewPostgresStore(pool), pool, nil)
}

func checkRebuild(context.Context, string, bool) error { return nil }
func finishRebuild(_ context.Context, op RebuildOperation) (map[string]string, error) {
	commits := map[string]string{}
	for _, r := range op.Releases {
		commits[r.ProjectID] = strings.Repeat("a", 40)
	}
	return commits, nil
}

func TestRebuildResumeKeepsCapturedContent(t *testing.T) {
	ctx, pool, svc := seedRebuild(t)
	p, err := svc.PlanRebuild(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	request := RebuildRequest{Plan: p}
	lease, err := rebuildlease.Runtime(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.RunRebuild(ctx, request, checkRebuild, finishRebuild); err == nil {
		t.Fatal("running runtime allowed rebuild")
	}
	lease()
	op, err := svc.RunRebuild(ctx, request, checkRebuild, func(context.Context, RebuildOperation) (map[string]string, error) {
		return nil, errors.New("interrupted Git")
	})
	if err == nil || op.ID == "" || len(op.Releases) != 1 {
		t.Fatalf("missing durable failed publication: %+v %v", op, err)
	}
	if lease, err := rebuildlease.Runtime(ctx, pool); err == nil {
		lease()
		t.Fatal("pending operation allowed runtime")
	}
	var marker int
	var archived string
	mustScan(t, pool, `SELECT content_capture_version FROM weave_releases WHERE project_id='ZREL'`, nil, &marker)
	mustScan(t, pool, `SELECT ui_name->>'en' FROM weave_projects_archive WHERE id='ZREL' AND version_number='9.0.1'`, nil, &archived)
	if marker != 1 || archived == "Old archive" {
		t.Fatal("legacy body not replaced by certified cut")
	}
	mustExec(t, pool, `UPDATE weave_projects SET ui_name='{"en":"Later draft"}' WHERE id='ZREL'`)
	finished, err := svc.RunRebuild(ctx, request, checkRebuild, finishRebuild)
	if err != nil {
		t.Fatal(err)
	}
	if !finished.Complete || string(finished.Releases[0].Receipt) != string(op.Releases[0].Receipt) {
		t.Fatal("resume recaptured content")
	}
	var after string
	mustScan(t, pool, `SELECT ui_name->>'en' FROM weave_projects_archive WHERE id='ZREL' AND version_number='9.0.1'`, nil, &after)
	if after != archived {
		t.Fatal("resume followed current drafts")
	}
	lease, err = rebuildlease.Runtime(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	lease()
	_, err = svc.RunRebuild(ctx, request, func(context.Context, string, bool) error { t.Fatal("completed operation repeated"); return nil }, finishRebuild)
	if err != nil {
		t.Fatal(err)
	}
	var processed int
	mustScan(t, pool, `SELECT count(*) FROM weave_change_set WHERE project_id='ZREL' AND kind='release' AND processed_at IS NOT NULL AND git_commit_sha=$1`, []any{strings.Repeat("a", 40)}, &processed)
	if processed != 1 {
		t.Fatalf("outbox completions %d", processed)
	}
}

func TestRebuildStalePlanAndFailureRollback(t *testing.T) {
	for _, mode := range []string{"stale", "capture-error"} {
		t.Run(mode, func(t *testing.T) {
			ctx, pool, svc := seedRebuild(t)
			p, err := svc.PlanRebuild(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			if mode == "stale" {
				mustExec(t, pool, `UPDATE weave_projects SET enforce_concept_lists=true WHERE id='ZREL'`)
			} else {
				mustExec(t, pool, `CREATE FUNCTION reject_rebuild_cut() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'cut failed'; END $$`)
				mustExec(t, pool, `CREATE TRIGGER reject_rebuild_cut BEFORE INSERT ON weave_models_archive FOR EACH ROW EXECUTE FUNCTION reject_rebuild_cut()`)
			}
			if _, err = svc.RunRebuild(ctx, RebuildRequest{Plan: p}, checkRebuild, finishRebuild); err == nil {
				t.Fatal("invalid rebuild succeeded")
			}
			var marker, operations int
			var title string
			mustScan(t, pool, `SELECT content_capture_version FROM weave_releases WHERE project_id='ZREL'`, nil, &marker)
			mustScan(t, pool, `SELECT ui_name->>'en' FROM weave_projects_archive WHERE id='ZREL' AND version_number='9.0.1'`, nil, &title)
			mustScan(t, pool, `SELECT count(*) FROM weave_release_rebuild_operations`, nil, &operations)
			if marker != 0 || operations != 0 || title != "Old archive" {
				t.Fatalf("failed rebuild changed state: %d %d %s", marker, operations, title)
			}
		})
	}
}

func TestRebuildSourcesBeforeConsumers(t *testing.T) {
	ctx, pool, svc := seedRebuild(t)
	mustExec(t, pool, `INSERT INTO weave_projects(id,owner_id) VALUES ('source','ZREL_OWNER')`)
	mustExec(t, pool, `INSERT INTO weave_models(id,project_id) VALUES ('source-model','source')`)
	mustExec(t, pool, `INSERT INTO weave_releases(project_id,version,created_by_id) VALUES ('source','1.0.0','ZREL_OWNER')`)
	mustExec(t, pool, `INSERT INTO weave_project_inheritance(project_id,parent_project_id,source_mode) VALUES ('ZREL','source','draft')`)
	if _, err := svc.PlanRebuild(ctx, []string{"ZREL"}); err == nil {
		t.Fatal("unverified external source accepted")
	}
	p, err := svc.PlanRebuild(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Projects) != 2 || p.Projects[0].ProjectID != "source" {
		t.Fatalf("wrong capture order: %+v", p.Projects)
	}
	// Fail the consumer after the source has already been cut in this transaction.
	mustExec(t, pool, `CREATE FUNCTION reject_rebuild_consumer() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.project_id='ZREL' THEN RAISE EXCEPTION 'consumer cut failed'; END IF; RETURN NEW; END $$`)
	mustExec(t, pool, `CREATE TRIGGER reject_rebuild_consumer BEFORE INSERT ON weave_models_archive FOR EACH ROW EXECUTE FUNCTION reject_rebuild_consumer()`)
	if _, err := svc.RunRebuild(ctx, RebuildRequest{Plan: p}, checkRebuild, finishRebuild); err == nil {
		t.Fatal("consumer failure accepted")
	}
	var oldReleases, newArchives int
	mustScan(t, pool, `SELECT count(*) FROM weave_releases WHERE content_capture_version=0`, nil, &oldReleases)
	mustScan(t, pool, `SELECT count(*) FROM weave_projects_archive WHERE id='source'`, nil, &newArchives)
	if oldReleases != 2 || newArchives != 0 {
		t.Fatal("source replacement escaped failed consumer transaction")
	}
	mustExec(t, pool, `DROP TRIGGER reject_rebuild_consumer ON weave_models_archive`)
	if _, err = svc.RunRebuild(ctx, RebuildRequest{Plan: p}, checkRebuild, finishRebuild); err != nil {
		t.Fatal(err)
	}
	var version, mode string
	mustScan(t, pool, `SELECT source_mode,source_version FROM weave_project_inheritance WHERE project_id='ZREL'`, nil, &mode, &version)
	if mode != "release" || version != "1.0.0" {
		t.Fatalf("inheritance not pinned: %s %s", mode, version)
	}
	mustScan(t, pool, `SELECT source_version FROM weave_release_project_dependencies WHERE project_id='ZREL' AND source_project_id='source'`, nil, &version)
	if version != "1.0.0" {
		t.Fatal("wrong captured dependency")
	}
}

func TestRebuildRejectsAmbiguousCatalogAndCycle(t *testing.T) {
	ctx, pool, svc := seedRebuild(t)
	mustExec(t, pool, `INSERT INTO weave_releases(project_id,version,created_by_id) VALUES ('ZREL','9.0.2','ZREL_OWNER')`)
	if _, err := svc.PlanRebuild(ctx, nil); err == nil || !strings.Contains(err.Error(), "exactly one") {
		t.Fatalf("ambiguous catalog: %v", err)
	}
	if _, err := rebuildOrder([]string{"a", "b"}, map[string][]string{"a": {"b"}, "b": {"a"}}); err == nil {
		t.Fatal("cycle accepted")
	}
}

// The lock command must not establish the repeatable-read snapshot before a
// preceding writer commits. Otherwise the old fingerprint could falsely match.
func TestRebuildSeesWriterThatCommitsWhileWaitingForLocks(t *testing.T) {
	ctx, pool, svc := seedRebuild(t)
	p, err := svc.PlanRebuild(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	writer, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Rollback(ctx)
	if _, err := writer.Exec(ctx, `UPDATE weave_projects SET ui_name='{"en":"Concurrent edit"}' WHERE id='ZREL'`); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	runCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	go func() {
		_, err := svc.RunRebuild(runCtx, RebuildRequest{Plan: p}, checkRebuild, finishRebuild)
		done <- err
	}()
	deadline := time.Now().Add(5 * time.Second)
	for {
		var waiting bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_locks WHERE relation='weave_projects'::regclass AND NOT granted)`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("rebuild never waited for input lock")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := writer.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err == nil || !strings.Contains(err.Error(), "changed") {
		t.Fatalf("stale snapshot accepted: %v", err)
	}
}
