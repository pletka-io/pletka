package weave_test

import (
	"context"
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/ids"
	"github.com/pletka-io/pletka/pkg/weave"
)

func TestProjectOntologyVersionStore_ImplementsInterface(t *testing.T) {
	pool := testPool(t)
	store := weave.NewPostgresStore(pool)
	var _ domain.ProjectOntologyVersionStore = store.ProjectOntologyVersions()
}

func TestProjectOntologyVersionStore_CRUD(t *testing.T) {
	pool := testPool(t)
	store := weave.NewPostgresStore(pool)
	povs := store.ProjectOntologyVersions()
	ctx := context.Background()

	projectID := "TEST_PROJ_POV_CRUD"
	versionID := "TEST_OV_POV_CRUD"

	// Re-run safety: delete any leftover rows before the test starts.
	_, _ = pool.Exec(ctx, `DELETE FROM weave_project_ontology_versions WHERE project_id LIKE 'TEST_%'`)

	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM weave_project_ontology_versions WHERE project_id = $1`, projectID)
	})

	// --- Create ---
	link := &domain.ProjectOntologyVersion{
		ProjectID:         projectID,
		OntologyVersionID: versionID,
		IsPrimary:         false,
		UsageNotes:        "initial notes",
	}

	if err := povs.Create(ctx, link); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// --- Get ---
	got, err := povs.Get(ctx, projectID, versionID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil {
		t.Fatal("Get returned nil")
	}
	if got.OntologyVersionID != versionID {
		t.Errorf("OntologyVersionID = %q, want %q", got.OntologyVersionID, versionID)
	}
	if got.UsageNotes != "initial notes" {
		t.Errorf("UsageNotes = %q, want %q", got.UsageNotes, "initial notes")
	}

	// --- Update ---
	got.UsageNotes = "updated notes"
	got.IsPrimary = true
	if err := povs.Update(ctx, got); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got2, err := povs.Get(ctx, projectID, versionID)
	if err != nil {
		t.Fatalf("Get after update: %v", err)
	}
	if got2.UsageNotes != "updated notes" {
		t.Errorf("UsageNotes after update = %q, want %q", got2.UsageNotes, "updated notes")
	}
	if !got2.IsPrimary {
		t.Error("IsPrimary should be true after update")
	}

	// --- List ---
	list, err := povs.List(ctx, projectID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("List returned %d items, want 1", len(list))
	}

	// --- Delete ---
	if err := povs.Delete(ctx, projectID, versionID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	list2, err := povs.List(ctx, projectID)
	if err != nil {
		t.Fatalf("List after delete: %v", err)
	}
	if len(list2) != 0 {
		t.Errorf("List after delete returned %d items, want 0", len(list2))
	}
}

func TestProjectOntologyVersionStore_SetPrimary_Singleton(t *testing.T) {
	pool := testPool(t)
	store := weave.NewPostgresStore(pool)
	povs := store.ProjectOntologyVersions()
	ctx := context.Background()

	projectID := "TEST_PROJ_POV_SETPRIMARY"
	versionID1 := "TEST_OV_POV_V1"
	versionID2 := "TEST_OV_POV_V2"

	// Re-run safety: delete any leftover rows before the test starts.
	_, _ = pool.Exec(ctx, `DELETE FROM weave_project_ontology_versions WHERE project_id LIKE 'TEST_%'`)

	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM weave_project_ontology_versions WHERE project_id = $1`, projectID)
	})

	// Link two versions, both not primary.
	if err := povs.Create(ctx, &domain.ProjectOntologyVersion{
		ProjectID:         projectID,
		OntologyVersionID: versionID1,
		IsPrimary:         false,
	}); err != nil {
		t.Fatalf("Create v1: %v", err)
	}
	if err := povs.Create(ctx, &domain.ProjectOntologyVersion{
		ProjectID:         projectID,
		OntologyVersionID: versionID2,
		IsPrimary:         false,
	}); err != nil {
		t.Fatalf("Create v2: %v", err)
	}

	// SetPrimary to v1.
	if err := povs.SetPrimary(ctx, projectID, versionID1); err != nil {
		t.Fatalf("SetPrimary v1: %v", err)
	}
	v1, _ := povs.Get(ctx, projectID, versionID1)
	if !v1.IsPrimary {
		t.Error("v1 should be primary after SetPrimary(v1)")
	}

	// SetPrimary to v2 — v1 should become non-primary.
	if err := povs.SetPrimary(ctx, projectID, versionID2); err != nil {
		t.Fatalf("SetPrimary v2: %v", err)
	}

	v1After, _ := povs.Get(ctx, projectID, versionID1)
	v2After, _ := povs.Get(ctx, projectID, versionID2)
	if v1After.IsPrimary {
		t.Error("v1 should not be primary after SetPrimary(v2)")
	}
	if !v2After.IsPrimary {
		t.Error("v2 should be primary after SetPrimary(v2)")
	}
}

