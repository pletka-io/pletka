//go:build integration

package gitmaterializer

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
)

// buildVendoredProjectFixture returns a child snapshot that vendors a single
// parent project, and the parent's own snapshot. Both carry a minimal but
// real entity (a category) so hydration of the full pipeline (shell +
// entities) is observable, matching how a vendored project snapshot is
// actually shaped on disk (see the doc comment on hydrateVendoredProjects).
func buildVendoredProjectFixture(t *testing.T, ownerID, parentID, childID string) *ProjectSnapshot {
	t.Helper()
	parentModule := "pletka.io/orgs/vendored-owner/projects/" + parentID
	parentRootDir := t.TempDir()

	parentSnapshot := &ProjectSnapshot{
		RootDir: parentRootDir,
		Manifest: ProjectManifestFile{
			SchemaVersion: 1,
			Project: ProjectManifestProject{
				ID:         parentID,
				Title:      map[string]string{"en": "Vendored Parent"},
				Namespace:  "https://example.org/vendored-parent/",
				Visibility: "private",
				Owner: &manifestActor{
					ActorID:     ownerID,
					Type:        "organization",
					Slug:        "vendored-owner",
					DisplayName: "Vendored Owner",
				},
			},
			Manifests: &ProjectManifestManifests{},
		},
		Entities: SnapshotEntityTree{
			Categories: []SnapshotEntityFile{
				{
					Path:       "categories/PARENT.CAT.1.yaml",
					EntityType: "category",
					EntityID:   "PARENT.CAT.1",
					Payload:    []byte(`{"semantic_id":"PARENT.CAT.1","system_name":"identity","ui_name":{"en":"Identity"},"status":"draft","canonical_order":1}`),
				},
			},
		},
	}

	// Calculate hash of the parent snapshot's directory for the Sum entry.
	parentHash, err := hashDirectoryTree(parentRootDir)
	if err != nil {
		t.Fatalf("hash parent snapshot dir: %v", err)
	}

	childSnapshot := &ProjectSnapshot{
		Manifest: ProjectManifestFile{
			SchemaVersion: 1,
			Project: ProjectManifestProject{
				ID:         childID,
				Title:      map[string]string{"en": "Vendoring Child"},
				Namespace:  "https://example.org/vendoring-child/",
				Visibility: "private",
				Owner: &manifestActor{
					ActorID:     ownerID,
					Type:        "organization",
					Slug:        "vendored-owner",
					DisplayName: "Vendored Owner",
				},
			},
			Inheritance: &ProjectManifestInheritance{
				Parents: []ProjectManifestParent{
					{ProjectID: parentID, IsPrimary: true, CanonicalOrder: 0, SourceMode: "draft"},
				},
			},
			Manifests: &ProjectManifestManifests{},
		},
		// Add pletka.sum entry for the vendored parent so verifyVendorChecksums passes.
		Sum: []PletkaSumEntry{
			{Module: parentModule, Version: "", TreeSHA: parentHash},
		},
		Vendor: VendorSnapshotSet{
			Projects: []VendoredProjectSnapshot{
				{
					Module:   parentModule,
					Version:  "",
					Snapshot: parentSnapshot,
				},
			},
		},
	}

	return childSnapshot
}

func createVendoredProjectsOwnerActor(t *testing.T, ctx context.Context, queries *sqlcgen.Queries, ownerID string) {
	t.Helper()
	if _, err := queries.WeaveCreateActor(ctx, sqlcgen.WeaveCreateActorParams{
		ID:          ownerID,
		Type:        "organization",
		DisplayName: "Vendored Owner",
		Slug:        "vendored-owner",
		Role:        "",
		Visibility:  "private",
	}); err != nil {
		t.Fatalf("create owner actor: %v", err)
	}
}

