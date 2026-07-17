package gitmaterializer_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/service/gitmaterializer"
)

func testPool(t *testing.T) *pgxpool.Pool {
	return testdb.Pool(t)
}

func TestInitProject_Integration(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available on PATH")
	}

	projectID := os.Getenv("TEST_PROJECT_ID")
	if projectID == "" {
		projectID = "LA"
	}

	pool := testPool(t)
	workDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))

	mat := gitmaterializer.NewMaterializer(pool, workDir, logger)
	if err := mat.InitProject(context.Background(), projectID); err != nil {
		if strings.Contains(err.Error(), "get project "+projectID+": no rows in result set") {
			t.Skipf("integration fixture project %s not available: %v", projectID, err)
		}
		t.Fatalf("InitProject: %v", err)
	}

	projectDir := filepath.Join(workDir, projectID)

	if _, err := os.Stat(filepath.Join(projectDir, ".git")); err != nil {
		t.Fatalf(".git directory not created: %v", err)
	}

	out, err := exec.Command("git", "-C", projectDir, "log", "--oneline").Output()
	if err != nil {
		t.Fatalf("git log: %v", err)
	}
	if len(out) == 0 {
		t.Error("expected at least one commit")
	}

	manifestPath := filepath.Join(projectDir, "project.yaml")
	if _, err := os.Stat(manifestPath); err != nil {
		t.Fatalf("expected project.yaml to exist: %v", err)
	}
	payload, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read project.yaml: %v", err)
	}
	if len(payload) == 0 {
		t.Fatal("expected project.yaml to be non-empty")
	}

	adoptionsIndexPath := filepath.Join(projectDir, "adoptions", "index.yaml")
	if _, err := os.Stat(adoptionsIndexPath); err != nil {
		t.Fatalf("expected adoptions/index.yaml to exist: %v", err)
	}

	forksIndexPath := filepath.Join(projectDir, "forks", "index.yaml")
	if _, err := os.Stat(forksIndexPath); err != nil {
		t.Fatalf("expected forks/index.yaml to exist: %v", err)
	}

	modPath := filepath.Join(projectDir, "pletka.mod")
	if _, err := os.Stat(modPath); err != nil {
		t.Fatalf("expected pletka.mod to exist: %v", err)
	}
}

func TestInitProject_SelfContainedSnapshot_Integration(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available on PATH")
	}

	projectID := "TPC"

	pool := testPool(t)
	workDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))

	mat := gitmaterializer.NewMaterializer(pool, workDir, logger)
	if err := mat.InitProjectWithOptions(context.Background(), projectID, gitmaterializer.InitProjectOptions{
		SelfContained: true,
	}); err != nil {
		if strings.Contains(err.Error(), "get project "+projectID+": no rows in result set") {
			t.Skipf("self-contained snapshot fixture project %s not available: %v", projectID, err)
		}
		if strings.Contains(err.Error(), "is missing rdf_content") {
			t.Skipf("self-contained snapshot requires ontology rdf_content in fixture data: %v", err)
		}
		t.Fatalf("InitProjectWithOptions(self-contained): %v", err)
	}

	projectDir := filepath.Join(workDir, projectID)
	vendorProjects := filepath.Join(projectDir, "vendor", "projects")
	if _, err := os.Stat(vendorProjects); err != nil {
		t.Fatalf("expected vendor/projects to exist: %v", err)
	}
	vendorOntologies := filepath.Join(projectDir, "vendor", "ontologies")
	if _, err := os.Stat(vendorOntologies); err != nil {
		t.Fatalf("expected vendor/ontologies to exist: %v", err)
	}
	if !hasVendoredProjectManifest(t, filepath.Join(projectDir, "vendor", "projects")) {
		t.Fatalf("expected vendored parent project manifest")
	}
	if _, err := os.Stat(filepath.Join(projectDir, "vendor", "projects", "pletka.io", "orgs", "takin-solutions", "projects", "LA", ".git")); !os.IsNotExist(err) {
		t.Fatalf("expected vendored parent project to omit .git, got: %v", err)
	}
	if _, err := os.Stat(filepath.Join(projectDir, "vendor", "ontologies", "ontology.pletka.io", "cidoc-crm", "7.1.3", "ontology.yaml")); err != nil {
		t.Fatalf("expected vendored ontology manifest: %v", err)
	}
	if _, err := os.Stat(filepath.Join(projectDir, "pletka.sum")); err != nil {
		t.Fatalf("expected pletka.sum to exist: %v", err)
	}
}

