//go:build integration

package collection

import (
	"context"
	"testing"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/auth"
)

// TestListUsage_VersionAware is the regression test for Redmine #3580:
// ListUsage must honor auth.ProjectVersionFromContext the same way
// field.ListUsage already does. A non-editor viewing a released project
// version must see the RELEASE's linkage, not whatever has been added to
// the live tables since the release was cut.
func TestListUsage_VersionAware(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()

	const (
		targetCollectionID = "TSTCUV.1" // the collection whose reuse ListUsage reports
		containerProj      = "TSTCUVP"  // project owning the container models
		archivedModelID    = "TSTCUV.2" // model archived at release "version"
		liveOnlyModelID    = "TSTCUV.3" // model created AFTER the release
		version            = "1.0.0"
	)

	_, err := pool.Exec(ctx, `INSERT INTO weave_projects (id, owner_id) VALUES ($1, 'unite')
		ON CONFLICT (id) DO NOTHING`, containerProj)
	if err != nil {
		t.Fatalf("seed project: %v", err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO weave_collections (id, project_id) VALUES ($1,$2)
		ON CONFLICT (id) DO NOTHING`, targetCollectionID, containerProj)
	if err != nil {
		t.Fatalf("seed target collection: %v", err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO weave_models (id, project_id) VALUES ($1,$2)
		ON CONFLICT (id) DO NOTHING`, archivedModelID, containerProj)
	if err != nil {
		t.Fatalf("seed archived container model: %v", err)
	}

	// Archived reference: a live field-override bundling the collection
	// into the model, ALSO snapshotted into the _archive tables at
	// "version" the way release creation would.
	_, err = pool.Exec(ctx, `INSERT INTO weave_field_overrides (field_id, project_id, entity_type, entity_id, part_of_collection_id)
		VALUES ('TSTCUVF.1', $1, 'model', $2, $3)`, containerProj, archivedModelID, targetCollectionID)
	if err != nil {
		t.Fatalf("seed archived override: %v", err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO weave_collections_archive (id, project_id, version_number) VALUES ($1,$2,$3)`,
		targetCollectionID, containerProj, version)
	if err != nil {
		t.Fatalf("archive target collection: %v", err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO weave_models_archive (id, project_id, version_number) VALUES ($1,$2,$3)`,
		archivedModelID, containerProj, version)
	if err != nil {
		t.Fatalf("archive container model: %v", err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO weave_field_overrides_archive (field_id, project_id, entity_type, entity_id, part_of_collection_id, version_number)
		VALUES ('TSTCUVF.1', $1, 'model', $2, $3, $4)`, containerProj, archivedModelID, targetCollectionID, version)
	if err != nil {
		t.Fatalf("archive override: %v", err)
	}

	// Live-only reference, created AFTER the release: must appear on the
	// live (no-version) path but must NOT appear under the pinned version.
	_, err = pool.Exec(ctx, `INSERT INTO weave_models (id, project_id) VALUES ($1,$2)
		ON CONFLICT (id) DO NOTHING`, liveOnlyModelID, containerProj)
	if err != nil {
		t.Fatalf("seed live-only container model: %v", err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO weave_field_overrides (field_id, project_id, entity_type, entity_id, part_of_collection_id)
		VALUES ('TSTCUVF.2', $1, 'model', $2, $3)`, containerProj, liveOnlyModelID, targetCollectionID)
	if err != nil {
		t.Fatalf("seed live-only override: %v", err)
	}

	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = pool.Exec(ctx, `DELETE FROM weave_field_overrides_archive WHERE project_id=$1`, containerProj)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_models_archive WHERE project_id=$1`, containerProj)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_collections_archive WHERE project_id=$1`, containerProj)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_field_overrides WHERE project_id=$1`, containerProj)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_models WHERE project_id=$1`, containerProj)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_collections WHERE project_id=$1`, containerProj)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_projects WHERE id=$1`, containerProj)
	})

	store := NewPostgresStore(pool)

	live, err := store.ListUsage(ctx, targetCollectionID, containerProj)
	if err != nil {
		t.Fatalf("ListUsage (live): %v", err)
	}
	if len(live) != 2 {
		t.Fatalf("live ListUsage = %d, want 2 (both containers): %+v", len(live), live)
	}

	versionedCtx := auth.WithProjectVersion(ctx, version)
	versioned, err := store.ListUsage(versionedCtx, targetCollectionID, containerProj)
	if err != nil {
		t.Fatalf("ListUsage (version): %v", err)
	}
	if len(versioned) != 1 {
		t.Fatalf("versioned ListUsage = %d, want 1 (release-consistent only): %+v", len(versioned), versioned)
	}
	if versioned[0].ID != archivedModelID {
		t.Errorf("versioned ListUsage[0].ID = %q, want %q", versioned[0].ID, archivedModelID)
	}
}
