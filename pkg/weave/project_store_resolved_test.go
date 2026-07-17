package weave_test

import (
	"context"
	"testing"

	weaveauth "github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave"
	"github.com/jackc/pgx/v5/pgxpool"
)

// seedProject inserts a minimal weave_projects row for testing. ownerID must
// exist in weave_actors. parentID may be nil. Caller is responsible for cleanup.
func seedProject(t *testing.T, pool *pgxpool.Pool, ctx context.Context, id, ownerID string, parentID *string) {
	t.Helper()
	_, err := pool.Exec(ctx,
		`INSERT INTO weave_projects (id, owner_id, parent_project_id)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (id) DO NOTHING`,
		id, ownerID, parentID,
	)
	if err != nil {
		t.Fatalf("seed project %s: %v", id, err)
	}
}

// seedOntologyLink inserts a minimal weave_project_ontology_versions row.
// Caller is responsible for cleanup.
func seedOntologyLink(t *testing.T, pool *pgxpool.Pool, ctx context.Context, projectID, versionID, usageNotes string) {
	t.Helper()
	_, err := pool.Exec(ctx,
		`INSERT INTO weave_project_ontology_versions
		  (project_id, ontology_version_id, is_primary, usage_notes, added_at)
		 VALUES ($1, $2, false, $3, now())
		 ON CONFLICT (project_id, ontology_version_id) DO NOTHING`,
		projectID, versionID, usageNotes,
	)
	if err != nil {
		t.Fatalf("seed ontology link %s->%s: %v", projectID, versionID, err)
	}
}

func seedInheritanceLink(t *testing.T, pool *pgxpool.Pool, ctx context.Context, projectID, parentID string, isPrimary bool, order int) {
	t.Helper()
	_, err := pool.Exec(ctx, `
		INSERT INTO weave_project_inheritance (project_id, parent_project_id, is_primary, canonical_order, source_mode, source_version)
		VALUES ($1, $2, $3, $4, 'draft', NULL)
		ON CONFLICT (project_id, parent_project_id)
		DO UPDATE SET is_primary = EXCLUDED.is_primary, canonical_order = EXCLUDED.canonical_order, source_mode = EXCLUDED.source_mode, source_version = EXCLUDED.source_version
	`, projectID, parentID, isPrimary, order)
	if err != nil {
		t.Fatalf("seed inheritance %s->%s: %v", projectID, parentID, err)
	}
}

func seedInheritanceLinkWithSource(t *testing.T, pool *pgxpool.Pool, ctx context.Context, projectID, parentID string, isPrimary bool, order int, sourceMode domain.DependencySourceMode, sourceVersion string) {
	t.Helper()
	mode := string(sourceMode)
	if mode == "" {
		mode = string(domain.DependencySourceDraft)
	}
	var sourceVersionPtr *string
	if sourceVersion != "" {
		sourceVersionPtr = &sourceVersion
	}
	_, err := pool.Exec(ctx, `
		INSERT INTO weave_project_inheritance (project_id, parent_project_id, is_primary, canonical_order, source_mode, source_version)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (project_id, parent_project_id)
		DO UPDATE SET
			is_primary = EXCLUDED.is_primary,
			canonical_order = EXCLUDED.canonical_order,
			source_mode = EXCLUDED.source_mode,
			source_version = EXCLUDED.source_version
	`, projectID, parentID, isPrimary, order, mode, sourceVersionPtr)
	if err != nil {
		t.Fatalf("seed inheritance %s->%s with source: %v", projectID, parentID, err)
	}
}

