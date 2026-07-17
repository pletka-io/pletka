package auth_test

import (
	"testing"

	"github.com/pletka-io/pletka/pkg/auth"
)

func snap(actorID string, roles map[string]string, owned ...string) *auth.AuthSnapshot {
	s := &auth.AuthSnapshot{
		ActorID:         actorID,
		Roles:           roles,
		OwnedProjectIDs: make(map[string]struct{}, len(owned)),
	}
	for _, id := range owned {
		s.OwnedProjectIDs[id] = struct{}{}
	}
	return s
}

func TestEffectiveRole_DirectProjectOwner(t *testing.T) {
	s := snap("u1", map[string]string{}, "LA")
	r := auth.Resource{ScopeType: "project", ID: "LA", Visibility: "private"}
	if got := s.EffectiveRole(r); got != "owner" {
		t.Errorf("direct owner: got %q want owner", got)
	}
}

func TestEffectiveRole_ExplicitProjectMembership(t *testing.T) {
	s := snap("u1", map[string]string{"project:LA": "maintainer"})
	r := auth.Resource{ScopeType: "project", ID: "LA", Visibility: "private"}
	if got := s.EffectiveRole(r); got != "maintainer" {
		t.Errorf("explicit: got %q want maintainer", got)
	}
}

func TestEffectiveRole_InheritedFromOrgAdmin(t *testing.T) {
	s := snap("u1", map[string]string{"org:org1": "admin"})
	r := auth.Resource{ScopeType: "project", ID: "LA", OrgID: "org1", Visibility: "private"}
	if got := s.EffectiveRole(r); got != "owner" {
		t.Errorf("org admin inherit: got %q want owner", got)
	}
}

func TestEffectiveRole_OrgMemberSeesPublicOnly(t *testing.T) {
	s := snap("u1", map[string]string{"org:org1": "member"})
	pub := auth.Resource{ScopeType: "project", ID: "LA", OrgID: "org1", Visibility: "public"}
	priv := auth.Resource{ScopeType: "project", ID: "SRD", OrgID: "org1", Visibility: "private"}
	if got := s.EffectiveRole(pub); got != "viewer" {
		t.Errorf("org member on public: got %q want viewer", got)
	}
	if got := s.EffectiveRole(priv); got != "" {
		t.Errorf("org member on private: got %q want empty", got)
	}
}

func TestEffectiveRole_AnonymousOnPublic(t *testing.T) {
	s := &auth.AuthSnapshot{IsAnonymous: true}
	pub := auth.Resource{ScopeType: "project", ID: "LA", Visibility: "public"}
	priv := auth.Resource{ScopeType: "project", ID: "LA", Visibility: "private"}
	if got := s.EffectiveRole(pub); got != "viewer" {
		t.Errorf("anon public: got %q want viewer", got)
	}
	if got := s.EffectiveRole(priv); got != "" {
		t.Errorf("anon private: got %q want empty", got)
	}
}

func TestEffectiveRole_InheritanceBeatsExplicitWhenHigher(t *testing.T) {
	// Org owner inheritance gives "owner"; an explicit "viewer" row on
	// the same project does NOT reduce access (core decision #5: explicit
	// overrides upward, never reduces). Higher of {inherited, explicit}
	// wins. Here inheritance wins because org owner > project viewer.
	s := snap("u1", map[string]string{
		"org:org1":   "owner",
		"project:LA": "viewer",
	})
	r := auth.Resource{ScopeType: "project", ID: "LA", OrgID: "org1", Visibility: "private"}
	if got := s.EffectiveRole(r); got != "owner" {
		t.Errorf("explicit vs inherited: got %q want owner (inherited from org owner)", got)
	}
}

func TestEffectiveRole_ExplicitElevatesAboveInherited(t *testing.T) {
	// Org member would inherit nothing on a private project. An explicit
	// "maintainer" row elevates the user to maintainer.
	s := snap("u1", map[string]string{
		"org:org1":   "member",
		"project:LA": "maintainer",
	})
	r := auth.Resource{ScopeType: "project", ID: "LA", OrgID: "org1", Visibility: "private"}
	if got := s.EffectiveRole(r); got != "maintainer" {
		t.Errorf("explicit elevating: got %q want maintainer", got)
	}
}

