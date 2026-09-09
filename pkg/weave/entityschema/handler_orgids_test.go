package entityschema

import (
	"sort"
	"testing"

	"github.com/pletka-io/pletka/pkg/auth"
)

// TestProjectCreateOrgIDs is the regression test for the L4 rewrite of
// projectCreateOrgIDs: it must return org ids where the snapshot's role
// grants auth.OrgProjectCreate (owner/admin) via the capability model, not
// a hand-rolled string comparison, and must exclude org roles that don't
// grant it (member/viewer) and non-org role keys.
func TestProjectCreateOrgIDs(t *testing.T) {
	t.Run("nil snapshot returns nil", func(t *testing.T) {
		if got := projectCreateOrgIDs(nil); got != nil {
			t.Fatalf("got %v, want nil", got)
		}
	})

	snap := &auth.AuthSnapshot{
		Roles: map[string]string{
			"org:org-owner":      "owner",
			"org:org-admin":      "admin",
			"org:org-member":     "member",
			"org:org-viewer":     "viewer",
			"project:proj-owner": "owner",
		},
	}

	got := projectCreateOrgIDs(snap)
	sort.Strings(got)

	want := []string{"org-admin", "org-owner"}
	if len(got) != len(want) {
		t.Fatalf("projectCreateOrgIDs() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("projectCreateOrgIDs() = %v, want %v", got, want)
		}
	}

	t.Run("super-admin org membership of any role is included", func(t *testing.T) {
		// AuthSnapshot.Can short-circuits true for IsSuperAdmin regardless
		// of the org's own role, so a super-admin holding only a "member"
		// row on an org must still see that org as a create target.
		superSnap := &auth.AuthSnapshot{
			IsSuperAdmin: true,
			Roles: map[string]string{
				"org:X": "member",
			},
		}
		got := projectCreateOrgIDs(superSnap)
		if len(got) != 1 || got[0] != "X" {
			t.Fatalf("projectCreateOrgIDs() = %v, want [X]", got)
		}
	})
}