func TestWeaveProjectStore_ResolvedOntologyVersions_ParentChain(t *testing.T) {
	pool := testPool(t)
	store := weave.NewPostgresStore(pool)
	projects := store.Projects()
	ctx := context.Background()

	ownerID := seedTestActor(t, pool, "TEST_ACTOR_CHAIN")

	gpID := "TEST_PROJ_CHAIN_GP"
	pID := "TEST_PROJ_CHAIN_P"
	cID := "TEST_PROJ_CHAIN_C"
	v1 := "TEST_OV_CHAIN_V1"
	v2 := "TEST_OV_CHAIN_V2"
	v3 := "TEST_OV_CHAIN_V3"

	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, `DELETE FROM weave_project_ontology_versions WHERE project_id LIKE 'TEST_%'`)
		_, _ = pool.Exec(bg, `DELETE FROM weave_project_inheritance WHERE project_id LIKE 'TEST_%' OR parent_project_id LIKE 'TEST_%'`)
		_, _ = pool.Exec(bg, `DELETE FROM weave_projects WHERE id LIKE 'TEST_%'`)
		_, _ = pool.Exec(bg, `DELETE FROM weave_actors WHERE id = $1`, ownerID)
	})

	// Seed GP (no parent), P (parent=GP), C (parent=P).
	seedProject(t, pool, ctx, gpID, ownerID, nil)
	seedProject(t, pool, ctx, pID, ownerID, &gpID)
	seedProject(t, pool, ctx, cID, ownerID, &pID)

	// Link: GP→v1, P→v2, C→v3.
	seedOntologyLink(t, pool, ctx, gpID, v1, "gp notes")
	seedOntologyLink(t, pool, ctx, pID, v2, "p notes")
	seedOntologyLink(t, pool, ctx, cID, v3, "c notes")

	resolved, err := projects.ResolvedOntologyVersions(ctx, cID, domain.ResolvedOntologyVersionOpts{})
	if err != nil {
		t.Fatalf("ResolvedOntologyVersions: %v", err)
	}
	if len(resolved) != 3 {
		t.Fatalf("got %d resolved entries, want 3", len(resolved))
	}

	// First entry should be own (C → v3, SourceProjectID="").
	if resolved[0].Link.OntologyVersionID != v3 {
		t.Errorf("resolved[0] version = %q, want %q (own)", resolved[0].Link.OntologyVersionID, v3)
	}
	if resolved[0].SourceProjectID != "" {
		t.Errorf("resolved[0] SourceProjectID = %q, want empty (own)", resolved[0].SourceProjectID)
	}

	// Second should be from P.
	if resolved[1].Link.OntologyVersionID != v2 {
		t.Errorf("resolved[1] version = %q, want %q (from P)", resolved[1].Link.OntologyVersionID, v2)
	}
	if resolved[1].SourceProjectID != pID {
		t.Errorf("resolved[1] SourceProjectID = %q, want %q", resolved[1].SourceProjectID, pID)
	}

	// Third should be from GP.
	if resolved[2].Link.OntologyVersionID != v1 {
		t.Errorf("resolved[2] version = %q, want %q (from GP)", resolved[2].Link.OntologyVersionID, v1)
	}
	if resolved[2].SourceProjectID != gpID {
		t.Errorf("resolved[2] SourceProjectID = %q, want %q", resolved[2].SourceProjectID, gpID)
	}
}

func TestWeaveProjectStore_ResolvedOntologyVersions_OnlyInherited(t *testing.T) {
	pool := testPool(t)
	store := weave.NewPostgresStore(pool)
	projects := store.Projects()
	ctx := context.Background()

	ownerID := seedTestActor(t, pool, "TEST_ACTOR_INHERITED")

	gpID := "TEST_PROJ_INH_GP"
	pID := "TEST_PROJ_INH_P"
	cID := "TEST_PROJ_INH_C"
	v1 := "TEST_OV_INH_V1"
	v2 := "TEST_OV_INH_V2"
	v3 := "TEST_OV_INH_V3"

	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, `DELETE FROM weave_project_ontology_versions WHERE project_id LIKE 'TEST_%'`)
		_, _ = pool.Exec(bg, `DELETE FROM weave_project_inheritance WHERE project_id LIKE 'TEST_%' OR parent_project_id LIKE 'TEST_%'`)
		_, _ = pool.Exec(bg, `DELETE FROM weave_projects WHERE id LIKE 'TEST_%'`)
		_, _ = pool.Exec(bg, `DELETE FROM weave_actors WHERE id = $1`, ownerID)
	})

	seedProject(t, pool, ctx, gpID, ownerID, nil)
	seedProject(t, pool, ctx, pID, ownerID, &gpID)
	seedProject(t, pool, ctx, cID, ownerID, &pID)

	seedOntologyLink(t, pool, ctx, gpID, v1, "")
	seedOntologyLink(t, pool, ctx, pID, v2, "")
	seedOntologyLink(t, pool, ctx, cID, v3, "")

	resolved, err := projects.ResolvedOntologyVersions(ctx, cID, domain.ResolvedOntologyVersionOpts{OnlyInherited: true})
	if err != nil {
		t.Fatalf("ResolvedOntologyVersions(OnlyInherited): %v", err)
	}
	if len(resolved) != 2 {
		t.Fatalf("got %d resolved entries, want 2 (no own)", len(resolved))
	}

	// Own version v3 should NOT appear.
	for _, r := range resolved {
		if r.Link.OntologyVersionID == v3 {
			t.Error("own version v3 should not appear when OnlyInherited=true")
		}
	}

	// v2 and v1 should appear.
	vids := map[string]bool{}
	for _, r := range resolved {
		vids[r.Link.OntologyVersionID] = true
	}
	if !vids[v2] {
		t.Error("v2 (from P) should appear when OnlyInherited=true")
	}
	if !vids[v1] {
		t.Error("v1 (from GP) should appear when OnlyInherited=true")
	}
}

