package exportgraph_test

import (
	"sort"
	"strings"
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/generators"
	"github.com/pletka-io/pletka/pkg/weave/generators/exportgraph"
)

func anchorSnapshot(t *testing.T, modelSem string, placements int) *generators.Snapshot {
	t.Helper()
	prefix := []domain.PathElement{qel("property", "crm:P1_is_identified_by"), qel("class", "crm:E33_E41_Linguistic_Appellation")}
	var cats []domain.CategoryGroup
	for i := 1; i <= placements; i++ {
		cats = append(cats, domain.CategoryGroup{
			ID: "cat" + string(rune('0'+i)), Position: i,
			Collections: []domain.CollectionGroup{{
				ID: "c1", Position: 1,
				SharedPathPrefix: prefix,
				Placement:        &domain.CollectionPlacement{MinOccurs: 1},
				Fields: []domain.ResolvedField{{ID: "f", SemanticID: "PF.1", PathElements: append(append([]domain.PathElement(nil), prefix...),
					qel("property", "crm:P190_has_symbolic_content"), qel("literal", "xsd:string"))}},
			}},
		})
	}
	return generators.BuildModelSnapshot(generators.ModelSnapshotInput{
		Project: domain.Project{Entity: domain.Entity{ID: "P"}, Namespace: "https://example.org/P/"},
		Model:   domain.Model{Entity: domain.Entity{ID: "m", SemanticID: modelSem}, OntologyScope: qel("class", "crm:E7_Activity")},
		View:    domain.ModelView{ModelID: "m", ProjectID: "P", Categories: cats},
		Collections: map[string]*domain.Collection{"c1": {
			Entity: domain.Entity{ID: "c1", SemanticID: "LAC.1", UIName: domain.Translations{"en": "Name"}},
		}},
	})
}

func anchorNodes(g *exportgraph.Graph) []*exportgraph.ResourceNode {
	var out []*exportgraph.ResourceNode
	for _, n := range g.Nodes {
		if n.Collection != nil {
			out = append(out, n)
		}
	}
	return out
}

// the tree's collection anchors carry the same PathNode chain as the
// members' path elements, so the builder can find them
func TestCoreAnchorsMatchMemberPathNodes(t *testing.T) {
	g := exportgraph.Build(anchorSnapshot(t, "M.1", 1))
	root := rootOf(t, g)
	if len(root.Edges) != 1 || !root.Edges[0].Entry {
		t.Fatalf("entry not found from the core tree's anchor: %+v", root.Edges)
	}
	nodes := anchorNodes(g)
	if len(nodes) != 1 {
		t.Fatalf("anchor nodes = %d, want 1", len(nodes))
	}
	c := nodes[0].Collection
	if c.SemanticID != "LAC.1" || c.Address != "M.1/LAC.1" || c.Label["en"] != "Name" {
		t.Fatalf("collection anchor = %+v", c)
	}
}

// The same collection placed in two categories anchors two nodes with two
// addresses; the second carries its node id.
func TestSameCollectionTwiceGetsTwoAddresses(t *testing.T) {
	g := exportgraph.Build(anchorSnapshot(t, "M.1", 2))
	nodes := anchorNodes(g)
	addrs := make([]string, 0, len(nodes))
	for _, n := range nodes {
		addrs = append(addrs, n.Collection.Address)
	}
	sort.Strings(addrs)
	if len(addrs) != 2 || addrs[0] != "M.1/LAC.1" || !strings.HasPrefix(addrs[1], "M.1/LAC.1/") {
		t.Fatalf("addresses = %v", addrs)
	}
}
