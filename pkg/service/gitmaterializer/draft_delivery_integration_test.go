//go:build integration

package gitmaterializer_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
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
	"github.com/pletka-io/pletka/pkg/service/gitmaterializer"
	"github.com/pletka-io/pletka/pkg/service/projectread"
	"github.com/pletka-io/pletka/pkg/weave"
	"github.com/pletka-io/pletka/pkg/weave/field"
	"github.com/pletka-io/pletka/pkg/weave/release"
)

type capturedFixture struct {
	pool    *pgxpool.Pool
	mat     *gitmaterializer.Materializer
	capture *draftcapture.Service
	repo    string
	request draftcapture.Request
}

func newCapturedFixture(t *testing.T) capturedFixture {
	t.Helper()
	skipIfNoGit(t)
	p := testdb.EmptyPool(t)
	descriptorExec(t, p, `INSERT INTO weave_actors(id,type,display_name,slug,visibility) VALUES('CG_OWNER','organization','Capture owner','cg-owner','public'); INSERT INTO weave_projects(id,owner_id,status,visibility) VALUES('CG','CG_OWNER','draft','public'); INSERT INTO weave_fields(id,project_id,path_elements) VALUES('CGFIELD','CG','[]')`)
	capture, err := draftcapture.New(p, projectread.DefaultOptions(), func(context.Context, auth.Resource) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	base := t.TempDir()
	return capturedFixture{pool: p, mat: gitmaterializer.NewMaterializer(p, base, testLogger()), capture: capture, repo: filepath.Join(base, "CG"), request: draftcapture.Request{ProjectID: "CG", ActorID: "CG_OWNER", ActorName: "Captured Author", ActorEmail: "captured@example.test", Message: "first edit"}}
}
func capturedTree(t *testing.T, repo, rev string) projectbundle.Files {
	t.Helper()
	listing, err := exec.CommandContext(t.Context(), "git", "-C", repo, "ls-tree", "-r", "--name-only", "-z", rev).Output()
	if err != nil {
		t.Fatal(err)
	}
	files := projectbundle.Files{}
	for _, path := range strings.Split(string(listing), "\x00") {
		if path == "" {
			continue
		}
		b, err := exec.CommandContext(t.Context(), "git", "-C", repo, "show", rev+":"+path).Output()
		if err != nil {
			t.Fatal(err)
		}
		files[path] = b
	}
	return files
}
func requireCapturedTree(t *testing.T, repo, rev string, value projectsnapshot.CompleteProject) {
	t.Helper()
	want, err := projectbundle.Encode(value)
	if err != nil {
		t.Fatal(err)
	}
	got := capturedTree(t, repo, rev)
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatal(diff)
	}
	if _, err := projectbundle.Decode(got, projectbundle.DefaultLimits()); err != nil {
		t.Fatal(err)
	}
}
func (f capturedFixture) init(t *testing.T) {
	t.Helper()
	if err := f.mat.InitProjectWithOptions(t.Context(), "CG", gitmaterializer.InitProjectOptions{CaptureDraft: true}); err != nil {
		t.Fatal(err)
	}
}
func (f capturedFixture) edit(t *testing.T, name string) draftcapture.Receipt {
	t.Helper()
	req := f.request
	req.Message = name
	r, err := f.capture.Mutate(t.Context(), req, func(ctx context.Context) error {
		return weave.NewChangeLogRunner(weave.NewPostgresStore(f.pool)).Run(ctx, func(ctx context.Context, rec domain.ChangeLogRecorder) error {
			_, err := txscope.New(f.pool).Exec(ctx, `UPDATE weave_fields SET system_name=$1 WHERE id='CGFIELD'`, name)
			if err != nil {
				return err
			}
			return rec.Record(ctx, domain.ChangeLogEntry{EntityType: "field", EntityID: "CGFIELD", ProjectID: "CG", Operation: "update", Payload: []byte(`{}`), PreviousPayload: []byte(`{}`)})
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestCapturedDraftBaselineAndDelayedEditsPreserveWorkspace(t *testing.T) {
	f := newCapturedFixture(t)
	ctx := t.Context()
	// Closed legacy audits are explicitly covered by the new baseline.
	descriptorExec(t, f.pool, `INSERT INTO weave_change_set(project_id,actor_name,actor_email,commit_message,closed_at) VALUES('CG','Old author','old@example.test','older audit',now())`)
	if err := os.MkdirAll(f.repo, 0755); err != nil {
		t.Fatal(err)
	}
	gitOut(t, f.repo, "init")
	gitOut(t, f.repo, "config", "user.name", "Workspace user")
	gitOut(t, f.repo, "config", "user.email", "workspace@example.test")
	if err := os.WriteFile(filepath.Join(f.repo, "notes.txt"), []byte("original"), 0644); err != nil {
		t.Fatal(err)
	}
	gitOut(t, f.repo, "add", "notes.txt")
	gitOut(t, f.repo, "commit", "-m", "Existing workspace")
	head := gitOut(t, f.repo, "rev-parse", "HEAD")
	if err := os.WriteFile(filepath.Join(f.repo, "notes.txt"), []byte("staged"), 0644); err != nil {
		t.Fatal(err)
	}
	gitOut(t, f.repo, "add", "notes.txt")
	if err := os.WriteFile(filepath.Join(f.repo, "notes.txt"), []byte("unstaged"), 0644); err != nil {
		t.Fatal(err)
	}
	index, err := os.ReadFile(filepath.Join(f.repo, ".git", "index"))
	if err != nil {
		t.Fatal(err)
	}
	f.init(t)
	baseline, err := f.capture.Baseline(ctx, "CG")
	if err != nil {
		t.Fatal(err)
	}
	requireCapturedTree(t, f.repo, gitmaterializer.CapturedDraftRef, baseline)
	first := f.edit(t, "first edit")
	second := f.edit(t, "later edit")
	if n, err := f.mat.ProcessPending(ctx, 1); err != nil || n != 1 {
		t.Fatalf("first delivery: %d %v", n, err)
	}
	firstSHA := gitOut(t, f.repo, "rev-parse", gitmaterializer.CapturedDraftRef)
	requireCapturedTree(t, f.repo, firstSHA, first.After())
	if got := gitOut(t, f.repo, "log", "-1", "--format=%s", firstSHA); got != "first edit" {
		t.Fatal(got)
	}
	if n, err := f.mat.ProcessPending(ctx, 1); err != nil || n != 1 {
		t.Fatalf("second delivery: %d %v", n, err)
	}
	requireCapturedTree(t, f.repo, gitmaterializer.CapturedDraftRef, second.After())
	if err := f.mat.InitProject(ctx, "CG"); err != nil {
		t.Fatal(err)
	}
	if n, err := f.mat.Reconcile(ctx, "CG"); err != nil || n != 0 {
		t.Fatalf("reconcile: %d %v", n, err)
	}
	if got := gitOut(t, f.repo, "rev-parse", "HEAD"); got != head {
		t.Fatal("workspace HEAD changed")
	}
	actual, err := os.ReadFile(filepath.Join(f.repo, ".git", "index"))
	if err != nil || !cmp.Equal(actual, index) {
		t.Fatal("workspace index changed", err)
	}
	body, err := os.ReadFile(filepath.Join(f.repo, "notes.txt"))
	if err != nil || string(body) != "unstaged" {
		t.Fatal("workspace file changed", err)
	}
	var outcomes []string
	rows, err := f.pool.Query(ctx, `SELECT materialized_outcome FROM weave_change_set WHERE project_id='CG' ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var outcome string
		if err := rows.Scan(&outcome); err != nil {
			t.Fatal(err)
		}
		outcomes = append(outcomes, outcome)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff([]string{"baselined", "committed", "committed"}, outcomes); diff != "" {
		t.Fatal(diff)
	}
}

func TestCapturedDraftResumesPublishedButUnacknowledgedCommit(t *testing.T) {
	f := newCapturedFixture(t)
	f.init(t)
	ctx := t.Context()
	r := f.edit(t, "recover exactly once")
	descriptorExec(t, f.pool, `CREATE FUNCTION fail_draft_ack() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected acknowledgement failure'; END $$; CREATE TRIGGER test_fail_ack BEFORE UPDATE ON weave_draft_deliveries FOR EACH ROW EXECUTE FUNCTION fail_draft_ack()`)
	if n, err := f.mat.ProcessPending(ctx, 10); err != nil || n != 0 {
		t.Fatalf("injected ack failure: %d %v", n, err)
	}
	published := gitOut(t, f.repo, "rev-parse", gitmaterializer.CapturedDraftRef)
	requireCapturedTree(t, f.repo, published, r.After())
	state, err := sqlcgen.New(f.pool).WeaveLastDraftDelivery(ctx, "CG")
	if err != nil || state.AcknowledgedAt.Valid || state.CommitSha != published {
		t.Fatalf("missing prepared receipt: %+v %v", state, err)
	}
	// A later committed edit must not change the bytes/message of the retry.
	later := f.edit(t, "next operation")
	descriptorExec(t, f.pool, `DROP TRIGGER test_fail_ack ON weave_draft_deliveries; DROP FUNCTION fail_draft_ack()`)
	if n, err := f.mat.ProcessPending(ctx, 1); err != nil || n != 1 {
		t.Fatalf("resume: %d %v", n, err)
	}
	if got := gitOut(t, f.repo, "rev-parse", gitmaterializer.CapturedDraftRef); got != published {
		t.Fatal("retry made a second commit")
	}
	if n, err := f.mat.ProcessPending(ctx, 1); err != nil || n != 1 {
		t.Fatalf("later delivery: %d %v", n, err)
	}
	requireCapturedTree(t, f.repo, gitmaterializer.CapturedDraftRef, later.After())
}

func TestCapturedDraftRejectsForeignRefAndRollsBackFailedEdits(t *testing.T) {
	f := newCapturedFixture(t)
	f.init(t)
	ctx := t.Context()
	_, err := f.capture.Mutate(ctx, f.request, func(bound context.Context) error {
		if err := field.NewPostgresStore(f.pool).Create(bound, &domain.Field{ID: "ROLLED_BACK", ProjectID: "CG", PathElements: []domain.PathElement{}}); err != nil {
			return err
		}
		return errors.New("abort mutation")
	})
	if err == nil {
		t.Fatal("failed edit succeeded")
	}
	if n, err := f.mat.ProcessPending(ctx, 10); err != nil || n != 0 {
		t.Fatalf("rollback enqueued delivery: %d %v", n, err)
	}
	baseline := gitOut(t, f.repo, "rev-parse", gitmaterializer.CapturedDraftRef)
	f.edit(t, "pending operation")
	// Move the owned ref to an unrelated, otherwise valid object; no overwrite.
	tree := gitOut(t, f.repo, "rev-parse", baseline+"^{tree}")
	cmd := exec.CommandContext(t.Context(), "git", "-C", f.repo, "commit-tree", tree, "-m", "foreign commit")
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=Other", "GIT_AUTHOR_EMAIL=other@example.test", "GIT_COMMITTER_NAME=Other", "GIT_COMMITTER_EMAIL=other@example.test")
	body, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	foreign := strings.TrimSpace(string(body))
	gitOut(t, f.repo, "update-ref", gitmaterializer.CapturedDraftRef, foreign, baseline)
	if _, err := f.mat.Reconcile(ctx, "CG"); err == nil {
		t.Fatal("unreviewed ref overwritten")
	}
	if got := gitOut(t, f.repo, "rev-parse", gitmaterializer.CapturedDraftRef); got != foreign {
		t.Fatal("foreign ref changed")
	}
	gitOut(t, f.repo, "update-ref", gitmaterializer.CapturedDraftRef, baseline, foreign)
	if _, err := f.mat.Reconcile(ctx, "CG"); err != nil {
		t.Fatal(err)
	}
	delivered := gitOut(t, f.repo, "rev-parse", gitmaterializer.CapturedDraftRef)
	gitOut(t, f.repo, "update-ref", "-d", gitmaterializer.CapturedDraftRef, delivered)
	if _, err := f.mat.Reconcile(ctx, "CG"); err != nil {
		t.Fatal(err)
	}
	if got := gitOut(t, f.repo, "rev-parse", gitmaterializer.CapturedDraftRef); got != delivered {
		t.Fatal("repair did not restore acknowledged commit")
	}
}

func TestCapturedDraftReconstructsPrunedPreparedObjects(t *testing.T) {
	f := newCapturedFixture(t)
	f.init(t)
	ctx := t.Context()
	r := f.edit(t, "prepared before crash")
	parent := gitOut(t, f.repo, "rev-parse", gitmaterializer.CapturedDraftRef)
	if err := f.mat.PrepareCapturedDraftForTest(ctx, "CG"); err != nil {
		t.Fatal(err)
	}
	state, err := sqlcgen.New(f.pool).WeaveLastDraftDelivery(ctx, "CG")
	if err != nil || state.AcknowledgedAt.Valid {
		t.Fatalf("prepare: %+v %v", state, err)
	}
	if got := gitOut(t, f.repo, "rev-parse", gitmaterializer.CapturedDraftRef); got != parent {
		t.Fatal("preparation published prematurely")
	}
	// Only this test's temporary repository is pruned. The database's durable
	// operation must suffice to reproduce its exact missing commit object.
	gitOut(t, f.repo, "prune", "--expire", "now")
	if err := exec.CommandContext(t.Context(), "git", "-C", f.repo, "cat-file", "-e", state.CommitSha).Run(); err == nil {
		t.Fatal("prepared object was not pruned")
	}
	if n, err := f.mat.ProcessPending(ctx, 1); err != nil || n != 1 {
		t.Fatalf("resume pruned preparation: %d %v", n, err)
	}
	if got := gitOut(t, f.repo, "rev-parse", gitmaterializer.CapturedDraftRef); got != state.CommitSha {
		t.Fatal("reconstruction changed operation identity")
	}
	requireCapturedTree(t, f.repo, gitmaterializer.CapturedDraftRef, r.After())
}

func TestCapturedDraftContextAndBytesStayFrozenDuringDelivery(t *testing.T) {
	f := newCapturedFixture(t)
	f.init(t)
	ctx := t.Context()
	descriptorExec(t, f.pool, `UPDATE weave_actors SET display_name='Captured context' WHERE id='CG_OWNER'`)
	req := f.request
	req.Message = "Refresh external context"
	r, err := f.capture.RefreshContext(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	descriptorExec(t, f.pool, `UPDATE weave_actors SET display_name='Later context' WHERE id='CG_OWNER'`)
	gitOut(t, f.repo, "config", "filter.corrupt.clean", "tr a-z A-Z")
	if err := os.WriteFile(filepath.Join(f.repo, ".git", "info", "attributes"), []byte("*.json filter=corrupt\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := f.mat.Reconcile(ctx, "CG"); err != nil {
		t.Fatal(err)
	}
	requireCapturedTree(t, f.repo, gitmaterializer.CapturedDraftRef, r.After())
	if got := gitOut(t, f.repo, "log", "-1", "--format=%s", gitmaterializer.CapturedDraftRef); got != req.Message {
		t.Fatal(got)
	}
}

func TestCapturedDraftStillPublishesImmutableReleaseJobs(t *testing.T) {
	f := newCapturedFixture(t)
	f.init(t)
	ctx := auth.WithPrincipal(auth.WithSnapshot(t.Context(), &auth.AuthSnapshot{IsSuperAdmin: true}), &auth.Principal{ActorID: "CG_OWNER"})
	req := f.request
	req.Message = "Release project 1.0.0"
	receipt, err := f.capture.Mutate(ctx, req, func(bound context.Context) error {
		_, err := release.NewService(release.NewPostgresStore(f.pool), f.pool, nil).Create(bound, "CG", release.CreateInput{Version: "1.0.0"})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	pending, err := sqlcgen.New(f.pool).WeaveListUnprocessedChangeSets(ctx, 10)
	if err != nil || len(pending) != 1 || pending[0].Kind != "release" {
		t.Fatalf("enrolled release was hidden: %+v %v", pending, err)
	}
	if n, err := f.mat.ProcessPending(ctx, 10); err != nil || n != 2 {
		t.Fatalf("release and capture jobs: %d %v", n, err)
	}
	gitOut(t, f.repo, "rev-parse", "refs/tags/v1.0.0")
	requireCapturedTree(t, f.repo, gitmaterializer.CapturedDraftRef, receipt.After())
}

func TestCapturedDraftJournalAndAuditIdentityAreProtected(t *testing.T) {
	f := newCapturedFixture(t)
	f.init(t)
	f.edit(t, "immutable identity")
	if _, err := f.mat.Reconcile(t.Context(), "CG"); err != nil {
		t.Fatal(err)
	}
	for _, sql := range []string{
		`UPDATE weave_draft_deliveries SET snapshot_digest=repeat('0',64) WHERE project_id='CG'`,
		`DELETE FROM weave_draft_baselines WHERE project_id='CG'`,
		`UPDATE weave_draft_audits SET revision=0 WHERE project_id='CG'`,
		`UPDATE weave_change_set SET commit_message='rewrite attribution' WHERE project_id='CG'`,
		`INSERT INTO weave_draft_audits SELECT id,project_id,0,'baseline' FROM weave_change_set WHERE project_id='CG'`,
	} {
		if _, err := f.pool.Exec(t.Context(), sql); err == nil {
			t.Fatalf("protected history accepted %s", sql)
		}
	}
}

func TestCapturedDraftRefCannotAliasOrBeCheckedOut(t *testing.T) {
	f := newCapturedFixture(t)
	f.init(t)
	baseline := gitOut(t, f.repo, "rev-parse", gitmaterializer.CapturedDraftRef)
	gitOut(t, f.repo, "symbolic-ref", gitmaterializer.CapturedDraftRef, "refs/heads/missing-target")
	if _, err := f.mat.Reconcile(t.Context(), "CG"); err == nil {
		t.Fatal("dangling symbolic ref accepted")
	}
	gitOut(t, f.repo, "symbolic-ref", "--delete", gitmaterializer.CapturedDraftRef)
	gitOut(t, f.repo, "update-ref", gitmaterializer.CapturedDraftRef, baseline)
	gitOut(t, f.repo, "checkout", "pletka-draft")
	if _, err := f.mat.Reconcile(t.Context(), "CG"); err == nil {
		t.Fatal("checked-out automation branch accepted")
	}
}

func TestCapturedDraftConcurrentPollersAdvanceOnce(t *testing.T) {
	f := newCapturedFixture(t)
	f.init(t)
	f.edit(t, "first queued revision")
	last := f.edit(t, "second queued revision")
	type outcome struct {
		n   int
		err error
	}
	done := make(chan outcome, 2)
	for range 2 {
		go func() { n, err := f.mat.ProcessPending(t.Context(), 1); done <- outcome{n, err} }()
	}
	total := 0
	for range 2 {
		r := <-done
		if r.err != nil {
			t.Fatal(r.err)
		}
		total += r.n
	}
	if total != 2 {
		t.Fatalf("processed %d revisions", total)
	}
	if got := gitOut(t, f.repo, "rev-list", "--count", gitmaterializer.CapturedDraftRef); got != "3" {
		t.Fatalf("duplicated/skipped operation: %s commits", got)
	}
	requireCapturedTree(t, f.repo, gitmaterializer.CapturedDraftRef, last.After())
}

func TestCapturedDraftUpgradeKeepsOriginalBaselineAndAuditCoverage(t *testing.T) {
	f := newCapturedFixture(t)
	ctx := t.Context()
	db := stdlib.OpenDBFromPool(f.pool)
	defer db.Close()
	// Create the actual pre-delivery state, where capture existed without a Git
	// baseline or exact audit-to-revision links.
	if err := database.MigrateDown(db); err != nil {
		t.Fatal(err)
	}
	baseline, err := f.capture.Enroll(ctx, "CG")
	if err != nil {
		t.Fatal(err)
	}
	first := f.edit(t, "before upgrade one")
	second := f.edit(t, "before upgrade two")
	if err := database.Migrate(db); err != nil {
		t.Fatal(err)
	}
	third := f.edit(t, "after upgrade")
	for i, want := range []projectsnapshot.CompleteProject{baseline, first.After(), second.After(), third.After()} {
		if n, err := f.mat.ProcessPending(ctx, 1); err != nil || n != 1 {
			t.Fatalf("revision %d: %d %v", i, n, err)
		}
		requireCapturedTree(t, f.repo, gitmaterializer.CapturedDraftRef, want)
		var processed int
		if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM weave_change_set WHERE project_id='CG' AND processed_at IS NOT NULL`).Scan(&processed); err != nil {
			t.Fatal(err)
		}
		expected := 0
		if i >= 2 {
			expected = i
		}
		if processed != expected {
			t.Fatalf("revision %d acknowledged %d audits, want %d", i, processed, expected)
		}
	}
	var outcomes []string
	if err := f.pool.QueryRow(ctx, `SELECT array_agg(materialized_outcome ORDER BY id) FROM weave_change_set WHERE project_id='CG'`).Scan(&outcomes); err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff([]string{"covered", "covered", "committed"}, outcomes); diff != "" {
		t.Fatal(diff)
	}
}