func TestWeaveProjectStore_ResolvedOntologyVersions_OwnWinsOnDedup(t *testing.T) {
	pool := testPool(t)
	store := weave.NewPostgresStore(pool)
	projects := store.Projects()
	ctx := context.Background()

	ownerID := seedTestActor(t, pool, "TEST_ACTOR_DEDUP")

	parentID := "TEST_PROJ_DEDUP_P"
	childID := "TEST_PROJ_DEDUP_C"
	sharedVersionID := "TEST_OV_DEDUP_V1"
	childNotes := "child notes — should win"

	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, `DELETE FROM weave_project_ontology_versions WHERE project_id LIKE 'TEST_%'`)
		_, _ = pool.Exec(bg, `DELETE FROM weave_project_inheritance WHERE project_id LIKE 'TEST_%' OR parent_project_id LIKE 'TEST_%'`)
		_, _ = pool.Exec(bg, `DELETE FROM weave_projects WHERE id LIKE 'TEST_%'`)
		_, _ = pool.Exec(bg, `DELETE FROM weave_actors WHERE id = $1`, ownerID)
	})

	seedProject(t, pool, ctx, parentID, ownerID, nil)
	seedProject(t, pool, ctx, childID, ownerID, &parentID)

	// Both parent and child link the same version, with different usage_notes.
	seedOntologyLink(t, pool, ctx, parentID, sharedVersionID, "parent notes — should lose")
	seedOntologyLink(t, pool, ctx, childID, sharedVersionID, childNotes)

	resolved, err := projects.ResolvedOntologyVersions(ctx, childID, domain.ResolvedOntologyVersionOpts{})
	if err != nil {
		t.Fatalf("ResolvedOntologyVersions: %v", err)
	}
	if len(resolved) != 1 {
		t.Fatalf("got %d resolved entries, want 1 (dedup)", len(resolved))
	}
	if resolved[0].Link.OntologyVersionID != sharedVersionID {
		t.Errorf("version = %q, want %q", resolved[0].Link.OntologyVersionID, sharedVersionID)
	}
	if resolved[0].Link.UsageNotes != childNotes {
		t.Errorf("UsageNotes = %q, want %q (child should win)", resolved[0].Link.UsageNotes, childNotes)
	}
	if resolved[0].SourceProjectID != "" {
		t.Errorf("SourceProjectID = %q, want empty (own wins)", resolved[0].SourceProjectID)
	}
}

