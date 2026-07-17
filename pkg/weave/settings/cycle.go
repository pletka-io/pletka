package settings

import (
	"context"
	"fmt"
)

// parentResolver is the narrow interface used by createsParentCycle so it
// can be unit-tested with a fake implementation. The slice's
// pkg/domain.WeaveProjectStore satisfies this interface via its
// GetParentID method.
type parentResolver interface {
	GetParentID(ctx context.Context, projectID string) (*string, error)
}

// createsParentCycle walks up from proposedParent via GetParentID and
// returns true when projectID is found in the chain within bounded
// depth (10 levels). An existing cycle upstream of the proposed parent
// halts the walk but does not count as "introduced by this update".
func createsParentCycle(ctx context.Context, ps parentResolver, projectID, proposedParent string) (bool, error) {
	current := proposedParent
	visited := map[string]bool{}
	for depth := 0; depth < 10 && current != ""; depth++ {
		if current == projectID {
			return true, nil
		}
		if visited[current] {
			// Existing cycle upstream — halt walk; not introduced by this update.
			return false, nil
		}
		visited[current] = true
		parent, err := ps.GetParentID(ctx, current)
		if err != nil {
			return false, fmt.Errorf("get parent of %s: %w", current, err)
		}
		if parent == nil {
			return false, nil
		}
		current = *parent
	}
	return false, nil
}