func TestProjectSnapshot_RoundTripHydrate_Integration(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available on PATH")
	}

	sourceProjectID := os.Getenv("TEST_ROUNDTRIP_PROJECT_ID")
	if sourceProjectID == "" {
		sourceProjectID = "TPC"
	}
	targetProjectID := os.Getenv("TEST_ROUNDTRIP_TARGET_PROJECT_ID")
	if targetProjectID == "" {
		targetProjectID = sourceProjectID + "_RT"
	}

	ctx := context.Background()
	pool := testPool(t)
	cleanupHydratedProject(t, pool, targetProjectID)
	t.Cleanup(func() {
		cleanupHydratedProject(t, pool, targetProjectID)
	})

	sourceDir := t.TempDir()
	targetDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))

	sourceMat := gitmaterializer.NewMaterializer(pool, sourceDir, logger)
	if err := sourceMat.InitProject(ctx, sourceProjectID); err != nil {
		if strings.Contains(err.Error(), "get project "+sourceProjectID+": no rows in result set") {
			t.Skipf("round-trip fixture project %s not available: %v", sourceProjectID, err)
		}
		t.Fatalf("InitProject(source): %v", err)
	}

	sourceRoot := filepath.Join(sourceDir, sourceProjectID)
	sourceSnapshot, err := gitmaterializer.LoadProjectSnapshot(sourceRoot)
	if err != nil {
		t.Fatalf("LoadProjectSnapshot(source): %v", err)
	}
	if targetProjectID != sourceProjectID && snapshotHasLocalOwnedEntities(sourceSnapshot) {
		t.Skipf("round-trip retargeting with local semantic IDs is not supported by default for %s -> %s", sourceProjectID, targetProjectID)
	}

	hydrator := gitmaterializer.NewMaterializer(pool, "", logger)
	plan, err := hydrator.HydrateProjectSnapshotAs(ctx, sourceRoot, targetProjectID)
	if err != nil {
		t.Fatalf("HydrateProjectSnapshotAs: %v", err)
	}
	if plan.ProjectID != targetProjectID {
		t.Fatalf("expected restore plan project %s, got %s", targetProjectID, plan.ProjectID)
	}

	targetMat := gitmaterializer.NewMaterializer(pool, targetDir, logger)
	if err := targetMat.InitProject(ctx, targetProjectID); err != nil {
		t.Fatalf("InitProject(target): %v", err)
	}

	targetSnapshot, err := gitmaterializer.LoadProjectSnapshot(filepath.Join(targetDir, targetProjectID))
	if err != nil {
		t.Fatalf("LoadProjectSnapshot(target): %v", err)
	}

	got := comparableSnapshot(targetSnapshot)
	want, err := comparableSnapshotRetargeted(sourceSnapshot, targetProjectID)
	if err != nil {
		t.Fatalf("comparableSnapshotRetargeted: %v", err)
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("round-trip snapshot mismatch (-want +got):\n%s", diff)
	}
}

func cleanupHydratedProject(t *testing.T, pool *pgxpool.Pool, projectID string) {
	t.Helper()
	ctx := context.Background()
	mustExecCleanup(t, pool, ctx, `
		DELETE FROM weave_override_refs
		WHERE override_id IN (
			SELECT id FROM weave_field_overrides WHERE project_id = $1
		)
	`, projectID)
	mustExecCleanup(t, pool, ctx, `DELETE FROM weave_field_overrides WHERE project_id = $1`, projectID)
	mustExecCleanup(t, pool, ctx, `DELETE FROM weave_entity_forks WHERE project_id = $1`, projectID)
	mustExecCleanup(t, pool, ctx, `DELETE FROM weave_adoptions WHERE project_id = $1`, projectID)
	mustExecCleanup(t, pool, ctx, `DELETE FROM weave_collections WHERE project_id = $1`, projectID)
	mustExecCleanup(t, pool, ctx, `DELETE FROM weave_models WHERE project_id = $1`, projectID)
	mustExecCleanup(t, pool, ctx, `DELETE FROM weave_fields WHERE project_id = $1`, projectID)
	mustExecCleanup(t, pool, ctx, `DELETE FROM weave_categories WHERE project_id = $1`, projectID)
	mustExecCleanup(t, pool, ctx, `DELETE FROM weave_namespace_bindings WHERE project_id = $1`, projectID)
	mustExecCleanup(t, pool, ctx, `DELETE FROM weave_project_ontology_versions WHERE project_id = $1`, projectID)
	mustExecCleanup(t, pool, ctx, `DELETE FROM weave_project_inheritance WHERE project_id = $1 OR parent_project_id = $1`, projectID)
	mustExecCleanup(t, pool, ctx, `DELETE FROM weave_releases WHERE project_id = $1`, projectID)
	mustExecCleanup(t, pool, ctx, `DELETE FROM weave_projects WHERE id = $1`, projectID)
}