func TestWeaveProjectStore_ResolvedOntologyVersions_CycleBounded(t *testing.T) {
	pool := testPool(t)
	store := weave.NewPostgresStore(pool)
	projects := store.Projects()
	ctx := context.Background()

	ownerID := seedTestActor(t, pool, "TEST_ACTOR_CYCLE")

	aID := "TEST_PROJ_CYCLE_A"
	bID := "TEST_PROJ_CYCLE_B"

	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, `DELETE FROM weave_project_inheritance WHERE project_id LIKE 'TEST_%' OR parent_project_id LIKE 'TEST_%'`)
		// Break cycle before deletion to avoid FK issues.
		_, _ = pool.Exec(bg, `UPDATE weave_projects SET parent_project_id = NULL WHERE id = ANY($1::text[])`, []string{aID, bID})
		_, _ = pool.Exec(bg, `DELETE FROM weave_project_ontology_versions WHERE project_id LIKE 'TEST_%'`)
		_, _ = pool.Exec(bg, `DELETE FROM weave_projects WHERE id LIKE 'TEST_%'`)
		_, _ = pool.Exec(bg, `DELETE FROM weave_actors WHERE id = $1`, ownerID)
	})

	// Insert A and B with circular parent references.
	// Insert without parent first, then update to set the cycle.
	if _, err := pool.Exec(ctx,
		`INSERT INTO weave_projects (id, owner_id) VALUES ($1, $2)`, aID, ownerID,
	); err != nil {
		t.Fatalf("insert A: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO weave_projects (id, owner_id, parent_project_id) VALUES ($1, $2, $3)`, bID, ownerID, aID,
	); err != nil {
		t.Fatalf("insert B: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`UPDATE weave_projects SET parent_project_id = $2 WHERE id = $1`, aID, bID,
	); err != nil {
		t.Fatalf("set A parent to B: %v", err)
	}

	// Should not hang and should return without error.
	resolved, err := projects.ResolvedOntologyVersions(ctx, aID, domain.ResolvedOntologyVersionOpts{})
	if err != nil {
		t.Fatalf("ResolvedOntologyVersions cycle: %v", err)
	}
	// Cycle is bounded; result may be empty or contain own entries but should complete.
	_ = resolved
}

func TestWeaveProjectStore_ResolvedOntologyVersions_ArchivedParentChain(t *testing.T) {
	pool := testPool(t)
	store := weave.NewPostgresStore(pool)
	projects := store.Projects()
	ctx := weaveauth.WithProjectVersion(context.Background(), "0.1.0-test")

	ownerID := seedTestActor(t, pool, "TEST_ACTOR_ARCHIVED_CHAIN")

	gpID := "TEST_PROJ_ARCH_GP"
	pID := "TEST_PROJ_ARCH_P"
	cID := "TEST_PROJ_ARCH_C"
	v1 := "TEST_OV_ARCH_V1"
	v2 := "TEST_OV_ARCH_V2"
	v3 := "TEST_OV_ARCH_V3"

	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, `DELETE FROM weave_project_inheritance_archive WHERE project_id LIKE 'TEST_%' OR parent_project_id LIKE 'TEST_%'`)
		_, _ = pool.Exec(bg, `DELETE FROM weave_project_ontology_versions_archive WHERE project_id LIKE 'TEST_%'`)
		_, _ = pool.Exec(bg, `DELETE FROM weave_projects_archive WHERE id LIKE 'TEST_%'`)
		_, _ = pool.Exec(bg, `DELETE FROM weave_project_inheritance WHERE project_id LIKE 'TEST_%' OR parent_project_id LIKE 'TEST_%'`)
		_, _ = pool.Exec(bg, `DELETE FROM weave_projects WHERE id LIKE 'TEST_%'`)
		_, _ = pool.Exec(bg, `DELETE FROM weave_actors WHERE id = $1`, ownerID)
	})

	seedProject(t, pool, context.Background(), gpID, ownerID, nil)
	seedProject(t, pool, context.Background(), pID, ownerID, &gpID)
	seedProject(t, pool, context.Background(), cID, ownerID, &pID)

	_, err := pool.Exec(context.Background(), `
		INSERT INTO weave_projects_archive (id, owner_id, parent_project_id, version_number)
		VALUES
			($1, $4, NULL, $5),
			($2, $4, $1, $5),
			($3, $4, $2, $5)
	`, gpID, pID, cID, ownerID, "0.1.0-test")
	if err != nil {
		t.Fatalf("seed archived projects: %v", err)
	}

	_, err = pool.Exec(context.Background(), `
		INSERT INTO weave_project_ontology_versions_archive
			(project_id, ontology_version_id, added_at, is_primary, usage_notes, version_number)
		VALUES
			($1, $4, now(), false, 'gp notes', $7),
			($2, $5, now(), false, 'p notes', $7),
			($3, $6, now(), false, 'c notes', $7)
	`, gpID, pID, cID, v1, v2, v3, "0.1.0-test")
	if err != nil {
		t.Fatalf("seed archived ontology links: %v", err)
	}

	resolved, err := projects.ResolvedOntologyVersions(ctx, cID, domain.ResolvedOntologyVersionOpts{})
	if err != nil {
		t.Fatalf("ResolvedOntologyVersions archived: %v", err)
	}
	if len(resolved) != 3 {
		t.Fatalf("got %d resolved archived entries, want 3", len(resolved))
	}
	if resolved[0].Link.OntologyVersionID != v3 || resolved[0].SourceProjectID != "" {
		t.Fatalf("resolved[0] = (%q,%q), want own %q", resolved[0].Link.OntologyVersionID, resolved[0].SourceProjectID, v3)
	}
	if resolved[1].Link.OntologyVersionID != v2 || resolved[1].SourceProjectID != pID {
		t.Fatalf("resolved[1] = (%q,%q), want (%q,%q)", resolved[1].Link.OntologyVersionID, resolved[1].SourceProjectID, v2, pID)
	}
	if resolved[2].Link.OntologyVersionID != v1 || resolved[2].SourceProjectID != gpID {
		t.Fatalf("resolved[2] = (%q,%q), want (%q,%q)", resolved[2].Link.OntologyVersionID, resolved[2].SourceProjectID, v1, gpID)
	}
}

