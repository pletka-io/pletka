//go:build integration

package ontology_test

import (
	"context"
	"errors"
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/ids"
	weaveontology "github.com/pletka-io/pletka/pkg/weave/ontology"
)

// TestService_UpdateFamily_RejectsParentCycle is the F2 guard: setting a
// family's parent to itself or to one of its descendants must be rejected with
// ErrCycle (mapped to a 422), not silently persisted (which made both families
// non-root and vanish from /ontologies).
func TestService_UpdateFamily_RejectsParentCycle(t *testing.T) {
	ctx := context.Background()
	svc := weaveontology.NewService(weaveontology.NewPostgresStore(testPool(t)), nil, nil)

	mk := func(name string, parent *string) string {
		id := ids.GenerateULID()
		if _, err := svc.CreateFamily(ctx, weaveontology.CreateFamilyInput{
			ID:             id,
			Slug:           "f2-" + id,
			Name:           name,
			Description:    domain.Translations{"en": name},
			ParentFamilyID: parent,
		}); err != nil {
			t.Fatalf("create family %s: %v", name, err)
		}
		return id
	}
	upd := func(id, name string, parent *string) error {
		_, err := svc.UpdateFamily(ctx, id, weaveontology.UpdateFamilyInput{
			Slug:           "f2-" + id,
			Name:           name,
			Description:    domain.Translations{"en": name},
			ParentFamilyID: parent,
		})
		return err
	}

	a := mk("F2 A", nil)
	b := mk("F2 B", &a)
	c := mk("F2 C", &b) // chain: c -> b -> a

	// Self-parent is a cycle.
	if err := upd(a, "F2 A", &a); !errors.Is(err, weaveontology.ErrCycle) {
		t.Fatalf("self-parent: got %v; want ErrCycle", err)
	}
	// Making the root a child of its own descendant closes a cycle.
	if err := upd(a, "F2 A", &c); !errors.Is(err, weaveontology.ErrCycle) {
		t.Fatalf("ancestor cycle: got %v; want ErrCycle", err)
	}
	// A legitimate reparent (c directly under the root a) is allowed.
	if err := upd(c, "F2 C", &a); err != nil {
		t.Fatalf("valid reparent: %v", err)
	}
	// Making c a root (empty parent) is allowed.
	empty := ""
	if err := upd(c, "F2 C", &empty); err != nil {
		t.Fatalf("make root: %v", err)
	}
}