func TestHydrateVendoredProjects(t *testing.T) {
	ctx := context.Background()

	t.Run("restore into DB missing the parent creates parent then child, inheritance resolves", func(t *testing.T) {
		pool := hydrateTestPool(t)
		queries := sqlcgen.New(pool)

		ownerID := "VENDORED_PROJECTS_OWNER_A"
		parentID := "VENDORED_PARENT_A"
		childID := "VENDORED_CHILD_A"

		t.Cleanup(func() {
			_, _ = pool.Exec(ctx, `DELETE FROM weave_categories WHERE project_id = ANY($1)`, []string{parentID, childID})
			_, _ = pool.Exec(ctx, `DELETE FROM weave_project_inheritance WHERE project_id = ANY($1) OR parent_project_id = ANY($1)`, []string{parentID, childID})
			_, _ = pool.Exec(ctx, `DELETE FROM weave_projects WHERE id = ANY($1)`, []string{parentID, childID})
			_, _ = pool.Exec(ctx, `DELETE FROM weave_actors WHERE id = $1`, ownerID)
		})
		createVendoredProjectsOwnerActor(t, ctx, queries, ownerID)

		childSnapshot := buildVendoredProjectFixture(t, ownerID, parentID, childID)
		plan := BuildRestorePlan(childSnapshot)

		mat := NewMaterializer(pool, t.TempDir(), nil)
		if err := mat.hydrateVendoredProjects(ctx, plan); err != nil {
			t.Fatalf("hydrateVendoredProjects: %v", err)
		}
		if err := mat.HydrateProjectShell(ctx, plan); err != nil {
			t.Fatalf("HydrateProjectShell (child): %v", err)
		}
		if err := mat.HydrateProjectEntities(ctx, plan); err != nil {
			t.Fatalf("HydrateProjectEntities (child): %v", err)
		}

		if _, err := queries.WeaveGetProjectByID(ctx, parentID); err != nil {
			t.Fatalf("expected vendored parent %s to be restored: %v", parentID, err)
		}
		if _, err := queries.WeaveGetProjectByID(ctx, childID); err != nil {
			t.Fatalf("expected child %s to be restored: %v", childID, err)
		}

		parentCategory, err := queries.WeaveGetCategoryByIdentifier(ctx, sqlcgen.WeaveGetCategoryByIdentifierParams{
			ProjectID:  parentID,
			SemanticID: stringPtr("PARENT.CAT.1"),
		})
		if err != nil {
			t.Fatalf("expected vendored parent's own entities to be restored: %v", err)
		}
		if parentCategory.ProjectID != parentID {
			t.Fatalf("expected restored category to belong to parent %s, got %s", parentID, parentCategory.ProjectID)
		}

		rows, err := pool.Query(ctx, `
			SELECT parent_project_id, is_primary
			FROM weave_project_inheritance
			WHERE project_id = $1
		`, childID)
		if err != nil {
			t.Fatalf("query inheritance: %v", err)
		}
		defer rows.Close()
		var found bool
		for rows.Next() {
			var pID string
			var primary bool
			if err := rows.Scan(&pID, &primary); err != nil {
				t.Fatalf("scan inheritance row: %v", err)
			}
			if pID == parentID && primary {
				found = true
			}
		}
		if !found {
			t.Fatalf("expected inheritance row linking %s -> %s (primary)", childID, parentID)
		}
	})

	t.Run("parent already present is skipped, child still restored", func(t *testing.T) {
		pool := hydrateTestPool(t)
		queries := sqlcgen.New(pool)

		ownerID := "VENDORED_PROJECTS_OWNER_B"
		parentID := "VENDORED_PARENT_B"
		childID := "VENDORED_CHILD_B"

		t.Cleanup(func() {
			_, _ = pool.Exec(ctx, `DELETE FROM weave_categories WHERE project_id = ANY($1)`, []string{parentID, childID})
			_, _ = pool.Exec(ctx, `DELETE FROM weave_project_inheritance WHERE project_id = ANY($1) OR parent_project_id = ANY($1)`, []string{parentID, childID})
			_, _ = pool.Exec(ctx, `DELETE FROM weave_projects WHERE id = ANY($1)`, []string{parentID, childID})
			_, _ = pool.Exec(ctx, `DELETE FROM weave_actors WHERE id = $1`, ownerID)
		})
		createVendoredProjectsOwnerActor(t, ctx, queries, ownerID)

		// Pre-create the parent project directly, WITHOUT the category the
		// vendored snapshot carries — proves the parent is skipped wholesale
		// (its entities are not re-hydrated), not merely upserted.
		uiName, err := json.Marshal(map[string]string{"en": "Pre-existing Parent"})
		if err != nil {
			t.Fatalf("marshal ui_name: %v", err)
		}
		if _, err := queries.WeaveCreateProject(ctx, sqlcgen.WeaveCreateProjectParams{
			ID:          parentID,
			UiName:      uiName,
			Description: uiName,
			Status:      "draft",
			OwnerID:     ownerID,
			Visibility:  "private",
		}); err != nil {
			t.Fatalf("pre-create parent project: %v", err)
		}

		childSnapshot := buildVendoredProjectFixture(t, ownerID, parentID, childID)
		plan := BuildRestorePlan(childSnapshot)

		mat := NewMaterializer(pool, t.TempDir(), nil)
		if err := mat.hydrateVendoredProjects(ctx, plan); err != nil {
			t.Fatalf("hydrateVendoredProjects: %v", err)
		}
		if err := mat.HydrateProjectShell(ctx, plan); err != nil {
			t.Fatalf("HydrateProjectShell (child): %v", err)
		}
		if err := mat.HydrateProjectEntities(ctx, plan); err != nil {
			t.Fatalf("HydrateProjectEntities (child): %v", err)
		}

		if _, err := queries.WeaveGetProjectByID(ctx, childID); err != nil {
			t.Fatalf("expected child %s to be restored even though the parent was skipped: %v", childID, err)
		}
		if _, err := queries.WeaveGetCategoryByIdentifier(ctx, sqlcgen.WeaveGetCategoryByIdentifierParams{
			ProjectID:  parentID,
			SemanticID: stringPtr("PARENT.CAT.1"),
		}); err == nil {
			t.Fatal("expected the pre-existing parent's entities NOT to be hydrated by a skipped vendored restore")
		}
	})

	t.Run("second run is idempotent", func(t *testing.T) {
		pool := hydrateTestPool(t)
		queries := sqlcgen.New(pool)

		ownerID := "VENDORED_PROJECTS_OWNER_C"
		parentID := "VENDORED_PARENT_C"
		childID := "VENDORED_CHILD_C"

		t.Cleanup(func() {
			_, _ = pool.Exec(ctx, `DELETE FROM weave_categories WHERE project_id = ANY($1)`, []string{parentID, childID})
			_, _ = pool.Exec(ctx, `DELETE FROM weave_project_inheritance WHERE project_id = ANY($1) OR parent_project_id = ANY($1)`, []string{parentID, childID})
			_, _ = pool.Exec(ctx, `DELETE FROM weave_projects WHERE id = ANY($1)`, []string{parentID, childID})
			_, _ = pool.Exec(ctx, `DELETE FROM weave_actors WHERE id = $1`, ownerID)
		})
		createVendoredProjectsOwnerActor(t, ctx, queries, ownerID)

		childSnapshot := buildVendoredProjectFixture(t, ownerID, parentID, childID)
		plan := BuildRestorePlan(childSnapshot)
		mat := NewMaterializer(pool, t.TempDir(), nil)

		for i := 0; i < 2; i++ {
			if err := mat.hydrateVendoredProjects(ctx, plan); err != nil {
				t.Fatalf("hydrateVendoredProjects run %d: %v", i+1, err)
			}
			if err := mat.HydrateProjectShell(ctx, plan); err != nil {
				t.Fatalf("HydrateProjectShell run %d: %v", i+1, err)
			}
			if err := mat.HydrateProjectEntities(ctx, plan); err != nil {
				t.Fatalf("HydrateProjectEntities run %d: %v", i+1, err)
			}
		}

		if _, err := queries.WeaveGetProjectByID(ctx, parentID); err != nil {
			t.Fatalf("expected vendored parent %s to be present after two runs: %v", parentID, err)
		}
		rows, err := pool.Query(ctx, `SELECT COUNT(*) FROM weave_projects WHERE id = $1`, parentID)
		if err != nil {
			t.Fatalf("count parent rows: %v", err)
		}
		defer rows.Close()
		var count int
		if rows.Next() {
			if err := rows.Scan(&count); err != nil {
				t.Fatalf("scan count: %v", err)
			}
		}
		if count != 1 {
			t.Fatalf("expected exactly 1 parent project row after two idempotent runs, got %d", count)
		}
	})

	t.Run("depth cap errors when nesting exceeds the guard", func(t *testing.T) {
		// Construct an artificial 4-level-deep chain of Vendor.Projects to
		// exercise the defensive depth cap. This shape is not produced by
		// vendorDependenciesForProject today (see the doc comment on
		// hydrateVendoredProjects) but the guard must still fire if it ever
		// is, rather than recursing unbounded.
		leaf := &ProjectSnapshot{
			Manifest: ProjectManifestFile{Project: ProjectManifestProject{ID: "DEPTH_LEAF"}},
		}
		level3 := &ProjectSnapshot{
			Manifest: ProjectManifestFile{Project: ProjectManifestProject{ID: "DEPTH_3"}},
			Vendor:   VendorSnapshotSet{Projects: []VendoredProjectSnapshot{{Module: "m3", Snapshot: leaf}}},
		}
		level2 := &ProjectSnapshot{
			Manifest: ProjectManifestFile{Project: ProjectManifestProject{ID: "DEPTH_2"}},
			Vendor:   VendorSnapshotSet{Projects: []VendoredProjectSnapshot{{Module: "m2", Snapshot: level3}}},
		}
		level1 := &ProjectSnapshot{
			Manifest: ProjectManifestFile{Project: ProjectManifestProject{ID: "DEPTH_1"}},
			Vendor:   VendorSnapshotSet{Projects: []VendoredProjectSnapshot{{Module: "m1", Snapshot: level2}}},
		}
		root := &ProjectSnapshot{
			Manifest: ProjectManifestFile{Project: ProjectManifestProject{ID: "DEPTH_ROOT"}},
			Vendor:   VendorSnapshotSet{Projects: []VendoredProjectSnapshot{{Module: "m0", Snapshot: level1}}},
		}
		plan := BuildRestorePlan(root)

		// Real pool: the walk performs a WeaveGetProjectByID existence check
		// at each level before the depth cap is reached.
		pool := hydrateTestPool(t)
		mat := NewMaterializer(pool, t.TempDir(), nil)
		err := mat.hydrateVendoredProjects(ctx, plan)
		if err == nil {
			t.Fatal("hydrateVendoredProjects() = nil, want depth-cap error")
		}
	})
}

