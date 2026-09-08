package usermenu_test

import (
	"testing"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/weave/usermenu"
)

func TestBuild_AnonymousReturnsNil(t *testing.T) {
	if got := usermenu.Build(nil, "/anything"); got != nil {
		t.Errorf("Build(nil) = %v, want nil", got)
	}
}

func TestBuild_InactiveReturnsNil(t *testing.T) {
	p := &auth.Principal{ActorID: "x", Role: "contributor", IsActive: false}
	if got := usermenu.Build(p, "/profile"); got != nil {
		t.Errorf("Build(inactive) = %v, want nil", got)
	}
}

func TestBuild_ContributorOmitsAdmin(t *testing.T) {
	p := &auth.Principal{ActorID: "x", DisplayName: "Alice", Role: "contributor", IsActive: true}
	m := usermenu.Build(p, "/profile")
	if m == nil {
		t.Fatal("Build returned nil")
	}
	for _, it := range m.Items {
		if it.Href == "/admin" {
			t.Errorf("contributor menu must not contain /admin item")
		}
	}
}

func TestBuild_SuperAdminIncludesAdmin(t *testing.T) {
	p := &auth.Principal{ActorID: "x", DisplayName: "Bob", Role: "super_admin", IsActive: true}
	m := usermenu.Build(p, "/admin/users")
	if m == nil {
		t.Fatal("Build returned nil")
	}
	hasAdmin := false
	for _, it := range m.Items {
		if it.Href == "/admin" {
			hasAdmin = true
		}
	}
	if !hasAdmin {
		t.Error("super_admin menu must contain /admin item")
	}
}

func TestBuild_AdminRoleOmitsAdmin(t *testing.T) {
	p := &auth.Principal{ActorID: "x", DisplayName: "Carol", Role: "admin", IsActive: true}
	m := usermenu.Build(p, "/profile")
	if m == nil {
		t.Fatal("Build returned nil")
	}
	for _, it := range m.Items {
		if it.Href == "/admin" {
			t.Errorf("admin role menu must not contain /admin item")
		}
	}
}

func TestBuild_ActiveLongestPrefix(t *testing.T) {
	p := &auth.Principal{ActorID: "x", DisplayName: "Alice", Role: "super_admin", IsActive: true}

	cases := []struct {
		path     string
		wantHref string // expected Active item; empty means none
	}{
		{"/profile", "/profile"},                   // exact
		{"/profile/settings", "/profile/settings"}, // longer prefix beats /profile
		{"/orgs/new", "/orgs/new"},
		{"/admin", "/admin"},
		{"/admin/users", "/admin"}, // nested → /admin still wins
		{"/somewhere/else", ""},    // no match → no highlight
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			m := usermenu.Build(p, tc.path)
			var got string
			for _, it := range m.Items {
				if it.Active {
					if got != "" {
						t.Fatalf("multiple items active: %s and %s", got, it.Href)
					}
					got = it.Href
				}
			}
			if got != tc.wantHref {
				t.Errorf("active href = %q, want %q", got, tc.wantHref)
			}
		})
	}
}