func TestWeaveProjectStore_ResolvedOntologyVersions_UsesInheritanceGraph(t *testing.T) {
	pool := testPool(t)
	store := weave.NewPostgresStore(pool)
	projects := store.Projects()
	ctx := context.Background()

	ownerID := seedTestActor(t, pool, "TEST_ACTOR_INHERIT_GRAPH")

	p1ID := "TEST_PROJ_GRAPH_P1"
	p2ID := "TEST_PROJ_GRAPH_P2"
	childID := "TEST_PROJ_GRAPH_C"
	v1 := "TEST_OV_GRAPH_V1"
	v2 := "TEST_OV_GRAPH_V2"
	v3 := "TEST_OV_GRAPH_V3"

	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, `DELETE FROM weave_project_ontology_versions WHERE project_id LIKE 'TEST_%'`)
		_, _ = pool.Exec(bg, `DELETE FROM weave_project_inheritance WHERE project_id LIKE 'TEST_%' OR parent_project_id LIKE 'TEST_%'`)
		_, _ = pool.Exec(bg, `DELETE FROM weave_projects WHERE id LIKE 'TEST_%'`)
		_, _ = pool.Exec(bg, `DELETE FROM weave_actors WHERE id = $1`, ownerID)
	})

	seedProject(t, pool, ctx, p1ID, ownerID, nil)
	seedProject(t, pool, ctx, p2ID, ownerID, nil)
	seedProject(t, pool, ctx, childID, ownerID, nil)
	seedInheritanceLink(t, pool, ctx, childID, p1ID, true, 0)
	seedInheritanceLink(t, pool, ctx, childID, p2ID, false, 1)

	seedOntologyLink(t, pool, ctx, p1ID, v1, "p1")
	seedOntologyLink(t, pool, ctx, p2ID, v2, "p2")
	seedOntologyLink(t, pool, ctx, childID, v3, "child")

	var linkCount int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM weave_project_inheritance
		WHERE project_id = $1
	`, childID).Scan(&linkCount); err != nil {
		t.Fatalf("count inheritances: %v", err)
	}
	if linkCount != 2 {
		t.Fatalf("got %d inheritance rows, want 2", linkCount)
	}

	resolved, err := projects.ResolvedOntologyVersions(ctx, childID, domain.ResolvedOntologyVersionOpts{})
	if err != nil {
		t.Fatalf("ResolvedOntologyVersions inheritance graph: %v", err)
	}
	if len(resolved) != 3 {
		t.Fatalf("got %d resolved entries, want 3", len(resolved))
	}
	if resolved[0].Link.OntologyVersionID != v3 || resolved[0].SourceProjectID != "" {
		t.Fatalf("resolved[0] = (%q,%q), want own %q", resolved[0].Link.OntologyVersionID, resolved[0].SourceProjectID, v3)
	}
	if resolved[1].Link.OntologyVersionID != v1 || resolved[1].SourceProjectID != p1ID {
		t.Fatalf("resolved[1] = (%q,%q), want (%q,%q)", resolved[1].Link.OntologyVersionID, resolved[1].SourceProjectID, v1, p1ID)
	}
	if resolved[2].Link.OntologyVersionID != v2 || resolved[2].SourceProjectID != p2ID {
		t.Fatalf("resolved[2] = (%q,%q), want (%q,%q)", resolved[2].Link.OntologyVersionID, resolved[2].SourceProjectID, v2, p2ID)
	}
}

func TestWeaveProjectStore_ResolvedOntologyVersions_PinnedParentRelease(t *testing.T) {
	pool := testPool(t)
	store := weave.NewPostgresStore(pool)
	projects := store.Projects()
	ctx := context.Background()

	ownerID := seedTestActor(t, pool, "TEST_ACTOR_PINNED_PARENT")

	parentID := "TEST_PROJ_PIN_PARENT"
	childID := "TEST_PROJ_PIN_CHILD"
	parentRelease := "1.2.0"
	parentHotVersion := "TEST_OV_PIN_HOT"
	parentReleasedVersion := "TEST_OV_PIN_RELEASED"
	childOwnVersion := "TEST_OV_PIN_CHILD"

	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, `DELETE FROM weave_project_ontology_versions_archive WHERE project_id LIKE 'TEST_%'`)
		_, _ = pool.Exec(bg, `DELETE FROM weave_projects_archive WHERE id LIKE 'TEST_%'`)
		_, _ = pool.Exec(bg, `DELETE FROM weave_project_ontology_versions WHERE project_id LIKE 'TEST_%'`)
		_, _ = pool.Exec(bg, `DELETE FROM weave_project_inheritance WHERE project_id LIKE 'TEST_%' OR parent_project_id LIKE 'TEST_%'`)
		_, _ = pool.Exec(bg, `DELETE FROM weave_projects WHERE id LIKE 'TEST_%'`)
		_, _ = pool.Exec(bg, `DELETE FROM weave_actors WHERE id = $1`, ownerID)
	})

	seedProject(t, pool, ctx, parentID, ownerID, nil)
	seedProject(t, pool, ctx, childID, ownerID, nil)
	seedInheritanceLinkWithSource(t, pool, ctx, childID, parentID, true, 0, domain.DependencySourceRelease, parentRelease)

	seedOntologyLink(t, pool, ctx, parentID, parentHotVersion, "parent hot")
	seedOntologyLink(t, pool, ctx, childID, childOwnVersion, "child own")

	if _, err := pool.Exec(ctx, `
		INSERT INTO weave_projects_archive (id, owner_id, parent_project_id, version_number)
		VALUES ($1, $2, NULL, $3)
		ON CONFLICT (id, version_number) DO NOTHING
	`, parentID, ownerID, parentRelease); err != nil {
		t.Fatalf("seed archived parent project: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO weave_project_ontology_versions_archive
			(project_id, ontology_version_id, added_at, is_primary, usage_notes, version_number)
		VALUES
			($1, $2, now(), false, 'parent released', $3)
	`, parentID, parentReleasedVersion, parentRelease); err != nil {
		t.Fatalf("seed archived parent ontology link: %v", err)
	}

	resolved, err := projects.ResolvedOntologyVersions(ctx, childID, domain.ResolvedOntologyVersionOpts{})
	if err != nil {
		t.Fatalf("ResolvedOntologyVersions pinned parent release: %v", err)
	}
	if len(resolved) != 2 {
		t.Fatalf("got %d resolved entries, want 2", len(resolved))
	}
	if resolved[0].Link.OntologyVersionID != childOwnVersion || resolved[0].SourceProjectID != "" {
		t.Fatalf("resolved[0] = (%q,%q), want own %q", resolved[0].Link.OntologyVersionID, resolved[0].SourceProjectID, childOwnVersion)
	}
	if resolved[1].Link.OntologyVersionID != parentReleasedVersion || resolved[1].SourceProjectID != parentID {
		t.Fatalf("resolved[1] = (%q,%q), want released parent (%q,%q)", resolved[1].Link.OntologyVersionID, resolved[1].SourceProjectID, parentReleasedVersion, parentID)
	}
	for _, row := range resolved {
		if row.Link.OntologyVersionID == parentHotVersion {
			t.Fatalf("hot parent ontology version %q should not appear for pinned parent release", parentHotVersion)
		}
	}
}