func TestHydrateRestorePlanVendoredProjects(t *testing.T) {
	ctx := context.Background()

	t.Run("HydrateRestorePlan calls hydrateVendoredProjects before other hydrators", func(t *testing.T) {
		pool := hydrateTestPool(t)
		queries := sqlcgen.New(pool)

		parentOwnerID := "HYDRATE_PLAN_PARENT_OWNER"
		parentID := "HYDRATE_PLAN_PARENT"
		childID := "HYDRATE_PLAN_CHILD"

		t.Cleanup(func() {
			_, _ = pool.Exec(ctx, `DELETE FROM weave_categories WHERE project_id = ANY($1)`, []string{parentID, childID})
			_, _ = pool.Exec(ctx, `DELETE FROM weave_project_inheritance WHERE project_id = ANY($1) OR parent_project_id = ANY($1)`, []string{parentID, childID})
			_, _ = pool.Exec(ctx, `DELETE FROM weave_projects WHERE id = ANY($1)`, []string{parentID, childID})
			_, _ = pool.Exec(ctx, `DELETE FROM weave_actors WHERE id = $1`, parentOwnerID)
		})

		// Create the parent's owner actor only. The child will reference a
		// different owner that doesn't exist, so HydrateProjectShell will fail.
		// This proves the ordering: hydrateVendoredProjects ran FIRST and created
		// the parent BEFORE HydrateProjectShell failed on the child.
		createVendoredProjectsOwnerActor(t, ctx, queries, parentOwnerID)

		// Build a snapshot where the parent (not in DB) vendors nothing, and the
		// child vendors the parent and references it as a primary parent.
		childSnapshot := buildVendoredProjectFixture(t, parentOwnerID, parentID, childID)

		// Replace the child's owner with a different actor that doesn't exist,
		// so HydrateProjectShell will fail when trying to validate it.
		childSnapshot.Manifest.Project.Owner = &manifestActor{
			ActorID:     "NONEXISTENT_CHILD_OWNER",
			Type:        "organization",
			Slug:        "nonexistent-owner",
			DisplayName: "Nonexistent Owner",
		}

		plan := BuildRestorePlan(childSnapshot)

		mat := NewMaterializer(pool, t.TempDir(), nil)

		// Call HydrateRestorePlan. hydrateVendoredProjects will run FIRST and
		// create the parent project. Then HydrateProjectShell will fail because
		// the child's owner doesn't exist. The assertion is that the parent row
		// was created BEFORE that error (proving the ordering: hydrateVendoredProjects
		// ran before HydrateProjectShell, matching the pattern of the ontology
		// wiring test in restore_ontologies_test.go:215-239).
		_ = mat.HydrateRestorePlan(ctx, plan)

		// Verify the parent project was restored.
		if _, err := queries.WeaveGetProjectByID(ctx, parentID); err != nil {
			t.Fatalf("expected vendored parent %s to be created by HydrateRestorePlan: %v", parentID, err)
		}
	})
}