func TestEffectiveRole_OrgScope(t *testing.T) {
	s := snap("u1", map[string]string{"org:org1": "admin"})
	r := auth.Resource{ScopeType: "org", ID: "org1"}
	if got := s.EffectiveRole(r); got != "admin" {
		t.Errorf("org scope: got %q want admin", got)
	}
	r2 := auth.Resource{ScopeType: "org", ID: "org2"}
	if got := s.EffectiveRole(r2); got != "" {
		t.Errorf("org non-member: got %q want empty", got)
	}
}

func TestCan_MaintainerDeletesPublishedField(t *testing.T) {
	s := snap("u1", map[string]string{"project:LA": "maintainer"})
	r := auth.Resource{ScopeType: "project", ID: "LA", Visibility: "private"}
	ent := &auth.EntityContext{CreatedByID: "other", Status: "published"}
	if !s.Can(auth.FieldDelete, r, ent) {
		t.Error("maintainer should delete published fields")
	}
}

func TestCan_ContributorDeletesOwnDraft(t *testing.T) {
	s := snap("u1", map[string]string{"project:LA": "contributor"})
	r := auth.Resource{ScopeType: "project", ID: "LA", Visibility: "private"}
	ent := &auth.EntityContext{CreatedByID: "u1", Status: "draft"}
	if !s.Can(auth.FieldDelete, r, ent) {
		t.Error("contributor should delete their OWN draft (ownership modifier)")
	}
}

func TestCan_ContributorCannotDeleteOwnPublished(t *testing.T) {
	s := snap("u1", map[string]string{"project:LA": "contributor"})
	r := auth.Resource{ScopeType: "project", ID: "LA", Visibility: "private"}
	ent := &auth.EntityContext{CreatedByID: "u1", Status: "published"}
	if s.Can(auth.FieldDelete, r, ent) {
		t.Error("contributor should NOT delete their own PUBLISHED field (ownership only covers drafts)")
	}
}

func TestCan_ContributorCannotDeleteOthersDraft(t *testing.T) {
	s := snap("u1", map[string]string{"project:LA": "contributor"})
	r := auth.Resource{ScopeType: "project", ID: "LA", Visibility: "private"}
	ent := &auth.EntityContext{CreatedByID: "other", Status: "draft"}
	if s.Can(auth.FieldDelete, r, ent) {
		t.Error("contributor should NOT delete someone else's draft")
	}
}

func TestCan_AnonymousOnPublicCanReadNotWrite(t *testing.T) {
	s := &auth.AuthSnapshot{IsAnonymous: true}
	r := auth.Resource{ScopeType: "project", ID: "LA", Visibility: "public"}
	if !s.Can(auth.FieldRead, r, nil) {
		t.Error("anon on public should read")
	}
	if s.Can(auth.FieldEdit, r, nil) {
		t.Error("anon on public should NOT edit")
	}
}

func TestCan_NilSnapshotIsAnonymous(t *testing.T) {
	var s *auth.AuthSnapshot
	rPub := auth.Resource{ScopeType: "project", ID: "LA", Visibility: "public"}
	rPriv := auth.Resource{ScopeType: "project", ID: "LA", Visibility: "private"}
	if !s.Can(auth.FieldRead, rPub, nil) {
		t.Error("nil snapshot should be treated as anonymous and see public reads")
	}
	if s.Can(auth.FieldRead, rPriv, nil) {
		t.Error("nil snapshot should NOT read private projects")
	}
}

func TestCan_RevokedMemberLosesOwnershipModifier(t *testing.T) {
	// Spec §Ownership modifier: creators can edit/delete their own drafts
	// as long as they still have read access to the project. If membership
	// is revoked and the project is private, they lose access — no orphan
	// editable drafts.
	s := snap("u1", map[string]string{} /* no memberships: just revoked */)
	rPriv := auth.Resource{ScopeType: "project", ID: "LA", Visibility: "private"}
	ent := &auth.EntityContext{CreatedByID: "u1", Status: "draft"}
	if s.Can(auth.FieldEdit, rPriv, ent) {
		t.Error("revoked member on private project should NOT edit even own drafts")
	}
	if s.Can(auth.FieldDelete, rPriv, ent) {
		t.Error("revoked member on private project should NOT delete even own drafts")
	}
}
