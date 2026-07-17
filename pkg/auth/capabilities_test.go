package auth_test

import (
	"testing"

	"github.com/pletka-io/pletka/pkg/auth"
)

func TestProjectRoleCapabilities_OwnerHasEverything(t *testing.T) {
	role := auth.RoleForScope("project", "owner")
	for _, cap := range auth.AllProjectCapabilities() {
		if !role.Has(cap) {
			t.Errorf("owner role missing project capability %q", cap)
		}
	}
}

func TestProjectRoleCapabilities_ViewerIsReadOnly(t *testing.T) {
	role := auth.RoleForScope("project", "viewer")
	allowed := []auth.Capability{
		auth.ProjectRead, auth.FieldRead, auth.ModelRead, auth.CollectionRead,
	}
	forbidden := []auth.Capability{
		auth.ProjectEdit, auth.ProjectDelete, auth.FieldCreate, auth.FieldEdit,
		auth.FieldPublish, auth.FieldDelete, auth.ModelEdit, auth.CollectionEdit,
		auth.CommentCreate,
	}
	for _, cap := range allowed {
		if !role.Has(cap) {
			t.Errorf("viewer should have %q", cap)
		}
	}
	for _, cap := range forbidden {
		if role.Has(cap) {
			t.Errorf("viewer should NOT have %q", cap)
		}
	}
}

func TestContributor_CannotEditOthersButCanCreate(t *testing.T) {
	role := auth.RoleForScope("project", "contributor")
	if !role.Has(auth.FieldCreate) {
		t.Error("contributor should have FieldCreate")
	}
	if role.Has(auth.FieldEdit) {
		t.Error("contributor should NOT have FieldEdit (ownership modifier handles own drafts)")
	}
	if role.Has(auth.FieldDelete) {
		t.Error("contributor should NOT have FieldDelete (ownership modifier handles own drafts)")
	}
}

func TestMaintainer_HasDeleteButNotProjectDelete(t *testing.T) {
	role := auth.RoleForScope("project", "maintainer")
	if !role.Has(auth.FieldDelete) {
		t.Error("maintainer should have FieldDelete")
	}
	if !role.Has(auth.ModelDelete) {
		t.Error("maintainer should have ModelDelete")
	}
	if !role.Has(auth.CollectionDelete) {
		t.Error("maintainer should have CollectionDelete")
	}
	if role.Has(auth.ProjectDelete) {
		t.Error("maintainer should NOT have ProjectDelete (owner-only)")
	}
}

func TestOrgRoles(t *testing.T) {
	cases := []struct {
		role string
		want []auth.Capability
		dont []auth.Capability
	}{
		{"owner", []auth.Capability{auth.OrgDelete, auth.OrgManageMembers, auth.OrgProjectCreate}, nil},
		{"admin", []auth.Capability{auth.OrgManageMembers, auth.OrgProjectCreate}, []auth.Capability{auth.OrgDelete}},
		{"member", []auth.Capability{auth.OrgRead}, []auth.Capability{auth.OrgEdit, auth.OrgManageMembers}},
	}
	for _, tc := range cases {
		t.Run(tc.role, func(t *testing.T) {
			r := auth.RoleForScope("org", tc.role)
			for _, cap := range tc.want {
				if !r.Has(cap) {
					t.Errorf("org.%s should have %q", tc.role, cap)
				}
			}
			for _, cap := range tc.dont {
				if r.Has(cap) {
					t.Errorf("org.%s should NOT have %q", tc.role, cap)
				}
			}
		})
	}
}

func TestOwnerSelfCaps(t *testing.T) {
	wantInSelf := []auth.Capability{
		auth.FieldEdit, auth.FieldDelete,
		auth.ModelEdit, auth.ModelDelete,
		auth.CollectionEdit, auth.CollectionDelete,
	}
	for _, cap := range wantInSelf {
		if !auth.OwnerSelfCaps().Has(cap) {
			t.Errorf("OwnerSelfCaps should include %q", cap)
		}
	}
	if auth.OwnerSelfCaps().Has(auth.FieldPublish) {
		t.Error("OwnerSelfCaps should NOT include FieldPublish (publish needs maintainer+)")
	}
}

func TestAdvancedDerivativeCaps_SuperAdminOnly(t *testing.T) {
	// SHACL/SPARQL/Arches/ExportGraph are company-private: no project role
	// grants them; only super_admin passes (via the Can() bypass). Escalation
	// = adding one of these to a role set — this test flags that on purpose.
	caps := []auth.Capability{
		auth.DerivativeSHACLRead, auth.DerivativeSPARQLRead,
		auth.DerivativeArchesRead, auth.DerivativeExportGraphRead,
		auth.DerivativeSnapshotRead, auth.DerivativeASCIITreeRead,
	}
	for _, role := range []string{"viewer", "contributor", "maintainer", "owner"} {
		set := auth.RoleForScope("project", role)
		for _, c := range caps {
			if set.Has(c) {
				t.Errorf("project role %q unexpectedly grants %q (should be super_admin only)", role, c)
			}
		}
	}
	sa := &auth.AuthSnapshot{IsSuperAdmin: true}
	res := auth.Resource{ScopeType: "project", ID: "X", Visibility: "private"}
	for _, c := range caps {
		if !sa.Can(c, res, nil) {
			t.Errorf("super_admin should pass %q", c)
		}
	}
}

func TestCanDerivative_FailClosed(t *testing.T) {
	r := auth.Resource{ScopeType: "project", ID: "X", Visibility: "public"}

	// A non-privileged snapshot: open formats pass, gated + unknown fail.
	reader := &auth.AuthSnapshot{}
	openOK := []string{"turtle", "jsonld", "mermaid", "cytoscape", "x3ml", "x3ml-b"}
	for _, f := range openOK {
		if !reader.CanDerivative(f, r) {
			t.Errorf("open format %q should be allowed for any reader", f)
		}
	}
	// gated (has a cap) + unlisted (fail-closed default) both deny.
	denied := []string{"shacl", "sparql", "exportgraph", "arches", "snapshot", "ascii-tree", "researchspace", "brand-new-format"}
	for _, f := range denied {
		if reader.CanDerivative(f, r) {
			t.Errorf("gated/unknown format %q should be denied for a non-super_admin reader", f)
		}
	}

	// super_admin passes everything, including an unlisted format.
	sa := &auth.AuthSnapshot{IsSuperAdmin: true}
	for _, f := range append(append([]string{}, openOK...), denied...) {
		if !sa.CanDerivative(f, r) {
			t.Errorf("super_admin should pass %q", f)
		}
	}
}