func TestWeaveProjectStore_ResolvedOntologyVersions_ArchivedInheritanceGraph(t *testing.T) {
	pool := testPool(t)
	store := weave.NewPostgresStore(pool)
	projects := store.Projects()
	ctx := weaveauth.WithProjectVersion(context.Background(), "0.1.1-test")

	ownerID := seedTestActor(t, pool, "TEST_ACTOR_ARCH_INHERIT")

	p1ID := "TEST_PROJ_ARCH_GRAPH_P1"
	p2ID := "TEST_PROJ_ARCH_GRAPH_P2"
	childID := "TEST_PROJ_ARCH_GRAPH_C"
	v1 := "TEST_OV_ARCH_GRAPH_V1"
	v2 := "TEST_OV_ARCH_GRAPH_V2"
	v3 := "TEST_OV_ARCH_GRAPH_V3"

	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, `DELETE FROM weave_project_inheritance_archive WHERE project_id LIKE 'TEST_%' OR parent_project_id LIKE 'TEST_%'`)
		_, _ = pool.Exec(bg, `DELETE FROM weave_project_ontology_versions_archive WHERE project_id LIKE 'TEST_%'`)
		_, _ = pool.Exec(bg, `DELETE FROM weave_projects_archive WHERE id LIKE 'TEST_%'`)
		_, _ = pool.Exec(bg, `DELETE FROM weave_project_inheritance WHERE project_id LIKE 'TEST_%' OR parent_project_id LIKE 'TEST_%'`)
		_, _ = pool.Exec(bg, `DELETE FROM weave_projects WHERE id LIKE 'TEST_%'`)
		_, _ = pool.Exec(bg, `DELETE FROM weave_actors WHERE id = $1`, ownerID)
	})

	seedProject(t, pool, context.Background(), p1ID, ownerID, nil)
	seedProject(t, pool, context.Background(), p2ID, ownerID, nil)
	seedProject(t, pool, context.Background(), childID, ownerID, nil)

	if _, err := pool.Exec(context.Background(), `
		INSERT INTO weave_projects_archive (id, owner_id, parent_project_id, version_number)
		VALUES
			($1, $4, NULL, $5),
			($2, $4, NULL, $5),
			($3, $4, NULL, $5)
	`, p1ID, p2ID, childID, ownerID, "0.1.1-test"); err != nil {
		t.Fatalf("seed archived projects: %v", err)
	}
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO weave_project_inheritance_archive (
			project_id, parent_project_id, is_primary, canonical_order, adopted_at, version_number
		) VALUES
			($1, $2, true, 0, now(), $4),
			($1, $3, false, 1, now(), $4)
	`, childID, p1ID, p2ID, "0.1.1-test"); err != nil {
		t.Fatalf("seed archived inheritance graph: %v", err)
	}
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO weave_project_ontology_versions_archive
			(project_id, ontology_version_id, added_at, is_primary, usage_notes, version_number)
		VALUES
			($1, $4, now(), false, 'p1', $7),
			($2, $5, now(), false, 'p2', $7),
			($3, $6, now(), false, 'child', $7)
	`, p1ID, p2ID, childID, v1, v2, v3, "0.1.1-test"); err != nil {
		t.Fatalf("seed archived ontology links: %v", err)
	}

	resolved, err := projects.ResolvedOntologyVersions(ctx, childID, domain.ResolvedOntologyVersionOpts{})
	if err != nil {
		t.Fatalf("ResolvedOntologyVersions archived inheritance graph: %v", err)
	}
	if len(resolved) != 3 {
		t.Fatalf("got %d resolved archived entries, want 3", len(resolved))
	}
	if resolved[0].Link.OntologyVersionID != v3 || resolved[0].SourceProjectID != "" {
		t.Fatalf("resolved[0] = (%q,%q), want own %q", resolved[0].Link.OntologyVersionID, resolved[0].SourceProjectID, v3)
	}
	if resolved[1].Link.OntologyVersionID != v1 || resolved[1].SourceProjectID != p1ID {
		t.Fatalf("resolved[1] = (%q,%q), want (%q,%q)", resolved[1].Link.OntologyVersionID, resolved[1].SourceProjectID, v1, p1ID)
	}
	if resolved[2].Link.OntologyVersionID != v2 || resolved[2].SourceProjectID != p2ID {
		t.Fatalf("resolved[2] = (%q,%q), want (%q,%q)", resolved[2].Link.OntologyVersionID, resolved[2].SourceProjectID, v2, p2ID)
	}
}
