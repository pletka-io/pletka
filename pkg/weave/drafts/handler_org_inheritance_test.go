//go:build integration

package drafts

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/internal/testdb"
	weaveauth "github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/weave"
)

// seedZDRFT creates a synthetic project ("ZDRFT") owned by a synthetic
// organization actor. Create only needs the project row (via
// h.weave.Projects().GetByID), so this is deliberately as small as hub's
// seedZHUB / attribution's seedZATTR.
func seedZDRFT(t *testing.T, pool *pgxpool.Pool) (ownerID, projectID string) {
	t.Helper()
	ctx := context.Background()
	ownerID = "ZDRFT_OWNER"
	projectID = "ZDRFT"

	purge := func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, `DELETE FROM weave_categories WHERE project_id=$1`, projectID)
		_, _ = pool.Exec(bg, `DELETE FROM weave_entity_counters WHERE project_id=$1`, projectID)
		_, _ = pool.Exec(bg, `DELETE FROM weave_memberships WHERE scope_type='project' AND scope_id=$1`, projectID)
		_, _ = pool.Exec(bg, `DELETE FROM weave_projects WHERE id=$1`, projectID)
		_, _ = pool.Exec(bg, `DELETE FROM weave_actors WHERE id=$1`, ownerID)
	}
	purge()
	t.Cleanup(purge)

	uiName, _ := json.Marshal(map[string]string{"en": "Drafts Org Inheritance Probe"})
	if _, err := pool.Exec(ctx, `INSERT INTO weave_actors (id, type, display_name, slug, visibility, created_at, updated_at)
		VALUES ($1,'organization','ZDRFT Owner Org','zdrft-owner','private',NOW(),NOW())`, ownerID); err != nil {
		t.Fatalf("seed org actor: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO weave_projects (id, ui_name, description, status, owner_id, visibility, created_at, updated_at)
		VALUES ($1,$2,$2,'draft',$3,'private',NOW(),NOW())`, projectID, uiName, ownerID); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	return ownerID, projectID
}

// TestCreate_HonorsOrgInheritedRole guards the same OrgID-drop bug fixed in
// settings, hub, and attribution: Create built its auth resource with
// OrgID="" (dropping org inheritance), so a user whose only route to the
// project was an org-level role (no explicit "project:<id>" membership) was
// denied at the only gate on POST /api/v1/drafts. Create now uses
// auth.ProjectResource, which carries OrgID = project.OwnerID, so
// org-inherited access resolves.
func TestCreate_HonorsOrgInheritedRole(t *testing.T) {
	pool := testdb.Pool(t)
	ownerID, projectID := seedZDRFT(t, pool)

	h := NewHandler(weave.NewPostgresStore(pool), slog.Default())

	body := func() *bytes.Buffer {
		b, err := json.Marshal(map[string]any{
			"type":       "category",
			"project_id": projectID,
			"name":       map[string]string{"en": "Org Inherited Draft Category"},
		})
		if err != nil {
			t.Fatalf("marshal request body: %v", err)
		}
		return bytes.NewBuffer(b)
	}

	// Org-inherited access: no "project:<id>" role, only "org:<ownerID>".
	orgUser := weaveauth.WithSnapshot(context.Background(), &weaveauth.AuthSnapshot{
		ActorID: "org-inherited-user",
		Roles:   map[string]string{"org:" + ownerID: "owner"},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/drafts", body()).WithContext(orgUser)
	rec := httptest.NewRecorder()
	h.Create(rec, req)
	if rec.Code == http.StatusForbidden {
		t.Fatalf("org-inherited owner was denied the drafts Create gate (status %d); Create must carry OrgID", rec.Code)
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("org-inherited owner Create status = %d, want %d; body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	// No roles at all must still be denied.
	nobody := weaveauth.WithSnapshot(context.Background(), &weaveauth.AuthSnapshot{
		ActorID: "unrelated-user",
		Roles:   map[string]string{},
	})
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/drafts", body()).WithContext(nobody)
	rec2 := httptest.NewRecorder()
	h.Create(rec2, req2)
	if rec2.Code != http.StatusForbidden {
		t.Fatalf("non-member Create status = %d, want %d", rec2.Code, http.StatusForbidden)
	}
}
