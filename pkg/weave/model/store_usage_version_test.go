//go:build integration

package model

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
		targetModelID = "TSTMUV.1" // the model whose reuse ListUsage reports
		containerProj = "TSTMUVP"  // project owning the container models
		archivedCtrID = "TSTMUV.2" // container archived at release "version"
		liveOnlyCtrID = "TSTMUV.3" // container created AFTER the release
		version       = "1.0.0"
	)

	_, err := pool.Exec(ctx, `INSERT INTO weave_projects (id, owner_id) VALUES ($1, 'unite')
		ON CONFLICT (id) DO NOTHING`, containerProj)
	if err != nil {
		t.Fatalf("seed project: %v", err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO weave_models (id, project_id) VALUES ($1,$2), ($3,$2)
		ON CONFLICT (id) DO NOTHING`, targetModelID, containerProj, archivedCtrID)
	if err != nil {
		t.Fatalf("seed target + archived container models: %v", err)
	}

	// Archived reference: a live override + override-ref, ALSO snapshotted
	// into the _archive tables at "version" the way release creation would.
	var archivedOverrideID int64
	err = pool.QueryRow(ctx, `INSERT INTO weave_field_overrides (field_id, project_id, entity_type, entity_id)
		VALUES ('TSTMUVF.1', $1, 'model', $2) RETURNING id`, containerProj, archivedCtrID).Scan(&archivedOverrideID)
	if err != nil {
		t.Fatalf("seed archived override: %v", err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO weave_override_refs (override_id, ref_type, target_id, semantic_id, position)
		VALUES ($1, 'resource_model', $2, $2, 0)`, archivedOverrideID, targetModelID)
	if err != nil {
		t.Fatalf("seed archived override ref: %v", err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO weave_models_archive (id, project_id, version_number) VALUES ($1,$2,$3)`,
		archivedCtrID, containerProj, version)
	if err != nil {
		t.Fatalf("archive container model: %v", err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO weave_field_overrides_archive (id, field_id, project_id, entity_type, entity_id, version_number)
		VALUES ($1, 'TSTMUVF.1', $2, 'model', $3, $4)`, archivedOverrideID, containerProj, archivedCtrID, version)
	if err != nil {
		t.Fatalf("archive override: %v", err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO weave_override_refs_archive (override_id, ref_type, target_id, semantic_id, position, project_id, version_number)
		VALUES ($1, 'resource_model', $2, $2, 0, $3, $4)`, archivedOverrideID, targetModelID, containerProj, version)
	if err != nil {
		t.Fatalf("archive override ref: %v", err)
	}

	// Live-only reference, created AFTER the release: must appear on the
	// live (no-version) path but must NOT appear under the pinned version.
	_, err = pool.Exec(ctx, `INSERT INTO weave_models (id, project_id) VALUES ($1,$2)
		ON CONFLICT (id) DO NOTHING`, liveOnlyCtrID, containerProj)
	if err != nil {
		t.Fatalf("seed live-only container model: %v", err)
	}
	var liveOverrideID int64
	err = pool.QueryRow(ctx, `INSERT INTO weave_field_overrides (field_id, project_id, entity_type, entity_id)
		VALUES ('TSTMUVF.2', $1, 'model', $2) RETURNING id`, containerProj, liveOnlyCtrID).Scan(&liveOverrideID)
	if err != nil {
		t.Fatalf("seed live-only override: %v", err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO weave_override_refs (override_id, ref_type, target_id, semantic_id, position)
		VALUES ($1, 'resource_model', $2, $2, 0)`, liveOverrideID, targetModelID)
	if err != nil {
		t.Fatalf("seed live-only override ref: %v", err)
	}

	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = pool.Exec(ctx, `DELETE FROM weave_override_refs_archive WHERE project_id=$1`, containerProj)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_field_overrides_archive WHERE project_id=$1`, containerProj)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_models_archive WHERE project_id=$1`, containerProj)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_field_overrides WHERE project_id=$1`, containerProj)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_models WHERE project_id=$1`, containerProj)
		_, _ = pool.Exec(ctx, `DELETE FROM weave_projects WHERE id=$1`, containerProj)
	})

	store := NewPostgresStore(pool)

	live, err := store.ListUsage(ctx, targetModelID)
	if err != nil {
		t.Fatalf("ListUsage (live): %v", err)
	}
	if len(live.Models) != 2 {
		t.Fatalf("live ListUsage.Models = %d, want 2 (both containers): %+v", len(live.Models), live.Models)
	}

	versionedCtx := auth.WithProjectVersion(ctx, version)
	versioned, err := store.ListUsage(versionedCtx, targetModelID)
	if err != nil {
		t.Fatalf("ListUsage (version): %v", err)
	}
	if len(versioned.Models) != 1 {
		t.Fatalf("versioned ListUsage.Models = %d, want 1 (release-consistent only): %+v", len(versioned.Models), versioned.Models)
	}
	if versioned.Models[0].ID != archivedCtrID {
		t.Errorf("versioned ListUsage.Models[0].ID = %q, want %q", versioned.Models[0].ID, archivedCtrID)
	}
}
