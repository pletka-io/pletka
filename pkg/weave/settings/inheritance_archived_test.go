//go:build integration

package settings

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/internal/testdb"
	weaveauth "github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/weave/release"
)

// zarchReleaseVersions are the only release versions this test file ever
// creates, kept in an 8.x range distinct from other packages' synthetic
// fixtures (e.g. release's own ZREL tests use 9.0.x) so a shared test
// database clone never sees a version collision on the namespace-bindings
// archive cleanup below, which can only be scoped by version_number (see
// pkg/weave/release/service_release_outbox_test.go's zrelReleaseVersions
// comment for why).
var zarchReleaseVersions = []string{"8.8.8"}

// seedZARCH creates a synthetic project ("ZARCH") with the minimal draft
// entities release.Create requires (one category, field, model, and base
// override), mirroring seedZREL in
// pkg/weave/release/service_release_outbox_test.go. Kept package-local
// because this test lives in a different package (settings, not release)
// and only needs a project the release service can snapshot — not the full
// release-package test surface.
func seedZARCH(t *testing.T, pool *pgxpool.Pool) (ctx context.Context, ownerID, projectID string) {
	t.Helper()
	ctx = context.Background()
	ownerID = "ZARCH_OWNER"
	projectID = "ZARCH"

	purge := func() {
		bg := context.Background()
		for _, stmt := range []string{
			`DELETE FROM weave_change_set WHERE project_id='ZARCH'`,
			`DELETE FROM weave_releases WHERE project_id='ZARCH'`,
			`DELETE FROM weave_field_overrides WHERE entity_id LIKE 'ZARCH%' OR field_id IN (SELECT id FROM weave_fields WHERE project_id='ZARCH')`,
			`DELETE FROM weave_fields WHERE project_id='ZARCH'`,
			`DELETE FROM weave_models WHERE project_id='ZARCH'`,
			`DELETE FROM weave_categories WHERE project_id='ZARCH'`,
			`DELETE FROM weave_memberships WHERE scope_type='project' AND scope_id='ZARCH'`,
			`DELETE FROM weave_projects WHERE id='ZARCH'`,
			`DELETE FROM weave_actors WHERE id='ZARCH_OWNER'`,
			`DELETE FROM weave_field_overrides_archive WHERE project_id='ZARCH'`,
			`DELETE FROM weave_fields_archive WHERE project_id='ZARCH'`,
			`DELETE FROM weave_models_archive WHERE project_id='ZARCH'`,
			`DELETE FROM weave_categories_archive WHERE project_id='ZARCH'`,
			`DELETE FROM weave_projects_archive WHERE id='ZARCH'`,
			`DELETE FROM weave_namespace_bindings_archive WHERE version_number = ANY($1)`,
		} {
			if strings.Contains(stmt, "$1") {
				_, _ = pool.Exec(bg, stmt, zarchReleaseVersions)
				continue
			}
			_, _ = pool.Exec(bg, stmt)
		}
	}
	purge()
	t.Cleanup(purge)

	mustExec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("seed %q: %v", sql, err)
		}
	}

	uiName, _ := json.Marshal(map[string]string{"en": "Archived Pin Probe"})
	mustExec(`INSERT INTO weave_actors (id, type, display_name, slug, visibility, created_at, updated_at)
	          VALUES ('ZARCH_OWNER','organization','Archived Pin Probe Org','zarch-owner','private',NOW(),NOW())`)
	mustExec(`INSERT INTO weave_projects (id, ui_name, description, status, owner_id, visibility, created_at, updated_at)
	          VALUES ($1,$2,$2,'draft','ZARCH_OWNER','private',NOW(),NOW())`, projectID, uiName)
	mustExec(`INSERT INTO weave_categories (id, semantic_id, project_id, ui_name, status, created_at, updated_at)
	          VALUES ('ZARCH.CAT.1','ZARCH.CAT.1',$1,$2,'draft',NOW(),NOW())`, projectID, uiName)
	mustExec(`INSERT INTO weave_fields (id, semantic_id, system_name, project_id, ui_name, status, path_elements, created_at, updated_at)
	          VALUES ('ZARCHF1','ZARCHF.1','archived_pin_probe',$1,$2,'draft','[]',NOW(),NOW())`, projectID, uiName)
	mustExec(`INSERT INTO weave_models (id, project_id, ui_name, status, created_at, updated_at)
	          VALUES ('ZARCHM.1',$1,$2,'draft',NOW(),NOW())`, projectID, uiName)
	mustExec(`INSERT INTO weave_field_overrides (field_id, entity_type, entity_id, project_id, created_at, updated_at)
	          VALUES ('ZARCHF1','model','ZARCHM.1',$1,NOW(),NOW())`, projectID)

	return ctx, ownerID, projectID
}