func mustExecCleanup(t *testing.T, pool *pgxpool.Pool, ctx context.Context, sql string, projectID string) {
	t.Helper()
	if _, err := pool.Exec(ctx, sql, projectID); err != nil {
		t.Fatalf("cleanup %q for %s: %v", sql, projectID, err)
	}
}

func hasVendoredProjectManifest(t *testing.T, root string) bool {
	t.Helper()
	found := false
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return nil
		}
		if d.Name() == "project.yaml" {
			found = true
		}
		return nil
	})
	return found
}

type roundTripComparable struct {
	Manifest  gitmaterializer.ProjectManifestFile  `json:"manifest"`
	Mod       *gitmaterializer.PletkaModFile       `json:"mod,omitempty"`
	Entities  gitmaterializer.SnapshotEntityTree   `json:"entities"`
	Adoptions *gitmaterializer.AdoptionManifestSet `json:"adoptions,omitempty"`
	Forks     *gitmaterializer.ForkManifestSet     `json:"forks,omitempty"`
}

func comparableSnapshot(snapshot *gitmaterializer.ProjectSnapshot) roundTripComparable {
	return roundTripComparable{
		Manifest:  snapshot.Manifest,
		Mod:       snapshot.Mod,
		Entities:  snapshot.Entities,
		Adoptions: snapshot.Adoptions,
		Forks:     snapshot.Forks,
	}
}

func comparableSnapshotRetargeted(snapshot *gitmaterializer.ProjectSnapshot, targetProjectID string) (roundTripComparable, error) {
	if snapshot == nil {
		return roundTripComparable{}, nil
	}
	raw, err := json.Marshal(snapshot)
	if err != nil {
		return roundTripComparable{}, err
	}
	var cloned gitmaterializer.ProjectSnapshot
	if err := json.Unmarshal(raw, &cloned); err != nil {
		return roundTripComparable{}, err
	}
	sourceProjectID := cloned.Manifest.Project.ID
	cloned.Manifest.Project.ID = targetProjectID
	if cloned.Mod != nil {
		cloned.Mod.Module.ProjectID = targetProjectID
		suffix := "/projects/" + sourceProjectID
		if strings.HasSuffix(cloned.Mod.Module.Path, suffix) {
			cloned.Mod.Module.Path = strings.TrimSuffix(cloned.Mod.Module.Path, suffix) + "/projects/" + targetProjectID
		}
	}
	if cloned.Adoptions != nil {
		for path, receipt := range cloned.Adoptions.Receipts {
			for i := range receipt.Adoption.Contexts {
				if receipt.Adoption.Contexts[i].ContextEntityType == "project" && receipt.Adoption.Contexts[i].ContextEntityID == sourceProjectID {
					receipt.Adoption.Contexts[i].ContextEntityID = targetProjectID
				}
			}
			cloned.Adoptions.Receipts[path] = receipt
		}
	}
	return comparableSnapshot(&cloned), nil
}

func snapshotHasLocalOwnedEntities(snapshot *gitmaterializer.ProjectSnapshot) bool {
	if snapshot == nil {
		return false
	}
	for _, item := range snapshot.Entities.Categories {
		if item.OwnerType == "" || item.OwnerID == "" || item.OwnerID == snapshot.Manifest.Project.ID {
			return true
		}
	}
	trees := [][]gitmaterializer.SnapshotEntityFile{
		snapshot.Entities.Fields,
		snapshot.Entities.Models,
		snapshot.Entities.Collections,
	}
	for _, items := range trees {
		for _, item := range items {
			if item.OwnerType == "" || item.OwnerID == "" || item.OwnerID == snapshot.Manifest.Project.ID {
				return true
			}
		}
	}
	return false
}
