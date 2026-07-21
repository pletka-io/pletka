//go:build integration

package weave_test

import (
	"context"
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave"
)

func TestProjectInheritanceStore_SetPrimaryMissingPreservesExistingPrimary(t *testing.T) {
	pool := testPool(t)
	store := weave.NewPostgresStore(pool)
	inheritances := store.ProjectInheritances()
	ctx := context.Background()

	ownerID := seedTestActor(t, pool, "TEST_INHERIT_OWNER")
	projectID := "TEST_INHERIT_PROJ"
	parentA := "TEST_INHERIT_PARENT_A"
	parentB := "TEST_INHERIT_PARENT_B"

	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, `DELETE FROM weave_project_inheritance WHERE project_id LIKE 'TEST_INHERIT_%' OR parent_project_id LIKE 'TEST_INHERIT_%'`)
		_, _ = pool.Exec(bg, `DELETE FROM weave_projects WHERE id LIKE 'TEST_INHERIT_%'`)
		_, _ = pool.Exec(bg, `DELETE FROM weave_actors WHERE id = $1`, ownerID)
	})

	seedProject(t, pool, ctx, parentA, ownerID, nil)
	seedProject(t, pool, ctx, parentB, ownerID, nil)
	seedProject(t, pool, ctx, projectID, ownerID, nil)
	seedInheritanceLink(t, pool, ctx, projectID, parentA, true, 0)
	seedInheritanceLink(t, pool, ctx, projectID, parentB, false, 1)

	if err := inheritances.SetPrimary(ctx, projectID, "TEST_INHERIT_PARENT_MISSING"); err == nil {
		t.Fatalf("SetPrimary on missing parent unexpectedly succeeded")
	}

	links, err := inheritances.List(ctx, projectID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(links) != 2 {
		t.Fatalf("got %d links, want 2", len(links))
	}
	if links[0].ParentProjectID != parentA || !links[0].IsPrimary {
		t.Fatalf("primary link changed after failed SetPrimary: %#v", links)
	}
}

func TestProjectInheritanceStore_RemoveCompactsCanonicalOrder(t *testing.T) {
	pool := testPool(t)
	store := weave.NewPostgresStore(pool)
	inheritances := store.ProjectInheritances()
	ctx := context.Background()

	ownerID := seedTestActor(t, pool, "TEST_INHERIT_ORDER_OWNER")
	projectID := "TEST_INHERIT_ORDER_PROJ"
	parentA := "TEST_INHERIT_ORDER_A"
	parentB := "TEST_INHERIT_ORDER_B"

	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, `DELETE FROM weave_project_inheritance WHERE project_id LIKE 'TEST_INHERIT_ORDER_%' OR parent_project_id LIKE 'TEST_INHERIT_ORDER_%'`)
		_, _ = pool.Exec(bg, `DELETE FROM weave_projects WHERE id LIKE 'TEST_INHERIT_ORDER_%'`)
		_, _ = pool.Exec(bg, `DELETE FROM weave_actors WHERE id = $1`, ownerID)
	})

	seedProject(t, pool, ctx, parentA, ownerID, nil)
	seedProject(t, pool, ctx, parentB, ownerID, nil)
	seedProject(t, pool, ctx, projectID, ownerID, nil)
	seedInheritanceLink(t, pool, ctx, projectID, parentA, true, 0)
	seedInheritanceLink(t, pool, ctx, projectID, parentB, false, 1)

	if err := inheritances.Remove(ctx, projectID, parentA); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	links, err := inheritances.List(ctx, projectID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("got %d links, want 1", len(links))
	}
	if links[0].ParentProjectID != parentB {
		t.Fatalf("remaining parent = %q, want %q", links[0].ParentProjectID, parentB)
	}
	if links[0].CanonicalOrder != 0 {
		t.Fatalf("remaining canonical_order = %d, want 0", links[0].CanonicalOrder)
	}
	if !links[0].IsPrimary {
		t.Fatalf("remaining parent should be promoted to primary")
	}
}

func TestProjectInheritanceStore_AddPersistsSourceSelector(t *testing.T) {
	pool := testPool(t)
	store := weave.NewPostgresStore(pool)
	inheritances := store.ProjectInheritances()
	ctx := context.Background()

	ownerID := seedTestActor(t, pool, "TEST_INHERIT_SOURCE_OWNER")
	projectID := "TEST_INHERIT_SOURCE_PROJ"
	parentA := "TEST_INHERIT_SOURCE_PARENT_A"
	parentB := "TEST_INHERIT_SOURCE_PARENT_B"

	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, `DELETE FROM weave_project_inheritance WHERE project_id LIKE 'TEST_INHERIT_SOURCE_%' OR parent_project_id LIKE 'TEST_INHERIT_SOURCE_%'`)
		_, _ = pool.Exec(bg, `DELETE FROM weave_projects WHERE id LIKE 'TEST_INHERIT_SOURCE_%'`)
		_, _ = pool.Exec(bg, `DELETE FROM weave_actors WHERE id = $1`, ownerID)
	})

	seedProject(t, pool, ctx, parentA, ownerID, nil)
	seedProject(t, pool, ctx, parentB, ownerID, nil)
	seedProject(t, pool, ctx, projectID, ownerID, nil)

	if err := inheritances.Add(ctx, domain.ProjectInheritance{
		ProjectID:       projectID,
		ParentProjectID: parentA,
		IsPrimary:       true,
		CanonicalOrder:  0,
		SourceMode:      domain.DependencySourceRelease,
		SourceVersion:   "1.2.0",
	}); err != nil {
		t.Fatalf("Add(release): %v", err)
	}
	if err := inheritances.Add(ctx, domain.ProjectInheritance{
		ProjectID:       projectID,
		ParentProjectID: parentB,
		IsPrimary:       false,
		CanonicalOrder:  1,
	}); err != nil {
		t.Fatalf("Add(draft default): %v", err)
	}

	links, err := inheritances.List(ctx, projectID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(links) != 2 {
		t.Fatalf("got %d links, want 2", len(links))
	}
	if links[0].ParentProjectID != parentA {
		t.Fatalf("primary link parent = %q, want %q", links[0].ParentProjectID, parentA)
	}
	if links[0].SourceMode != domain.DependencySourceRelease || links[0].SourceVersion != "1.2.0" {
		t.Fatalf("release selector not preserved: %#v", links[0])
	}
	if links[1].ParentProjectID != parentB {
		t.Fatalf("secondary link parent = %q, want %q", links[1].ParentProjectID, parentB)
	}
	if links[1].SourceMode != domain.DependencySourceDraft || links[1].SourceVersion != "" {
		t.Fatalf("draft default not preserved: %#v", links[1])
	}
}
