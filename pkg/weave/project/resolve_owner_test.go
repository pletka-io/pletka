package project

import (
	"context"
	"errors"
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
)

type fakeOrgResolver struct {
	bySlug map[string]*domain.Organization
	err    error
}

func (f fakeOrgResolver) GetBySlug(_ context.Context, slug string) (*domain.Organization, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.bySlug[slug], nil
}

// TestResolveOwnerID proves an owner sent as an org slug is mapped to the org's
// actor id, while a non-slug value (or a failed/absent lookup) is left as sent
// so the personal-workspace and bare-actor-id cases still work (#fk_wp_owner).
func TestResolveOwnerID(t *testing.T) {
	orgs := fakeOrgResolver{bySlug: map[string]*domain.Organization{
		"letterenhuis": {ID: "01ORGULID0000000000000000"},
	}}

	cases := []struct {
		name      string
		resolver  OrgResolver
		requested string
		want      string
	}{
		{"slug resolves to actor id", orgs, "letterenhuis", "01ORGULID0000000000000000"},
		{"unknown slug left as sent", orgs, "01ACTORULID000000000000000", "01ACTORULID000000000000000"},
		{"lookup error left as sent", fakeOrgResolver{err: errors.New("boom")}, "letterenhuis", "letterenhuis"},
		{"nil resolver left as sent", nil, "letterenhuis", "letterenhuis"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := &Handler{orgs: tc.resolver}
			if got := h.resolveOwnerID(context.Background(), tc.requested); got != tc.want {
				t.Fatalf("resolveOwnerID(%q) = %q, want %q", tc.requested, got, tc.want)
			}
		})
	}
}
