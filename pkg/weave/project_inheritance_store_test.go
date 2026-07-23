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

// TestProjectInheritanceStore_CycleGuard exercises migration 004's database
// trigger: any write that would close an inheritance loop is rejected at the
// DB layer, regardless of writer (store, import waves, restore, raw SQL).
// The SI <-> SUR cycle that blocked release-baseline came from a writer that
// bypassed the UI-level guard — the trigger is the backstop.
func TestProjectInheritanceStore_CycleGuard(t *testing.T) {
	pool := testPool(t)
	store := weave.NewPostgresStore(pool)
	inheritances := store.ProjectInheritances()
	ctx := context.Background()

	ownerID := seedTestActor(t, pool, "TEST_CYCLE_OWNER")
	a, b, c := "TEST_CYCLE_A", "TEST_CYCLE_B", "TEST_CYCLE_C"

	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, `DELETE FROM weave_project_inheritance WHERE project_id LIKE 'TEST_CYCLE_%' OR parent_project_id LIKE 'TEST_CYCLE_%'`)
		_, _ = pool.Exec(bg, `DELETE FROM weave_projects WHERE id LIKE 'TEST_CYCLE_%'`)
		_, _ = pool.Exec(bg, `DELETE FROM weave_actors WHERE id = $1`, ownerID)
	})

	seedProject(t, pool, ctx, a, ownerID, nil)
	seedProject(t, pool, ctx, b, ownerID, nil)
	seedProject(t, pool, ctx, c, ownerID, nil)

	// Legit chain: A -> B -> C.
	if err := inheritances.Add(ctx, domain.ProjectInheritance{ProjectID: a, ParentProjectID: b}); err != nil {
		t.Fatalf("add A->B: %v", err)
	}
	if err := inheritances.Add(ctx, domain.ProjectInheritance{ProjectID: b, ParentProjectID: c}); err != nil {
		t.Fatalf("add B->C: %v", err)
	}

	// Self-link rejected.
	if err := inheritances.Add(ctx, domain.ProjectInheritance{ProjectID: a, ParentProjectID: a}); err == nil {
		t.Fatal("self-link A->A unexpectedly succeeded")
	}
	// Direct cycle rejected: B -> A while A -> B exists.
	if err := inheritances.Add(ctx, domain.ProjectInheritance{ProjectID: b, ParentProjectID: a}); err == nil {
		t.Fatal("direct cycle B->A unexpectedly succeeded")
	}
	// Transitive cycle rejected: C -> A while A -> B -> C exists.
	if err := inheritances.Add(ctx, domain.ProjectInheritance{ProjectID: c, ParentProjectID: a}); err == nil {
		t.Fatal("transitive cycle C->A unexpectedly succeeded")
	}
	// The guard lives in the database, not the store: a raw INSERT that
	// bypasses every Go code path is rejected too.
	if _, err := pool.Exec(ctx, `
		INSERT INTO weave_project_inheritance (project_id, parent_project_id)
		VALUES ($1, $2)`, c, a); err == nil {
		t.Fatal("raw-SQL transitive cycle C->A unexpectedly succeeded")
	}

	// Graph unchanged; a further legit link still works.
	links, err := inheritances.List(ctx, a)
	if err != nil {
		t.Fatalf("List(A): %v", err)
	}
	if len(links) != 1 || links[0].ParentProjectID != b {
		t.Fatalf("graph changed after rejected writes: %#v", links)
	}
	if err := inheritances.Add(ctx, domain.ProjectInheritance{ProjectID: a, ParentProjectID: c}); err != nil {
		t.Fatalf("legit add A->C after rejections: %v", err)
	}
}
