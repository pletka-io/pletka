package auth

import (
	"context"
	"testing"
)

func TestCheckProjectReadUsesExplicitCurrentResource(t *testing.T) {
	for _, tc := range []struct {
		name     string
		snapshot *AuthSnapshot
		resource Resource
		allow    bool
	}{
		{"public anonymous", nil, Resource{ScopeType: "project", ID: "p", Visibility: "public"}, true},
		{"private anonymous", nil, Resource{ScopeType: "project", ID: "p", Visibility: "private"}, false},
		{"private member", &AuthSnapshot{Roles: map[string]string{"project:p": "viewer"}}, Resource{ScopeType: "project", ID: "p", Visibility: "private"}, true},
		{"wrong member", &AuthSnapshot{Roles: map[string]string{"project:other": "viewer"}}, Resource{ScopeType: "project", ID: "p", Visibility: "private"}, false},
		{"empty even for admin", &AuthSnapshot{IsSuperAdmin: true}, Resource{}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := CheckProjectRead(WithSnapshot(context.Background(), tc.snapshot), tc.resource)
			if (err == nil) != tc.allow {
				t.Fatalf("access=%v err=%v", tc.allow, err)
			}
		})
	}
}
