//go:build integration

package project

import (
	"context"
	"testing"

	"github.com/pletka-io/pletka/internal/testdb"
	weaveauth "github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/ids"
)

// TestCanEdit_HonorsOrgInheritedRole guards audit finding H4: CanEdit used to
// take only (projectID, visibility) and hand-build an auth.Resource with an
// empty OrgID, so auth.EffectiveRole could never fold an org owner/admin
// into a project owner — org owners got "edit denied" on override saves and
// adoptions even though they own the project via their org. CanEdit now
// takes the loaded *domain.Project and builds its resource via
// auth.ProjectResource(p), which carries OrgID = p.OwnerID.
//
// Mirrors pkg/weave/settings/settings_org_inheritance_test.go
// (TestLoadProjectAndGate_HonorsOrgInheritedRole), which guards the same bug
// class in the settings slice.
func TestCanEdit_HonorsOrgInheritedRole(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()

	// "unite" is an existing fixture org actor (internal/testdb/fixture_identities.go)
	// seeded into every per-package clone — reuse it as the project owner
	// instead of inserting a new actor row.
	const ownerID = "unite"
	projectID := ids.GenerateULID()

	if _, err := pool.Exec(ctx,
		`INSERT INTO weave_projects (id, owner_id, visibility, ui_name) VALUES ($1, $2, 'private', $3)`,
		projectID, ownerID, []byte(`{"en":"CanEdit Org Inheritance Probe"}`),
	); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM weave_projects WHERE id = $1`, projectID)
	})

	store := NewPostgresStore(pool)
	proj, err := store.GetByID(ctx, projectID)
	if err != nil || proj == nil {
		t.Fatalf("load seeded project: %v", err)
	}

	// Lock the mechanism: auth.ProjectResource must carry OrgID, or org
	// inheritance can't resolve at all — same assertion the settings test
	// makes on its resource.
	if got := weaveauth.ProjectResource(proj).OrgID; got != ownerID {
		t.Fatalf("auth.ProjectResource(project).OrgID = %q, want %q", got, ownerID)
	}

	svc := NewService(store, nil, nil, nil, nil)

	// A user whose ONLY route to this project is an org-level role — no
	// "project:<id>" entry. Pre-fix, CanEdit's hand-built resource had an
	// empty OrgID, so this would resolve to no role and deny the edit.
	orgOwner := weaveauth.WithSnapshot(ctx, &weaveauth.AuthSnapshot{
		ActorID: "org-inherited-editor",
		Roles:   map[string]string{"org:" + ownerID: "owner"},
	})
	if !svc.CanEdit(orgOwner, proj) {
		t.Fatal("org-inherited owner was denied CanEdit; auth.ProjectResource(p) must carry OrgID")
	}

	// A user with neither an org role nor a project role must still be denied.
	nobody := weaveauth.WithSnapshot(ctx, &weaveauth.AuthSnapshot{
		ActorID: "unrelated-user",
		Roles:   map[string]string{},
	})
	if svc.CanEdit(nobody, proj) {
		t.Fatal("a non-member must not pass CanEdit")
	}
}
