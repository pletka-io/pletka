//go:build integration

package attribution

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/internal/testdb"
	weaveauth "github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/weave"
)

// seedZATTR creates a synthetic project ("ZATTR") owned by a synthetic
// organization actor. requireProject only needs the project row (via
// s.projects.GetByID), so this is deliberately as small as hub's seedZHUB.
func seedZATTR(t *testing.T, pool *pgxpool.Pool) (ownerID, projectID string) {
	t.Helper()
	ctx := context.Background()
	ownerID = "ZATTR_OWNER"
	projectID = "ZATTR"

	purge := func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, `DELETE FROM weave_memberships WHERE scope_type='project' AND scope_id=$1`, projectID)
		_, _ = pool.Exec(bg, `DELETE FROM weave_projects WHERE id=$1`, projectID)
		_, _ = pool.Exec(bg, `DELETE FROM weave_actors WHERE id=$1`, ownerID)
	}
	purge()
	t.Cleanup(purge)

	uiName, _ := json.Marshal(map[string]string{"en": "Attribution Org Inheritance Probe"})
	if _, err := pool.Exec(ctx, `INSERT INTO weave_actors (id, type, display_name, slug, visibility, created_at, updated_at)
		VALUES ($1,'organization','ZATTR Owner Org','zattr-owner','private',NOW(),NOW())`, ownerID); err != nil {
		t.Fatalf("seed org actor: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO weave_projects (id, ui_name, description, status, owner_id, visibility, created_at, updated_at)
		VALUES ($1,$2,$2,'draft',$3,'private',NOW(),NOW())`, projectID, uiName, ownerID); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	return ownerID, projectID
}

// TestRequireProjectEdit_HonorsOrgInheritedRole guards the fix for the same
// OrgID-drop bug found in settings and hub: requireProject built its auth
// resource with OrgID="" (dropping org inheritance), so a user whose only
// route to the project was an org-level role (no explicit "project:<id>"
// membership) was denied. requireProject now uses auth.ProjectResource,
// which carries OrgID = project.OwnerID, so org-inherited access resolves.
func TestRequireProjectEdit_HonorsOrgInheritedRole(t *testing.T) {
	pool := testdb.Pool(t)
	ownerID, projectID := seedZATTR(t, pool)

	store := NewPostgresStore(pool)
	projects := weave.NewPostgresStore(pool).Projects()
	svc := NewService(store, projects, slog.Default())

	// Org-inherited access: no "project:<id>" role, only "org:<ownerID>".
	// Org role vocabulary is "owner"/"admin"/"member" (auth.AuthSnapshot.
	// EffectiveRole) — "owner" resolves to inherited project "owner",
	// which has ProjectEdit.
	orgUser := weaveauth.WithSnapshot(context.Background(), &weaveauth.AuthSnapshot{
		ActorID: "org-inherited-user",
		Roles:   map[string]string{"org:" + ownerID: "owner"},
	})
	project, err := svc.requireProjectEdit(orgUser, projectID)
	if err != nil {
		t.Fatalf("org-inherited owner was denied requireProjectEdit: %v; requireProject must carry OrgID", err)
	}
	if project == nil {
		t.Fatal("expected the loaded project on success")
	}

	// No roles at all must still be denied.
	nobody := weaveauth.WithSnapshot(context.Background(), &weaveauth.AuthSnapshot{
		ActorID: "unrelated-user",
		Roles:   map[string]string{},
	})
	if _, err := svc.requireProjectEdit(nobody, projectID); err != ErrForbidden {
		t.Fatalf("non-member requireProjectEdit err = %v, want ErrForbidden", err)
	}
}
