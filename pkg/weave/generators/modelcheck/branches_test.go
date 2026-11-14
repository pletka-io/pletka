package modelcheck

import (
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/generators"
)

// chained is fn with PathNode chains on its class elements.
func chained(sem, typ string, min int, set string, path ...domain.PathElement) generators.FieldNode {
	f := fn(sem, typ, min, nil, path...)
	chain := ""
	for i := range f.Path {
		chain += "/" + f.Path[i].Element.PrefixedName()
		if f.Path[i].Element.Type == "class" {
			f.Path[i].Element.PathNode = chain
		}
	}
	f.Field.SetValue = set
	return f
}

func c6(fs []Finding) []Finding {
	var out []Finding
	for _, f := range fs {
		if f.Code == "C6" {
			out = append(out, f)
		}
	}
	return out
}

var (
	p67  = el("property", "crm:P67i_is_referred_to_by")
	e33  = el("class", "crm:E33_Linguistic_Object")
	p190 = el("property", "crm:P190_has_symbolic_content")
	str  = el("literal", "xsd:string")
	p2   = el("property", "crm:P2_has_type")
	e55  = el("class", "crm:E55_Type")
)

// statement and reference: two P67i → E33 branches reached through
// different placements (different PathNode chains)
func twoBranches(typesA, typesB []string) *generators.Snapshot {
	snap := snapOf()
	add := func(prefix, sem string, types []string) {
		f := chained(sem, "String", 0, "", p67, e33, p190, str)
		for i := range f.Path {
			f.Path[i].Element.PathNode = prefix + f.Path[i].Element.PathNode
		}
		snap.Fields = append(snap.Fields, f)
		for i, v := range types {
			t := chained(sem+"t"+string(rune('a'+i)), "Concept", 1, v, p67, e33, p2, e55)
			for j := range t.Path {
				t.Path[j].Element.PathNode = prefix + t.Path[j].Element.PathNode
			}
			snap.Fields = append(snap.Fields, t)
		}
	}
	add("a", "PF.1", typesA)
	add("b", "PF.2", typesB)
	return snap
}

func TestC6SameClassBranches(t *testing.T) {
	got := c6(Model(twoBranches(nil, nil)))
	if len(got) != 1 || got[0].Model != "PM.1" {
		t.Fatalf("%+v", got)
	}
}

func TestC6ApartByFixedType(t *testing.T) {
	if got := c6(Model(twoBranches([]string{"aat:1"}, []string{"aat:2"}))); len(got) != 0 {
		t.Fatalf("%+v", got)
	}
}

func TestC6SharedValueIsNotApart(t *testing.T) {
	if got := c6(Model(twoBranches([]string{"aat:1", "aat:2"}, []string{"aat:1"}))); len(got) != 1 {
		t.Fatalf("%+v", got)
	}
}

func TestC6DifferentClassesAreNotReported(t *testing.T) {
	snap := snapOf(
		chained("PF.1", "Model", 0, "", p67, e33),
		chained("PF.2", "Model", 0, "", p67, el("class", "crm:E31_Document")),
	)
	if got := c6(Model(snap)); len(got) != 0 {
		t.Fatalf("%+v", got)
	}
}

func TestC6InsideReferencedCollection(t *testing.T) {
	member := func(sem string, path ...domain.PathElement) domain.ResolvedField {
		return domain.ResolvedField{SemanticID: sem, ExpectedValueType: "Model", PathElements: path}
	}
	snap := snapOf()
	snap.Referenced = map[string]generators.ReferencedCollection{"c": {
		Collection: &domain.Collection{Entity: domain.Entity{ID: "c", SemanticID: "PC.3"}},
		Fields: []domain.ResolvedField{
			member("PF.5", p67, e33), // two fields end on the same E33 node
			member("PF.6", p67, e33),
		},
	}}
	got := c6(Model(snap))
	if len(got) != 1 || got[0].Collection != "PC.3" {
		t.Fatalf("%+v", got)
	}
}
