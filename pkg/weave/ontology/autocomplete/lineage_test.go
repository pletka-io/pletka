package autocomplete

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/pletka-io/pletka/pkg/domain"
)

// lineageFake holds classes per version and their subclass_of targets.
type lineageFake struct {
	classes map[string]map[string]string // version → qname → class id
	parents map[string][]string          // class id → parent qnames
}

func (f lineageFake) GetClassByQname(_ context.Context, versionID, qname string) (*domain.OntologyClass, error) {
	id, ok := f.classes[versionID][qname]
	if !ok {
		return nil, nil
	}
	c := &domain.OntologyClass{}
	c.ID = id
	return c, nil
}

func (f lineageFake) ListRelationsForSourceByType(_ context.Context, sourceID, _, _ string) ([]*domain.OntologyRelation, error) {
	out := make([]*domain.OntologyRelation, 0, len(f.parents[sourceID]))
	for _, p := range f.parents[sourceID] {
		out = append(out, &domain.OntologyRelation{TargetQname: p})
	}
	return out, nil
}

func TestClassLineageWalksEveryVersion(t *testing.T) {
	store := lineageFake{
		classes: map[string]map[string]string{
			"v1": {"crm:E33_E41": "a", "crm:E33": "b", "crm:E41": "c"},
			"v2": {"crm:E33_E41": "d", "x:Extra": "e"},
		},
		parents: map[string][]string{"a": {"crm:E33", "crm:E41"}, "d": {"x:Extra"}, "b": {"crm:E73"}},
	}
	got, err := ClassLineage(context.Background(), store, []string{"v1", "v2"}, "crm:E33_E41")
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff([]string{"crm:E33_E41", "crm:E33", "crm:E41", "x:Extra", "crm:E73"}, got); diff != "" {
		t.Fatal(diff)
	}
}