func archivedPinAuthCtx(ctx context.Context, ownerID string) context.Context {
	authCtx := weaveauth.WithSnapshot(ctx, &weaveauth.AuthSnapshot{IsSuperAdmin: true})
	return weaveauth.WithPrincipal(authCtx, &weaveauth.Principal{ActorID: ownerID})
}

// TestArchivedRelease_ExcludedFromVersionsAndFlaggedArchived exercises the
// two store behaviors validateInheritanceSource relies on: an archived
// release must disappear from ReleaseVersions (the pin dropdown source) and
// ReleaseArchived must report true for it, so the handler can distinguish
// "archived" from "never existed" in its !found branch. Testing at the
// store level is deliberate — the handler branch itself is 5 lines wired
// directly to these two store calls (handler.go's validateInheritanceSource).
func TestArchivedRelease_ExcludedFromVersionsAndFlaggedArchived(t *testing.T) {
	pool := testdb.Pool(t)
	ctx, ownerID, projectID := seedZARCH(t, pool)
	authCtx := archivedPinAuthCtx(ctx, ownerID)

	releaseSvc := release.NewService(release.NewPostgresStore(pool), pool, nil)
	if _, err := releaseSvc.Create(authCtx, projectID, release.CreateInput{Version: "8.8.8"}); err != nil {
		t.Fatalf("release Create: %v", err)
	}

	store := NewPostgresStore(pool)

	versionsBefore, err := store.ReleaseVersions(ctx, projectID)
	if err != nil {
		t.Fatalf("ReleaseVersions before archive: %v", err)
	}
	if !containsVersion(versionsBefore, "8.8.8") {
		t.Fatalf("expected 8.8.8 in versions before archive, got %v", versionsBefore)
	}

	archivedBefore, err := store.ReleaseArchived(ctx, projectID, "8.8.8")
	if err != nil {
		t.Fatalf("ReleaseArchived before archive: %v", err)
	}
	if archivedBefore {
		t.Fatalf("expected 8.8.8 to not be archived yet")
	}

	if _, err := releaseSvc.Archive(authCtx, projectID, "8.8.8", release.ArchiveInput{Message: "superseded"}); err != nil {
		t.Fatalf("release Archive: %v", err)
	}

	versionsAfter, err := store.ReleaseVersions(ctx, projectID)
	if err != nil {
		t.Fatalf("ReleaseVersions after archive: %v", err)
	}
	if containsVersion(versionsAfter, "8.8.8") {
		t.Fatalf("expected 8.8.8 to be excluded from versions after archive, got %v", versionsAfter)
	}

	archivedAfter, err := store.ReleaseArchived(ctx, projectID, "8.8.8")
	if err != nil {
		t.Fatalf("ReleaseArchived after archive: %v", err)
	}
	if !archivedAfter {
		t.Fatalf("expected 8.8.8 to report archived=true")
	}

	// A version that never existed at all must report archived=false, not
	// an error — this is the pgx.ErrNoRows -> false path in ReleaseArchived.
	neverExisted, err := store.ReleaseArchived(ctx, projectID, "8.8.9")
	if err != nil {
		t.Fatalf("ReleaseArchived for nonexistent version: %v", err)
	}
	if neverExisted {
		t.Fatalf("expected nonexistent version to report archived=false")
	}
}

func containsVersion(versions []string, target string) bool {
	for _, v := range versions {
		if v == target {
			return true
		}
	}
	return false
}
