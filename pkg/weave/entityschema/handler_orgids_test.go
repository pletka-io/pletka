package entityschema

import (
	"sort"
	"testing"

	"github.com/pletka-io/pletka/pkg/auth"
)

// TestProjectCreateOrgIDs covers projectCreateOrgIDs: it returns the org ids
// whose role grants auth.OrgProjectCreate (owner/admin) via the capability
// model — excluding roles that don't (member/viewer) and non-org keys — and
// returns the all=true sentinel for a super-admin so the caller enumerates
// every org (Redmine #3559).
func TestProjectCreateOrgIDs(t *testing.T) {
	t.Run("nil snapshot returns none", func(t *testing.T) {
		all, ids := projectCreateOrgIDs(nil)
		if all || ids != nil {
			t.Fatalf("got (all=%v, ids=%v), want (false, nil)", all, ids)
		}
	})

	t.Run("regular actor: only owner/admin org roles", func(t *testing.T) {
		snap := &auth.AuthSnapshot{
			Roles: map[string]string{
				"org:org-owner":      "owner",
				"org:org-admin":      "admin",
				"org:org-member":     "member",
				"org:org-viewer":     "viewer",
				"project:proj-owner": "owner",
			},
		}
		all, got := projectCreateOrgIDs(snap)
		if all {
			t.Fatalf("all = true, want false for a non super-admin")
		}
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
	})

	t.Run("super-admin returns the all sentinel", func(t *testing.T) {
		// A super-admin can create under ANY org. Even with no explicit org
		// membership rows (the #3559 case), they must get all=true so the
		// caller enumerates every org instead of the empty role-derived set.
		superSnap := &auth.AuthSnapshot{IsSuperAdmin: true}
		all, ids := projectCreateOrgIDs(superSnap)
		if !all || ids != nil {
			t.Fatalf("got (all=%v, ids=%v), want (true, nil)", all, ids)
		}
	})
}
