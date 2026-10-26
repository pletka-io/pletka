//go:build integration

package collection

import (
	"context"
	"sort"
	"testing"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/weave/apierror"
)

// TestReleaseViewDoesNotLeakDraftCollections is the symptom test: a read
// pinned to a release must not see a collection, a rename or a scope class
// that arrived in the working state after the release was cut. The release
// is seeded into the archive, then the live table is changed differently, so
// the two sides disagree and a leak shows as the live answer.
func TestReleaseViewDoesNotLeakDraftCollections(t *testing.T) {
	svc, pool := newServiceWithPool(t)
	ctx := context.Background()
	const project, version = "RVL1", "1.0.0"

	// The release carried coll-a (named "orig") and coll-b.
	seedArchivedNamed(t, pool, project, project+".a", "orig", version)
	seedArchivedNamed(t, pool, project, project+".b", "kept", version)
	// Working state after the cut: coll-a renamed, coll-b gone, coll-c new.
	seedLiveNamed(t, pool, project, project+".a", "edited-live")
	seedLiveNamed(t, pool, project, project+".c", "after-release")

	names := func(scope auth.ReadScope) []string {
		t.Helper()
		got, n, err := svc.List(ctx, scope, project)
		if err != nil {
			t.Fatalf("List(%v): %v", scope, err)
		}
		if int(n) != len(got) {
			t.Fatalf("List(%v): count %d disagrees with %d rows", scope, n, len(got))
		}
		var out []string
		for _, c := range got {
			out = append(out, c.SystemName)
		}
		sort.Strings(out)
		return out
	}
	equal := func(a, b []string) bool {
		if len(a) != len(b) {
			return false
		}
		for i := range a {
			if a[i] != b[i] {
				return false
			}
		}
		return true
	}

	if got, want := names(auth.Release(version)), []string{"kept", "orig"}; !equal(got, want) {
		t.Fatalf("release List = %v, want %v: the pinned view must carry the archive, not the live edits", got, want)
	}
	if got, want := names(auth.Draft()), []string{"after-release", "edited-live"}; !equal(got, want) {
		t.Fatalf("draft List = %v, want %v: a draft read must see the live edits", got, want)
	}

	// Get at the release: the renamed collection answers with its archived name,
	// the one created later does not exist, the deleted one still does.
	a, err := svc.Get(ctx, auth.Release(version), project, project+".a")
	if err != nil || a == nil || a.SystemName != "orig" {
		t.Fatalf("release Get(a) = %+v, %v; want the archived name \"orig\"", a, err)
	}
	if c, err := svc.Get(ctx, auth.Release(version), project, project+".c"); apierror.FromError(err).Code != apierror.CodeNotInRelease || c != nil {
		t.Fatalf("release Get(c) = %+v, %v; a collection created after the release must not resolve", c, err)
	}
	if b, err := svc.Get(ctx, auth.Release(version), project, project+".b"); err != nil || b == nil || b.SystemName != "kept" {
		t.Fatalf("release Get(b) = %+v, %v; a collection deleted after the release must still resolve", b, err)
	}
	live, err := svc.Get(ctx, auth.Draft(), project, project+".a")
	if err != nil || live == nil || live.SystemName != "edited-live" {
		t.Fatalf("draft Get(a) = %+v, %v; want the live name \"edited-live\"", live, err)
	}
}

// TestReleaseViewScopeClassesIgnoreLiveOnlyPaths pins the same symptom for the
// scope-class read the field picker uses: a class that exists only in the
// working state must not be offered at a release.
func TestReleaseViewScopeClassesIgnoreLiveOnlyPaths(t *testing.T) {
	svc, pool := newServiceWithPool(t)
	ctx := context.Background()
	const project, version = "RVL2", "1.0.0"

	seedArchivedCollectionWithFieldPath(t, pool, project, project+".a", version, "crm:E22_Man-Made_Object")
	seedLiveCollectionWithFieldPath(t, pool, project, project+".b", "crm:E53_Place")

	rel, err := svc.ScopeClasses(ctx, auth.Release(version), project)
	if err != nil {
		t.Fatalf("release ScopeClasses: %v", err)
	}
	if len(rel) != 1 || rel[0] != "crm:E22_Man-Made_Object" {
		t.Fatalf("release ScopeClasses = %v, want only the archived class", rel)
	}
	dr, err := svc.ScopeClasses(ctx, auth.Draft(), project)
	if err != nil {
		t.Fatalf("draft ScopeClasses: %v", err)
	}
	found := false
	for _, c := range dr {
		if c == "crm:E53_Place" {
			found = true
		}
	}
	if !found {
		t.Fatalf("draft ScopeClasses = %v, want it to include the live-only crm:E53_Place", dr)
	}
}
