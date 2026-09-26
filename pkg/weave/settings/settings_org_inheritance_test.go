//go:build integration

package settings

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	weaveauth "github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/weave"
)

// TestLoadProjectAndGate_HonorsOrgInheritedRole guards the fix for the reported
// bug where a user with an org-inherited role (no explicit project membership)
// saw the Settings tab but got a 403 opening it: the settings slice built its
// auth resource with OrgID="" (dropping org inheritance), while the tab and
// shell used auth.ProjectResource (which carries OrgID). loadProjectAndGate now
// uses auth.ProjectResource, so org-inherited access resolves consistently.
func TestLoadProjectAndGate_HonorsOrgInheritedRole(t *testing.T) {
	pool := testdb.Pool(t)
	_, ownerID, projectID := seedZARCH(t, pool)

	h := NewHandler(weave.NewPostgresStore(pool), NewPostgresStore(pool), nil, nil, slog.Default())

	// A user whose ONLY route to this project is an org-level role — no
	// "project:<id>" entry. Pre-fix, the settings resource's empty OrgID made
	// this resolve to no role → 403.
	orgUser := weaveauth.WithSnapshot(context.Background(), &weaveauth.AuthSnapshot{
		ActorID: "org-inherited-user",
		Roles:   map[string]string{"org:" + ownerID: "owner"},
	})
	rec := httptest.NewRecorder()
	project, res, ok := h.loadProjectAndGate(orgUser, rec, projectID, weaveauth.ProjectEdit)
	if !ok {
		t.Fatalf("org-inherited owner was denied settings access (status %d); the settings resource must carry OrgID", rec.Code)
	}
	if project == nil || res == nil {
		t.Fatal("expected the project + resource on success")
	}
	if res.OrgID != ownerID {
		t.Fatalf("resource OrgID = %q, want %q (org inheritance can't resolve without it)", res.OrgID, ownerID)
	}

	// A user with neither an org role nor a project role must still be denied.
	nobody := weaveauth.WithSnapshot(context.Background(), &weaveauth.AuthSnapshot{
		ActorID: "unrelated-user",
		Roles:   map[string]string{},
	})
	rec2 := httptest.NewRecorder()
	if _, _, ok := h.loadProjectAndGate(nobody, rec2, projectID, weaveauth.ProjectEdit); ok {
		t.Fatal("a non-member must not pass the settings gate")
	}
	if rec2.Code != http.StatusForbidden {
		t.Fatalf("non-member status = %d, want %d", rec2.Code, http.StatusForbidden)
	}
}
