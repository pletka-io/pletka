//go:build integration

package release

import (
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/pletka-io/pletka/pkg/service/gitmaterializer"
)

func TestLegacyRebuildResumesPersistedRequestAfterPublication(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	ctx, pool, _ := seedLegacyRebuild(t, false)
	svc := NewService(NewPostgresStore(pool), pool, nil, WithModuleHosts("modules.example.test", "ontologies.example.test"))
	plan, err := svc.PlanLegacyRebuild(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Format != 2 || len(plan.Projects) != 1 {
		t.Fatalf("expected one format-2 release: %+v", plan)
	}
	// Match the CLI's journal payload, including the repository root and exact
	// reviewed refs. The publisher reconstructs its inputs from this payload.
	type publication struct {
		GitRoot string                            `json:"git_root"`
		Refs    []gitmaterializer.ReleaseRefState `json:"refs"`
	}
	root := t.TempDir()
	m := gitmaterializer.NewMaterializer(pool, root, nil)
	ref, err := m.InspectReleaseRefs(ctx, legacyProject, legacyVersion)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(publication{GitRoot: root, Refs: []gitmaterializer.ReleaseRefState{ref}})
	if err != nil {
		t.Fatal(err)
	}
	request := RebuildRequest{Plan: RebuildPlan(plan), Publication: raw}
	id, err := RebuildID(request)
	if err != nil {
		t.Fatal(err)
	}
	run := func(request RebuildRequest, wantResume bool, interrupt error) (RebuildOperation, gitmaterializer.PreparedRelease, error) {
		t.Helper()
		var pub publication
		if err := json.Unmarshal(request.Publication, &pub); err != nil {
			t.Fatal(err)
		}
		if len(pub.Refs) != 1 {
			t.Fatalf("persisted publication refs missing: %+v", pub)
		}
		materializer := gitmaterializer.NewMaterializer(pool, pub.GitRoot, nil).WithModuleHosts(request.Plan.ModuleHost, request.Plan.OntologyHost)
		service := NewService(NewPostgresStore(pool), pool, nil, WithModuleHosts(request.Plan.ModuleHost, request.Plan.OntologyHost))
		check := func(ctx context.Context, operation string, resume bool) error {
			if operation != id || resume != wantResume {
				t.Fatalf("operation identity or resume state changed: %s, %v", operation, resume)
			}
			return materializer.CheckReleaseRepoint(ctx, operation, pub.Refs[0], resume)
		}
		var prepared gitmaterializer.PreparedRelease
		publish := func(ctx context.Context, op RebuildOperation) (map[string]string, error) {
			var err error
			prepared, err = materializer.PrepareLegacyReleaseRepoint(ctx, op.ID, pub.Refs[0])
			if err != nil {
				return nil, err
			}
			if err := materializer.PublishReleaseRepoint(ctx, op.ID, prepared); err != nil {
				return nil, err
			}
			if interrupt != nil {
				return nil, interrupt
			}
			return map[string]string{prepared.Expected.ProjectID: prepared.Commit}, nil
		}
		op, err := service.RunLegacyRebuild(ctx, LegacyRebuildRequest{Plan: LegacyRebuildPlan(request.Plan), Publication: request.Publication}, check, publish)
		return op, prepared, err
	}
	interrupted := errors.New("process died after publishing refs, before marking completion")
	op, firstPrepared, err := run(request, false, interrupted)
	if !errors.Is(err, interrupted) || op.ID != id || op.Complete || len(op.Releases) != 1 {
		t.Fatalf("expected captured but unfinished operation: %+v, %v", op, err)
	}
	published, err := m.InspectReleaseRefs(ctx, legacyProject, legacyVersion)
	if err != nil || published.Branch != firstPrepared.Commit || published.Tag != firstPrepared.Tag {
		t.Fatalf("interruption did not follow actual publication: %+v, %v", published, err)
	}
	var pending bool
	mustScan(t, pool, `SELECT processed_at IS NULL FROM weave_change_set WHERE id=$1`, []any{op.Releases[0].ChangeSetID}, &pending)
	if !pending {
		t.Fatal("interrupted publication marked the outbox complete")
	}

	// This is the CLI resume boundary: load by operation ID, then execute the
	// stored request with fresh service/materializer instances, without a plan file.
	reader := NewService(NewPostgresStore(pool), pool, nil)
	stored, err := reader.GetRebuild(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if stored.ID != id || stored.Complete || stored.Request.Plan.Format != 2 {
		t.Fatalf("wrong persisted operation: %+v", stored)
	}
	if diff := cmp.Diff(request, stored.Request); diff != "" {
		t.Fatalf("journal changed the request: %s", diff)
	}
	resumedRequest := LegacyRebuildRequest{Plan: LegacyRebuildPlan(stored.Request.Plan), Publication: stored.Request.Publication}
	if got, err := LegacyRebuildID(resumedRequest); err != nil || got != id {
		t.Fatalf("persisted legacy request changed identity: %s, %v", got, err)
	}
	mustExec(t, pool, `UPDATE weave_projects SET ui_name='{"en":"Later draft"}' WHERE id='legacy-root'`)
	completed, resumedPrepared, err := run(stored.Request, true, nil)
	if err != nil || completed.ID != id || !completed.Complete {
		t.Fatalf("resume did not complete the original operation: %+v, %v", completed, err)
	}
	if diff := cmp.Diff(firstPrepared, resumedPrepared); diff != "" {
		t.Fatalf("resume changed prepared objects: %s", diff)
	}
	if diff := cmp.Diff(stored.Releases, completed.Releases); diff != "" {
		t.Fatalf("resume recaptured receipts or replaced outbox identities: %s", diff)
	}
	var archivedTitle, commit string
	mustScan(t, pool, `SELECT ui_name->>'en' FROM weave_projects_archive WHERE id='legacy-root' AND version_number='0.1.0'`, nil, &archivedTitle)
	if archivedTitle != "Current draft" {
		t.Fatalf("resume followed current drafts: %s", archivedTitle)
	}
	mustScan(t, pool, `SELECT processed_at IS NULL,git_commit_sha FROM weave_change_set WHERE id=$1`, []any{stored.Releases[0].ChangeSetID}, &pending, &commit)
	if pending || commit != firstPrepared.Commit {
		t.Fatalf("completion lost the original published commit: pending=%v, commit=%s", pending, commit)
	}
	finished, err := reader.GetRebuild(ctx, id)
	if err != nil || !finished.Complete {
		t.Fatalf("completion was not persisted: %+v, %v", finished, err)
	}
	if diff := cmp.Diff(stored.Request, finished.Request); diff != "" {
		t.Fatalf("completion changed the journal request: %s", diff)
	}
	// Completed replay must not check refs, prepare objects, or publish again.
	replayed, err := reader.RunLegacyRebuild(ctx, LegacyRebuildRequest{Plan: LegacyRebuildPlan(finished.Request.Plan), Publication: finished.Request.Publication},
		func(context.Context, string, bool) error {
			t.Fatal("completed operation checked refs again")
			return nil
		},
		func(context.Context, RebuildOperation) (map[string]string, error) {
			t.Fatal("completed operation published again")
			return nil, nil
		})
	if err != nil || !replayed.Complete {
		t.Fatalf("completed replay failed: %+v, %v", replayed, err)
	}
	if diff := cmp.Diff(finished, replayed); diff != "" {
		t.Fatalf("completed replay changed the operation: %s", diff)
	}
}