func TestProjectOntologyVersionStore_CountAndSampleUsage(t *testing.T) {
	pool := testPool(t)
	store := weave.NewPostgresStore(pool)
	povs := store.ProjectOntologyVersions()
	ctx := context.Background()

	projectID := "TEST_PROJ_POV_USAGE"
	versionID := "TEST_OV_POV_USAGE"
	fieldID := ids.GenerateULID()

	// Re-run safety: delete any leftover rows before the test starts.
	_, _ = pool.Exec(ctx, `DELETE FROM weave_project_ontology_versions WHERE project_id LIKE 'TEST_%'`)

	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM weave_path_elements WHERE field_id = $1`, fieldID)
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM weave_fields WHERE id = $1`, fieldID)
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM weave_project_ontology_versions WHERE project_id = $1`, projectID)
	})

	// Seed a weave_fields row.
	if _, err := pool.Exec(ctx,
		`INSERT INTO weave_fields (id, project_id, status, semantic_id, system_name, ui_name)
		 VALUES ($1, $2, 'draft', 'TST.1', 'test_field', '{"en":"Test Field"}'::jsonb)`,
		fieldID, projectID,
	); err != nil {
		t.Fatalf("seed weave_fields: %v", err)
	}

	// Seed a weave_path_elements row referencing the version.
	if _, err := pool.Exec(ctx,
		`INSERT INTO weave_path_elements (field_id, position, local_name, type, ontology_version_id)
		 VALUES ($1, 1, 'E21_Person', 'class', $2)`,
		fieldID, versionID,
	); err != nil {
		t.Fatalf("seed weave_path_elements: %v", err)
	}

	// CountPathElementUsage should return 1.
	count, err := povs.CountPathElementUsage(ctx, projectID, versionID)
	if err != nil {
		t.Fatalf("CountPathElementUsage: %v", err)
	}
	if count != 1 {
		t.Errorf("CountPathElementUsage = %d, want 1", count)
	}

	// SamplePathElementFields should return 1 entry.
	samples, err := povs.SamplePathElementFields(ctx, projectID, versionID, 10)
	if err != nil {
		t.Fatalf("SamplePathElementFields: %v", err)
	}
	if len(samples) != 1 {
		t.Fatalf("SamplePathElementFields returned %d samples, want 1", len(samples))
	}
	if samples[0].ID != fieldID {
		t.Errorf("sample ID = %q, want %q", samples[0].ID, fieldID)
	}
	if samples[0].SemanticID != "TST.1" {
		t.Errorf("sample SemanticID = %q, want %q", samples[0].SemanticID, "TST.1")
	}
	if samples[0].SystemName != "test_field" {
		t.Errorf("sample SystemName = %q, want %q", samples[0].SystemName, "test_field")
	}
	if samples[0].UIName.Get("en") != "Test Field" {
		t.Errorf("sample UIName[en] = %q, want %q", samples[0].UIName.Get("en"), "Test Field")
	}

	// CountPathElementUsage with a different version should return 0.
	count2, err := povs.CountPathElementUsage(ctx, projectID, "OTHER_VERSION")
	if err != nil {
		t.Fatalf("CountPathElementUsage other version: %v", err)
	}
	if count2 != 0 {
		t.Errorf("CountPathElementUsage other version = %d, want 0", count2)
	}
}

func TestProjectOntologyVersionStore_ListWithCounts(t *testing.T) {
	pool := testPool(t)
	store := weave.NewPostgresStore(pool)
	povs := store.ProjectOntologyVersions()
	ctx := context.Background()

	projectID := "TEST_PROJ_POV_LWC"
	versionID := "TEST_OV_POV_LWC"
	fieldID := ids.GenerateULID()

	// Re-run safety.
	_, _ = pool.Exec(ctx, `DELETE FROM weave_project_ontology_versions WHERE project_id LIKE 'TEST_%'`)

	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM weave_path_elements WHERE field_id = $1`, fieldID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM weave_fields WHERE id = $1`, fieldID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM weave_project_ontology_versions WHERE project_id = $1`, projectID)
	})

	// Seed a link.
	if err := povs.Create(ctx, &domain.ProjectOntologyVersion{
		ProjectID:         projectID,
		OntologyVersionID: versionID,
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Seed a field + path element referencing the version.
	if _, err := pool.Exec(ctx,
		`INSERT INTO weave_fields (id, project_id, status, semantic_id, system_name, ui_name)
		 VALUES ($1, $2, 'draft', 'TST.LWC', 'test_lwc', '{"en":"LWC Field"}'::jsonb)`,
		fieldID, projectID,
	); err != nil {
		t.Fatalf("seed weave_fields: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO weave_path_elements (field_id, position, local_name, type, ontology_version_id)
		 VALUES ($1, 1, 'E21_Person', 'class', $2)`,
		fieldID, versionID,
	); err != nil {
		t.Fatalf("seed weave_path_elements: %v", err)
	}

	results, err := povs.ListWithCounts(ctx, projectID)
	if err != nil {
		t.Fatalf("ListWithCounts: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("ListWithCounts returned %d items, want 1", len(results))
	}
	if results[0].Link.OntologyVersionID != versionID {
		t.Errorf("OntologyVersionID = %q, want %q", results[0].Link.OntologyVersionID, versionID)
	}
	if results[0].UsageCount != 1 {
		t.Errorf("UsageCount = %d, want 1", results[0].UsageCount)
	}
}

func TestProjectOntologyVersionStore_ListGrouped(t *testing.T) {
	pool := testPool(t)
	store := weave.NewPostgresStore(pool)
	povs := store.ProjectOntologyVersions()
	ctx := context.Background()

	projectID := "TEST_PROJ_POV_GROUPED"
	baseOntologyID := "TEST_ONTOLOGY_GROUPED"
	baseVersionID := "TEST_OV_GROUPED_BASE"
	extVersionID := "TEST_OV_GROUPED_EXT"

	// Re-run safety.
	_, _ = pool.Exec(ctx, `DELETE FROM weave_project_ontology_versions WHERE project_id LIKE 'TEST_%'`)
	_, _ = pool.Exec(ctx, `DELETE FROM weave_ontology_versions WHERE id LIKE 'TEST_%'`)
	_, _ = pool.Exec(ctx, `DELETE FROM weave_ontologies WHERE id LIKE 'TEST_%'`)

	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM weave_project_ontology_versions WHERE project_id = $1`, projectID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM weave_ontology_versions WHERE id LIKE 'TEST_%'`)
		_, _ = pool.Exec(context.Background(), `DELETE FROM weave_ontologies WHERE id LIKE 'TEST_%'`)
	})

	// Seed an ontology row so the versions can FK against it.
	if _, err := pool.Exec(ctx,
		`INSERT INTO weave_ontologies (id, prefix, namespace, name, ontology_type)
		 VALUES ($1, 'test_grouped', 'http://test.local/grouped/', 'Test Grouped', 'base')`,
		baseOntologyID,
	); err != nil {
		t.Fatalf("seed ontology: %v", err)
	}

	// Seed a base ontology_version (no compatible_base_versions).
	if _, err := pool.Exec(ctx,
		`INSERT INTO weave_ontology_versions (id, ontology_id, version_string, created_at, updated_at)
		 VALUES ($1, $2, '1.0', now(), now())`,
		baseVersionID, baseOntologyID,
	); err != nil {
		t.Fatalf("seed base ontology_version: %v", err)
	}

	// Seed an extension ontology_version (compatible_base_versions = ARRAY[baseVersionID]).
	if _, err := pool.Exec(ctx,
		`INSERT INTO weave_ontology_versions (id, ontology_id, version_string, compatible_base_versions, created_at, updated_at)
		 VALUES ($1, $2, '1.0-ext', ARRAY[$3]::text[], now(), now())`,
		extVersionID, baseOntologyID, baseVersionID,
	); err != nil {
		t.Fatalf("seed extension ontology_version: %v", err)
	}

	// Link both in the project.
	if err := povs.Create(ctx, &domain.ProjectOntologyVersion{
		ProjectID:         projectID,
		OntologyVersionID: baseVersionID,
		IsPrimary:         true,
	}); err != nil {
		t.Fatalf("Create base link: %v", err)
	}
	if err := povs.Create(ctx, &domain.ProjectOntologyVersion{
		ProjectID:         projectID,
		OntologyVersionID: extVersionID,
	}); err != nil {
		t.Fatalf("Create ext link: %v", err)
	}

	groups, err := povs.ListGrouped(ctx, projectID)
	if err != nil {
		t.Fatalf("ListGrouped: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("ListGrouped returned %d groups, want 1", len(groups))
	}
	g := groups[0]
	if g.BaseVersionID != baseVersionID {
		t.Errorf("BaseVersionID = %q, want %q", g.BaseVersionID, baseVersionID)
	}
	if g.Source != "own" {
		t.Errorf("Source = %q, want \"own\"", g.Source)
	}
	if !g.Primary {
		t.Error("Primary should be true for the base group")
	}
	if len(g.Items) != 2 {
		t.Fatalf("group Items count = %d, want 2", len(g.Items))
	}
	// Base should be first.
	if g.Items[0].Link.OntologyVersionID != baseVersionID {
		t.Errorf("Items[0] version = %q, want %q (base)", g.Items[0].Link.OntologyVersionID, baseVersionID)
	}
	if g.Items[1].Link.OntologyVersionID != extVersionID {
		t.Errorf("Items[1] version = %q, want %q (extension)", g.Items[1].Link.OntologyVersionID, extVersionID)
	}
}
