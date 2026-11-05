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
	mustExec(t, p, "UPDATE weave_projects_archive SET effective_concept_namespace=NULL, enforce_concept_lists=NULL WHERE id='legacy-root' AND version_number='0.1.0'")
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
	var effective string
	var enforce bool
	mustScan(t, p, `SELECT effective_concept_namespace, enforce_concept_lists FROM weave_projects_archive WHERE id='legacy-root' AND version_number='0.1.0'`, nil, &effective, &enforce)
	if effective != "https://vocab.pletka.io/" || enforce {
		t.Fatalf("absent archived concept policy not filled: %q %v", effective, enforce)
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
	var source, operation, filledPolicy, filledPins string
	mustScan(t, p, `SELECT
		(SELECT count(*) FROM weave_release_project_dependencies WHERE project_id='legacy-root' AND version_number='0.1.0' AND source_project_id='legacy-source' AND source_version='0.1.0'),
		(SELECT count(*) FROM weave_adoptions_archive WHERE project_id='legacy-root' AND version_number='0.1.0' AND source_version='0.1.0'),
		(SELECT document->>'source' FROM weave_release_imports WHERE project_id='legacy-root' AND version_number='0.1.0'),
		(SELECT document->>'rebuild_operation' FROM weave_release_imports WHERE project_id='legacy-root' AND version_number='0.1.0'),
		(SELECT document->'filled'->>'concept_policy' FROM weave_release_imports WHERE project_id='legacy-root' AND version_number='0.1.0'),
		(SELECT document->'filled'->>'adoption_pins' FROM weave_release_imports WHERE project_id='legacy-root' AND version_number='0.1.0')`,
		nil, &pinned, &adoptionPinned, &source, &operation, &filledPolicy, &filledPins)
	if pinned != 1 || adoptionPinned != 1 || source != "preserved-legacy-archive" || operation != op.ID || filledPolicy != "1" || filledPins != "1" {
		t.Fatalf("dependency or ledger evidence wrong: %d %d %q %q filled=%q/%q", pinned, adoptionPinned, source, operation, filledPolicy, filledPins)
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

// seedArchiveOnlySource adds a released, archived source project whose id
// sorts after legacy-root. Nothing in the live drafts references it; each
// test adds one archive-only reference from legacy-root@0.1.0.
func seedArchiveOnlySource(t *testing.T) (context.Context, *pgxpool.Pool, *Service) {
	t.Helper()
	ctx, p, svc := seedLegacyRebuild(t, false)
	for _, sql := range []string{
		`INSERT INTO weave_projects(id,owner_id) VALUES ('legacy-zsource','legacy-owner')`,
		`INSERT INTO weave_fields(id,project_id,ontology_scope) VALUES ('legacy-zsource-field','legacy-zsource','{"prefix":"crm","local_name":"E1"}')`,
		`INSERT INTO weave_releases(project_id,version,created_by_id) VALUES ('legacy-zsource','0.1.0','legacy-owner')`,
		`INSERT INTO weave_project_ontology_versions(project_id,ontology_version_id,added_at,is_primary) VALUES ('legacy-zsource','legacy-crm-7.1.3',now(),true)`,
	} {
		mustExec(t, p, sql)
	}
	testdb.ArchiveDraft(t, p, "legacy-zsource", "0.1.0")
	return ctx, p, svc
}

// assertArchiveSourceFirst plans the whole selection, checks the archive-only
// source is captured before its consumer and that the run pins it.
func assertArchiveSourceFirst(ctx context.Context, t *testing.T, p *pgxpool.Pool, svc *Service) {
	t.Helper()
	plan, err := svc.PlanLegacyRebuild(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Projects) != 2 || plan.Projects[0].ProjectID != "legacy-zsource" || plan.Projects[1].ProjectID != "legacy-root" {
		t.Fatalf("archive-only source not ordered first: %+v", plan.Projects)
	}
	op, err := svc.RunLegacyRebuild(ctx, LegacyRebuildRequest{Plan: plan}, checkRebuild, finishRebuild)
	if err != nil || !op.Complete {
		t.Fatalf("preserve failed: %+v %v", op, err)
	}
	var pinned int
	mustScan(t, p, `SELECT count(*) FROM weave_release_project_dependencies
		WHERE project_id='legacy-root' AND version_number='0.1.0' AND source_project_id='legacy-zsource' AND source_version='0.1.0'`, nil, &pinned)
	if pinned != 1 {
		t.Fatalf("archive-only source not pinned: %d", pinned)
	}
}

func TestPreserveOrdersArchiveOnlySourcesFirst(t *testing.T) {
	ctx, p, svc := seedArchiveOnlySource(t)
	mustExec(t, p, `INSERT INTO weave_adoptions_archive(project_id,context_entity_type,context_entity_id,entity_type,source_project_id,source_entity_id,adopted_at,version_number)
		VALUES ('legacy-root','model','legacy-model','field','legacy-zsource','legacy-zsource-field',now(),'0.1.0')`)
	if _, err := svc.PlanLegacyRebuild(ctx, []string{"legacy-root"}); err == nil || !strings.Contains(err.Error(), "include source project legacy-zsource") {
		t.Fatalf("consumer planned without its archived source: %v", err)
	}
	assertArchiveSourceFirst(ctx, t, p, svc)
}

func TestPreserveRefusesArchivedDraftFollowingAtPlan(t *testing.T) {
	ctx, p, svc := seedArchiveOnlySource(t)
	mustExec(t, p, `INSERT INTO weave_project_inheritance_archive(project_id,parent_project_id,version_number,source_mode)
		VALUES ('legacy-root','legacy-zsource','0.1.0','draft')`)
	_, err := svc.PlanLegacyRebuild(ctx, nil)
	if err == nil || !strings.Contains(err.Error(), "legacy-root@0.1.0 -> legacy-zsource") {
		t.Fatalf("archived draft-following inheritance planned: %v", err)
	}
	var changed int
	mustScan(t, p, `SELECT count(*) FROM weave_releases WHERE content_capture_version<>0`, nil, &changed)
	if changed != 0 {
		t.Fatalf("refused plan changed %d releases", changed)
	}
}

func TestPreserveResolvesOwnerFromArchiveOnly(t *testing.T) {
	ctx, p, svc := seedArchiveOnlySource(t)
	mustExec(t, p, `INSERT INTO weave_field_overrides_archive(id,field_id,project_id,entity_type,entity_id,version_number)
		VALUES (990001,'legacy-zsource-field','legacy-root','model','legacy-model','0.1.0')`)
	mustExec(t, p, `DELETE FROM weave_fields WHERE id='legacy-zsource-field'`)
	assertArchiveSourceFirst(ctx, t, p, svc)
}
