//go:build integration

package hub

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/internal/testdb"
	weaveauth "github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/weave"
)

// seedZHUB creates a synthetic project ("ZHUB") owned by a synthetic
// organization actor. loadProjectAndGate only needs the project row (via
// h.weave.Projects().GetByID), so this is deliberately smaller than
// settings' seedZARCH (which also seeds a category/field/model/override
// for release.Create) — the hub gate never reads those.
func seedZHUB(t *testing.T, pool *pgxpool.Pool) (ownerID, projectID string) {
	t.Helper()
	ctx := context.Background()
	ownerID = "ZHUB_OWNER"
	projectID = "ZHUB"

	purge := func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, `DELETE FROM weave_memberships WHERE scope_type='project' AND scope_id=$1`, projectID)
		_, _ = pool.Exec(bg, `DELETE FROM weave_projects WHERE id=$1`, projectID)
		_, _ = pool.Exec(bg, `DELETE FROM weave_actors WHERE id=$1`, ownerID)
	}
	purge()
	t.Cleanup(purge)

	uiName, _ := json.Marshal(map[string]string{"en": "Hub Org Inheritance Probe"})
	if _, err := pool.Exec(ctx, `INSERT INTO weave_actors (id, type, display_name, slug, visibility, created_at, updated_at)
		VALUES ($1,'organization','ZHUB Owner Org','zhub-owner','private',NOW(),NOW())`, ownerID); err != nil {
		t.Fatalf("seed org actor: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO weave_projects (id, ui_name, description, status, owner_id, visibility, created_at, updated_at)
		VALUES ($1,$2,$2,'draft',$3,'private',NOW(),NOW())`, projectID, uiName, ownerID); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	return ownerID, projectID
}

// TestLoadProjectAndGate_HonorsOrgInheritedRole guards the audit-H1 fix: the
// hub slice's local projectResource helper built auth.Resource without
// OrgID, so an org owner/admin with no explicit project membership got 403
// on every Integrations settings endpoint (ListSchema, AddConfig,
// UpdateConfig, SetEnabled, Remove, RunAction all gate through
// loadProjectAndGate). loadProjectAndGate now uses auth.ProjectResource,
// which carries OrgID = project.OwnerID, so org-inherited access resolves.
func TestLoadProjectAndGate_HonorsOrgInheritedRole(t *testing.T) {
	pool := testdb.Pool(t)
	ownerID, projectID := seedZHUB(t, pool)

	h := NewHandler(weave.NewPostgresStore(pool), nil, nil, nil, nil, slog.Default())

	// Org-inherited access: no "project:<id>" role, only "org:<ownerID>".
	orgUser := weaveauth.WithSnapshot(context.Background(), &weaveauth.AuthSnapshot{
		ActorID: "org-inherited-user",
		Roles:   map[string]string{"org:" + ownerID: "owner"},
	})
	rec := httptest.NewRecorder()
	project, ok := h.loadProjectAndGate(orgUser, rec, projectID, weaveauth.ProjectEdit)
	if !ok {
		t.Fatalf("org-inherited owner was denied hub access (status %d); loadProjectAndGate must carry OrgID", rec.Code)
	}
	if project == nil {
		t.Fatal("expected the loaded project on success")
	}

	// No roles at all must still be denied.
	nobody := weaveauth.WithSnapshot(context.Background(), &weaveauth.AuthSnapshot{
		ActorID: "unrelated-user",
		Roles:   map[string]string{},
	})
	rec2 := httptest.NewRecorder()
	if _, ok := h.loadProjectAndGate(nobody, rec2, projectID, weaveauth.ProjectEdit); ok {
		t.Fatal("a non-member must not pass the hub gate")
	}
}
