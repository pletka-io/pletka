package settings

import (
	"context"
	"fmt"
	"testing"
)

// fakeParentResolver is a minimal in-memory implementation of parentResolver
// used to exercise createsParentCycle without spinning up a full WeaveStore.
type fakeParentResolver struct {
	parents map[string]*string // projectID -> parentID
}

func (f *fakeParentResolver) GetParentID(_ context.Context, id string) (*string, error) {
	return f.parents[id], nil
}

func strptr(s string) *string { return &s }

// longChain returns a parents map with n levels: L0->L1->...->L(n-1)->nil.
// Used to verify that bounded depth cuts off walks before misbehaviour.
func longChain(n int) map[string]*string {
	m := map[string]*string{}
	for i := 0; i < n-1; i++ {
		nxt := fmt.Sprintf("L%d", i+1)
		m[fmt.Sprintf("L%d", i)] = &nxt
	}
	m[fmt.Sprintf("L%d", n-1)] = nil
	return m
}

func TestCreatesParentCycle(t *testing.T) {
	cases := []struct {
		name           string
		parents        map[string]*string
		projectID      string
		proposedParent string
		want           bool
	}{
		{
			name:           "no ancestors",
			parents:        map[string]*string{"P": nil},
			projectID:      "C",
			proposedParent: "P",
			want:           false,
		},
		{
			name:           "self-link (already blocked upstream, but check)",
			parents:        map[string]*string{"C": nil},
			projectID:      "C",
			proposedParent: "C",
			want:           true,
		},
		{
			name:           "one-hop cycle: C->P, propose P=C",
			parents:        map[string]*string{"P": strptr("C"), "C": nil},
			projectID:      "C",
			proposedParent: "P",
			want:           true,
		},
		{
			name:           "two-hop cycle: GP->P->C, propose GP",
			parents:        map[string]*string{"GP": strptr("P"), "P": strptr("C"), "C": nil},
			projectID:      "C",
			proposedParent: "GP",
			want:           true,
		},
		{
			name:           "three-hop no cycle",
			parents:        map[string]*string{"A": strptr("B"), "B": strptr("D"), "D": nil},
			projectID:      "C",
			proposedParent: "A",
			want:           false,
		},
		{
			name:           "bounded depth cuts off long chain",
			parents:        longChain(12),
			projectID:      "C",
			proposedParent: "L0",
			want:           false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := createsParentCycle(context.Background(), &fakeParentResolver{parents: c.parents}, c.projectID, c.proposedParent)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != c.want {
				t.Errorf("got %v want %v", got, c.want)
			}
		})
	}
}
