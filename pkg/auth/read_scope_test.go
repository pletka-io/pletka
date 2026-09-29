package auth

import (
	"context"
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
)

func TestReadScope_ZeroValueIsInvalid(t *testing.T) {
	var zero ReadScope
	if zero.Valid() {
		t.Fatal("the zero ReadScope reports Valid() — a caller that forgets the scope must fail loudly, not read draft")
	}
	if Draft().Valid() != true || Release("0.1.0").Valid() != true {
		t.Fatal("Draft() and Release() must both be valid")
	}
}

func TestReadScope_DraftAndRelease(t *testing.T) {
	d := Draft()
	if d.IsRelease() || d.Version() != "" || d.String() != "draft" {
		t.Errorf("Draft() = {release:%v version:%q string:%q}, want {false \"\" \"draft\"}", d.IsRelease(), d.Version(), d.String())
	}
	r := Release("0.1.0")
	if !r.IsRelease() || r.Version() != "0.1.0" || r.String() != "release 0.1.0" {
		t.Errorf("Release(0.1.0) = {release:%v version:%q string:%q}, want {true \"0.1.0\" \"release 0.1.0\"}", r.IsRelease(), r.Version(), r.String())
	}
}

// Release("") is Draft(), not an invalid scope: callers build a scope from a
// version string that is empty for draft, and a silent invalid there would
// fail at the read instead of at the mistake.
func TestReadScope_ReleaseWithEmptyVersionIsDraft(t *testing.T) {
	if s := Release(""); s.IsRelease() || !s.Valid() {
		t.Errorf("Release(\"\") = {release:%v valid:%v}, want {false true}", s.IsRelease(), s.Valid())
	}
}

func TestResolveReadScope(t *testing.T) {
	pub := &domain.Project{Entity: domain.Entity{ID: "LA"}, Visibility: domain.VisibilityPublic}
	priv := &domain.Project{Entity: domain.Entity{ID: "LA"}, Visibility: domain.VisibilityPrivate}
	editor := snapshotCanEdit(t, "LA")
	viewer := snapshotReadOnly(t)

	cases := []struct {
		name     string
		snap     *AuthSnapshot
		project  *domain.Project
		explicit string
		latest   string
		want     ReadScope
	}{
		{"explicit version wins for an editor", editor, pub, "0.1.0", "0.2.0", Release("0.1.0")},
		{"explicit version wins for a viewer", viewer, pub, "0.1.0", "0.2.0", Release("0.1.0")},
		{"public editor sees the working state", editor, pub, "", "0.2.0", Draft()},
		{"public viewer sees the latest release", viewer, pub, "", "0.2.0", Release("0.2.0")},
		{"public viewer, no release, falls back to draft", viewer, pub, "", "", Draft()},
		{"private project reads hot for its members", viewer, priv, "", "0.2.0", Draft()},
		{"nil project is draft", viewer, nil, "", "0.2.0", Draft()},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ResolveReadScope(c.snap, c.project, c.explicit, c.latest)
			if got != c.want {
				t.Errorf("got %v, want %v", got, c.want)
			}
		})
	}
}

func TestReadScopeFromContext_AbsentIsInvalid(t *testing.T) {
	// Absent must NOT default to draft: a handler that forgot to install the
	// scope should fail at its first read, in development, rather than serve
	// the draft in production six weeks later.
	if s := ReadScopeFromContext(context.Background()); s.Valid() {
		t.Fatal("ReadScopeFromContext on a bare context reports Valid()")
	}
	ctx := WithReadScope(context.Background(), Release("1.2.3"))
	if got := ReadScopeFromContext(ctx); got != Release("1.2.3") {
		t.Errorf("round trip = %v, want release 1.2.3", got)
	}
}

//nolint:unparam // projectID kept for signature symmetry with real capability checks; this fixture only needs the super-admin bypass.
func snapshotCanEdit(t *testing.T, projectID string) *AuthSnapshot {
	t.Helper()
	// A super-admin passes every capability check including ProjectEdit,
	// which is all this test needs — it exercises ResolveReadScope's
	// branching, not the capability model itself.
	return &AuthSnapshot{IsSuperAdmin: true}
}

func snapshotReadOnly(t *testing.T) *AuthSnapshot {
	t.Helper()
	return &AuthSnapshot{}
}
