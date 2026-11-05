//go:build integration

package release

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/database/readsession"
)

// seedPreserve archives the seeded drafts at 0.1.0, then moves the drafts on:
// one extra model and one credit in the root draft, and an adoption that only
// the archive knows about (the draft dropped that source). The release row
// gets an explicit old date so the preserved metadata is distinguishable.
func seedPreserve(t *testing.T) (context.Context, *pgxpool.Pool, *Service, time.Time) {
	t.Helper()
	ctx, p, svc := seedLegacyRebuild(t, true)
	cut := time.Date(2025, 7, 1, 10, 0, 0, 0, time.UTC)
	mustExec(t, p, `UPDATE weave_releases SET created_at=$1 WHERE project_id IN ('legacy-root','legacy-source')`, cut)
	testdb.ArchiveDraft(t, p, "legacy-source", "0.1.0")
	testdb.ArchiveDraft(t, p, "legacy-root", "0.1.0")
	for _, sql := range []string{
		`INSERT INTO weave_adoptions_archive(project_id,context_entity_type,context_entity_id,entity_type,source_project_id,source_entity_id,adopted_at,version_number)
		 VALUES ('legacy-root','model','legacy-model','field','legacy-source','legacy-source-field',now(),'0.1.0')`,
		`INSERT INTO weave_models(id,project_id) VALUES ('later-model','legacy-root')`,
		`INSERT INTO weave_project_actors(project_id,actor_id,role) VALUES ('legacy-root','legacy-owner','editor')`,
	} {
		mustExec(t, p, sql)
	}
	return ctx, p, svc, cut
}

func TestPreserveLegacyArchiveKeepsOriginalRows(t *testing.T) {
	ctx, p, svc, cut := seedPreserve(t)
	plan, err := svc.PlanLegacyRebuild(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Format != 2 || plan.Source != "preserved-archive" || len(plan.Projects) != 2 || plan.Projects[0].ProjectID != "legacy-source" {
		t.Fatalf("wrong plan: %+v", plan)
	}
	root := plan.Projects[1]
	if !root.CreatedAt.Equal(cut) || root.CreatedByID != "legacy-owner" || len(root.ArchiveDigest) != 64 {
		t.Fatalf("plan lost catalog metadata or digest: %+v", root)
	}
	op, err := svc.RunLegacyRebuild(ctx, LegacyRebuildRequest{Plan: plan}, checkRebuild, finishRebuild)
	if err != nil || !op.Complete {
		t.Fatalf("preserve failed: %+v %v", op, err)
	}
	var later, kept, credits, draftLater int
	mustScan(t, p, `SELECT
		(SELECT count(*) FROM weave_models_archive WHERE id='later-model' AND version_number='0.1.0'),
		(SELECT count(*) FROM weave_models_archive WHERE id='legacy-model' AND version_number='0.1.0'),
		(SELECT count(*) FROM weave_project_actors_archive WHERE project_id='legacy-root' AND version_number='0.1.0'),
		(SELECT count(*) FROM weave_models WHERE id='later-model')`, nil, &later, &kept, &credits, &draftLater)
	if later != 0 || kept != 1 || credits != 0 || draftLater != 1 {
		t.Fatalf("archive followed the draft: later=%d kept=%d credits=%d draft=%d", later, kept, credits, draftLater)
	}
	var createdAt time.Time
	var createdBy string
	var contract, ownership int
	var receiptMatches bool
	mustScan(t, p, `SELECT created_at, created_by_id, content_capture_version, archive_ownership_version,
		content_capture_receipt = weave_legacy_archive_receipt(project_id, version)
		FROM weave_releases WHERE project_id='legacy-root' AND version='0.1.0'`, nil, &createdAt, &createdBy, &contract, &ownership, &receiptMatches)
	if !createdAt.Equal(cut) || createdBy != "legacy-owner" || contract != 2 || ownership != 1 || !receiptMatches {
		t.Fatalf("release metadata or receipt wrong: %v %s %d %d %v", createdAt, createdBy, contract, ownership, receiptMatches)
	}
	var pinned, adoptionPinned int
	var source, operation string
	mustScan(t, p, `SELECT
		(SELECT count(*) FROM weave_release_project_dependencies WHERE project_id='legacy-root' AND version_number='0.1.0' AND source_project_id='legacy-source' AND source_version='0.1.0'),
		(SELECT count(*) FROM weave_adoptions_archive WHERE project_id='legacy-root' AND version_number='0.1.0' AND source_version='0.1.0'),
		(SELECT document->>'source' FROM weave_release_imports WHERE project_id='legacy-root' AND version_number='0.1.0'),
		(SELECT document->>'rebuild_operation' FROM weave_release_imports WHERE project_id='legacy-root' AND version_number='0.1.0')`,
		nil, &pinned, &adoptionPinned, &source, &operation)
	if pinned != 1 || adoptionPinned != 1 || source != "preserved-legacy-archive" || operation != op.ID {
		t.Fatalf("dependency or ledger evidence wrong: %d %d %q %q", pinned, adoptionPinned, source, operation)
	}
	err = readsession.NewFactory(p).WithSourceAuthorization(auth.CheckProjectRead).Read(ctx, auth.Release("0.1.0"), "legacy-root", func(s *readsession.Session) error {
		origin, err := s.Origin(ctx)
		if err != nil {
			return err
		}
		if !origin.Imported {
			t.Error("preserved release reads as a local cut")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	readLegacySnapshot(ctx, t, p) // the release is readable on the legacy contract
	if _, err := svc.PlanLegacyRebuild(ctx, []string{"legacy-root"}); err == nil {
		t.Fatal("preserved release selected again")
	}
}

func TestPreserveRefusesChangedArchiveAndWrongSource(t *testing.T) {
	ctx, p, svc, _ := seedPreserve(t)
	plan, err := svc.PlanLegacyRebuild(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	mustExec(t, p, `UPDATE weave_fields_archive SET description='{"en":"edited after plan"}' WHERE project_id='legacy-root' AND version_number='0.1.0'`)
	_, err = svc.RunLegacyRebuild(ctx, LegacyRebuildRequest{Plan: plan}, checkRebuild, finishRebuild)
	if err == nil || !strings.Contains(err.Error(), "inputs or configuration changed") {
		t.Fatalf("changed archive row accepted: %v", err)
	}
	var preserved int
	mustScan(t, p, `SELECT count(*) FROM weave_releases WHERE archive_ownership_version=1`, nil, &preserved)
	if preserved != 0 {
		t.Fatal("refused apply left a preserved release")
	}
	plan, err = svc.PlanLegacyRebuild(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	plan.Source = "current-drafts"
	if _, err := svc.RunLegacyRebuild(ctx, LegacyRebuildRequest{Plan: plan}, checkRebuild, finishRebuild); err == nil {
		t.Fatal("legacy contract accepted a drafts plan")
	}
	verified, err := svc.PlanRebuild(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	verified.Source = "preserved-archive"
	if _, err := svc.RunRebuild(ctx, RebuildRequest{Plan: verified}, checkRebuild, finishRebuild); err == nil {
		t.Fatal("verified contract accepted a preserve plan")
	}
}
