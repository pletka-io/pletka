package exportgraph_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/generators"
	"github.com/pletka-io/pletka/pkg/weave/generators/exportgraph"
)

func testNS() generators.NamespaceSet { return generators.NamespaceSet{ByPrefix: map[string]string{}} }

// typedName is a Name node with one set-value field per entry of sets
// (value, required) at the same P2 -> E55 step.
func typedName(sets ...[2]any) *exportgraph.Graph {
	var fields []generators.FieldNode
	for i, s := range sets {
		f := tfield("LAF."+string(rune('1'+i)), 0, s[1].(bool), nil, tprop("crm:P2_has_type"), tclass("crm:E55_Type", ""))
		f.Field.SetValue = s[0].(string)
		fields = append(fields, f)
	}
	return exportgraph.Build(&generators.Snapshot{
		Model:  &domain.Model{Entity: domain.Entity{SemanticID: "M.1"}, OntologyScope: qel("class", "crm:E33_E41_Linguistic_Appellation")},
		Fields: fields,
	})
}

func idOf(t *testing.T, g *exportgraph.Graph) exportgraph.Identity {
	t.Helper()
	return exportgraph.NodeIdentity(rootOf(t, g), testNS())
}

func TestIdentityTakesRequiredOneStepSetValues(t *testing.T) {
	id := idOf(t, typedName([2]any{"http://vocab.getty.edu/aat/300404670", true}))
	want := exportgraph.Identity{
		Class:  "crm:E33_E41_Linguistic_Appellation",
		Values: map[string][]string{"crm:P2_has_type": {"<http://vocab.getty.edu/aat/300404670>"}},
	}
	if diff := cmp.Diff(want, id); diff != "" {
		t.Fatal(diff)
	}
	if got := idOf(t, typedName([2]any{"http://vocab.getty.edu/aat/300404670", false})); len(got.Values) != 0 {
		t.Fatalf("optional set value became a discriminator: %v", got.Values)
	}
}

func TestIdentityTakesRequiredLiteralSetValues(t *testing.T) {
	f := tfield("LAF.1", 1, false, nil, tprop("crm:P190_has_symbolic_content"), tlit("xsd:string"))
	f.Field.SetValue = "fixed"
	g := exportgraph.Build(&generators.Snapshot{
		Model:  &domain.Model{Entity: domain.Entity{SemanticID: "M.1"}, OntologyScope: qel("class", "crm:E33_E41_Linguistic_Appellation")},
		Fields: []generators.FieldNode{f},
	})
	if got := idOf(t, g).Values["crm:P190_has_symbolic_content"]; !cmp.Equal(got, []string{`"fixed"`}) {
		t.Fatalf("literal set value = %v", got)
	}
}

func TestIdentityKeepsEveryFixedValue(t *testing.T) {
	id := idOf(t, typedName([2]any{"http://vocab.getty.edu/aat/1", true}, [2]any{"http://vocab.getty.edu/aat/2", true}))
	want := []string{"<http://vocab.getty.edu/aat/1>", "<http://vocab.getty.edu/aat/2>"}
	if diff := cmp.Diff(want, id.Values["crm:P2_has_type"]); diff != "" {
		t.Fatal(diff)
	}
}

func TestIdentityResolvesEndingFieldWithoutID(t *testing.T) {
	a := tfield("", 0, false, nil, tprop("crm:P2_has_type"), tclass("crm:E55_Type", ""))
	b := tfield("", 0, true, nil, tprop("crm:P2_has_type"), tclass("crm:E55_Type", ""))
	b.Field.SetValue = "http://vocab.getty.edu/aat/9"
	g := exportgraph.Build(&generators.Snapshot{
		Model:  &domain.Model{Entity: domain.Entity{SemanticID: "M.1"}, OntologyScope: qel("class", "crm:E33_E41_Linguistic_Appellation")},
		Fields: []generators.FieldNode{a, b},
	})
	if got := idOf(t, g).Values["crm:P2_has_type"]; !cmp.Equal(got, []string{"<http://vocab.getty.edu/aat/9>"}) {
		t.Fatalf("second ending's set value lost: %v", got)
	}
}

func TestDistinguishable(t *testing.T) {
	primary := idOf(t, typedName([2]any{"http://vocab.getty.edu/aat/300404670", true}))
	alt := idOf(t, typedName([2]any{"http://vocab.getty.edu/aat/300404671", true}))
	plain := idOf(t, typedName([2]any{"", false}))
	other := exportgraph.Identity{Class: "crm:E42_Identifier"}
	both := exportgraph.Identity{Class: primary.Class, Values: map[string][]string{"crm:P2_has_type": {"<http://vocab.getty.edu/aat/300404670>", "<http://vocab.getty.edu/aat/300404671>"}}}
	for _, tt := range []struct {
		name string
		a, b exportgraph.Identity
		want bool
	}{
		{"different fixed type", primary, alt, true},
		{"plain matches primary's values too", primary, plain, false},
		{"same identity", plain, plain, false},
		{"different classes can overlap through subclassing", plain, other, false},
		{"a value carrying both types matches both", both, primary, false},
		{"fixed values themselves differ", exportgraph.Identity{Self: "<a>"}, exportgraph.Identity{Self: "<b>"}, true},
	} {
		if got := exportgraph.Distinguishable(tt.a, tt.b); got != tt.want {
			t.Errorf("%s: Distinguishable = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestWithLineageMakesUnrelatedClassesDistinguishable(t *testing.T) {
	a := exportgraph.Identity{Class: "crm:E21_Person"}
	b := exportgraph.Identity{Class: "crm:E42_Identifier"}
	anc := map[string][]string{"crm:E21_Person": {"crm:E39_Actor"}, "crm:E42_Identifier": {"crm:E41_Appellation"}}
	if exportgraph.Distinguishable(a, b) {
		t.Fatal("classes alone distinguished without lineage")
	}
	if !exportgraph.Distinguishable(a.WithLineage(anc), b.WithLineage(anc)) {
		t.Fatal("unrelated classes not distinguished with lineage")
	}
	if got := a.WithLineage(map[string][]string{}); got.Lineage != nil {
		t.Fatal("lineage set although an ancestor is unknown")
	}
}
