package auth

import (
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
)

func TestResolveEffectiveVersion(t *testing.T) {
	publicProject := &domain.Project{Entity: domain.Entity{ID: "P"}, OwnerID: "P", Visibility: "public"}
	internalProject := &domain.Project{Entity: domain.Entity{ID: "P"}, OwnerID: "P", Visibility: "internal"}
	privateProject := &domain.Project{Entity: domain.Entity{ID: "P"}, OwnerID: "P", Visibility: "private"}

	ownerSnap := &AuthSnapshot{
		ActorID:         "u1",
		OwnedProjectIDs: map[string]struct{}{"P": {}},
	}
	anonymousSnap := &AuthSnapshot{IsAnonymous: true}
	viewerSnap := &AuthSnapshot{
		ActorID: "u2",
		Roles:   map[string]string{"project:P": "viewer"},
	}
	memberSnap := &AuthSnapshot{
		ActorID: "u3",
		Roles:   map[string]string{"project:P": "contributor"},
	}

	tests := []struct {
		name            string
		snap            *AuthSnapshot
		project         *domain.Project
		explicitVersion string
		latestRelease   string
		want            string
	}{
		{
			name:            "explicit version wins regardless of snapshot",
			snap:            anonymousSnap,
			project:         publicProject,
			explicitVersion: "7.1.3",
			latestRelease:   "1.2.0",
			want:            "7.1.3",
		},
		{
			name:    "public editor sees working state",
			snap:    ownerSnap,
			project: publicProject,
			want:    "",
		},
		{
			name:          "public anonymous falls back to latest release",
			snap:          anonymousSnap,
			project:       publicProject,
			latestRelease: "1.2.0",
			want:          "1.2.0",
		},
		{
			name:          "public non-editor member falls back to latest release",
			snap:          viewerSnap,
			project:       publicProject,
			latestRelease: "1.2.0",
			want:          "1.2.0",
		},
		{
			name:          "public anonymous with no release falls back to hot",
			snap:          anonymousSnap,
			project:       publicProject,
			latestRelease: "",
			want:          "",
		},
		{
			name:    "internal member sees hot",
			snap:    memberSnap,
			project: internalProject,
			want:    "",
		},
		{
			name:    "private member sees hot",
			snap:    memberSnap,
			project: privateProject,
			want:    "",
		},
		{
			name:    "nil project is defensive no-op",
			snap:    ownerSnap,
			project: nil,
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveEffectiveVersion(tt.snap, tt.project, tt.explicitVersion, tt.latestRelease)
			if got != tt.want {
				t.Errorf("ResolveEffectiveVersion() = %q, want %q", got, tt.want)
			}
		})
	}
}
