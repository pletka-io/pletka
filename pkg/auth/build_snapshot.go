package auth

import (
	"context"
	"fmt"

	"github.com/pletka-io/pletka/pkg/domain"
)

// BuildSnapshot assembles an AuthSnapshot from the weave stores. For an
// empty actorID it returns an anonymous snapshot without issuing any DB
// queries.
func BuildSnapshot(ctx context.Context, ws domain.WeaveStore, actorID string) (*AuthSnapshot, error) {
	if actorID == "" {
		return &AuthSnapshot{
			IsAnonymous:     true,
			Roles:           map[string]string{},
			OwnedProjectIDs: map[string]struct{}{},
		}, nil
	}

	rows, err := ws.Memberships().SnapshotForActor(ctx, actorID)
	if err != nil {
		return nil, fmt.Errorf("snapshot: %w", err)
	}

	s := &AuthSnapshot{
		ActorID:         actorID,
		Roles:           make(map[string]string, len(rows)),
		OwnedProjectIDs: map[string]struct{}{},
	}
	for _, r := range rows {
		switch r.Kind {
		case "actor":
			// Actor-level role (from weave_actors.role). Flags super-admin
			// for the Can()/EffectiveRole bypass.
			if r.Role == "super_admin" {
				s.IsSuperAdmin = true
			}
		case "membership":
			s.Roles[r.ScopeType+":"+r.ScopeID] = r.Role
		case "owned":
			s.OwnedProjectIDs[r.ScopeID] = struct{}{}
		}
	}
	return s, nil
}
