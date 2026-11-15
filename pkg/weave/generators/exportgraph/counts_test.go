package exportgraph_test

import (
	"strings"
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/generators"
	"github.com/pletka-io/pletka/pkg/weave/generators/exportgraph"
	"github.com/pletka-io/pletka/pkg/weave/views"
)

func intp(n int) *int { return &n }

func qel(kind, q string) domain.PathElement {
	prefix, local, _ := strings.Cut(q, ":")
	return domain.PathElement{Type: kind, URI: q, Prefix: prefix, LocalName: local}
}

func tprop(q string) generators.PathNode {
	return generators.PathNode{Element: qel("property", q)}
}

func tclass(q, chain string) generators.PathNode {
	e := qel("class", q)
	e.PathNode = chain
	return generators.PathNode{Element: e}
}

func tlit(q string) generators.PathNode {
	return generators.PathNode{Element: qel("literal", q)}
}

func tfield(sem string, min int, required bool, max *int, path ...generators.PathNode) generators.FieldNode {
	return generators.FieldNode{
		Field:   domain.ResolvedField{ID: sem, SemanticID: sem, MinOccurs: min, IsRequired: required, MaxOccurs: max},
		Path:    path,
		Address: sem,
	}
}

func buildCounts(root string, fields ...generators.FieldNode) *exportgraph.Graph {
	return exportgraph.Build(&generators.Snapshot{
		Fields: fields,
		Tree:   &views.Tree{Root: views.TreeNode{Scope: ptr(qel("class", root))}},
	})
}

func buildCountsSnap(snap *generators.Snapshot) *exportgraph.Graph {
	snap.Tree.Root.Scope = ptr(qel("class", "crm:E7_Activity"))
	return exportgraph.Build(snap)
}

func ptr[T any](v T) *T { return &v }

func nodeOf(t *testing.T, g *exportgraph.Graph, key string) *exportgraph.ResourceNode {
	t.Helper()
	for _, n := range g.Nodes {
		if n.Key == key {
			return n
		}
	}
	t.Fatalf("no node %q", key)
	return nil
}

func rootOf(t *testing.T, g *exportgraph.Graph) *exportgraph.ResourceNode {
	return nodeOf(t, g, g.RootKey)
}

func rootEdge(t *testing.T, g *exportgraph.Graph, pred string) *exportgraph.ResourceEdge {
	t.Helper()
	root := rootOf(t, g)
	for i := range root.Edges {
		if root.Edges[i].Predicate.URI == pred {
			return &root.Edges[i]
		}
	}
	t.Fatalf("no root edge %q", pred)
	return nil
}

// nameSnapshot: a collection placed on the model (min 1, max 3), anchored at
// E33_E41 via P1; content min 1 max 1 under it.
func nameSnapshot() *generators.Snapshot {
	content := tfield("LAF.6", 1, false, intp(1),
		tprop("crm:P1_is_identified_by"), tclass("crm:E33_E41_Linguistic_Appellation", "c/name/p1/e33"),
		tprop("crm:P190_has_symbolic_content"), tlit("xsd:string"))
	content.CollectionID, content.CollectionSemanticID = "c1", "LAC.1"
	content.CollectionMinOccurs, content.CollectionMaxOccurs = 1, intp(3)
	anchor := qel("class", "crm:E33_E41_Linguistic_Appellation")
	anchor.PathNode = "c/name/p1/e33"
	return &generators.Snapshot{
		Fields: []generators.FieldNode{content},
		Tree: &views.Tree{Root: views.TreeNode{Children: []views.TreeNode{{
			Kind:     views.NodeCategory,
			Children: []views.TreeNode{{Kind: views.NodeCollection, SemanticID: "LAC.1", Anchor: &anchor}},
		}}}},
	}
}

func TestFieldCountsApplyPerParentNode(t *testing.T) {
	g := buildCountsSnap(nameSnapshot())
	root := rootOf(t, g)
	if len(root.Edges) != 1 {
		t.Fatalf("root edges = %d, want 1 (P1)", len(root.Edges))
	}
	name := nodeOf(t, g, root.Edges[0].TargetKey)
	if len(name.Literals) != 1 {
		t.Fatalf("name node literals: %+v", name.Literals)
	}
	c := name.Literals[0]
	if c.Min != 1 || c.Max == nil || *c.Max != 1 {
		t.Fatalf("content counts = %d..%v, want 1..1 on the name node", c.Min, c.Max)
	}
}

func TestEntryEdgeCountsComeFromPlacement(t *testing.T) {
	entry := rootOf(t, buildCountsSnap(nameSnapshot())).Edges[0]
	if !entry.Entry || entry.Min != 1 || entry.Max == nil || *entry.Max != 3 {
		t.Fatalf("entry edge = %+v, want placement counts 1..3", entry)
	}
	// a field that ends at the anchor counts the same step: the stricter
	// of the placement and the field applies
	snap := nameSnapshot()
	snap.Fields[0].CollectionMinOccurs = 0
	typ := tfield("LAF.9", 0, true, intp(1), tprop("crm:P1_is_identified_by"), tclass("crm:E33_E41_Linguistic_Appellation", "c/name/p1/e33"))
	typ.CollectionID, typ.CollectionSemanticID = "c1", "LAC.1"
	typ.CollectionMaxOccurs = intp(3)
	snap.Fields = append(snap.Fields, typ)
	entry = rootOf(t, buildCountsSnap(snap)).Edges[0]
	if entry.Min != 1 || entry.Max == nil || *entry.Max != 1 || len(entry.Endings) != 1 {
		t.Fatalf("entry edge with a field at the anchor = %+v, want 1..1", entry)
	}
}

func TestFieldsMergeOnPathNode(t *testing.T) {
	a := tfield("F.1", 0, false, nil, tprop("crm:P4_has_time-span"), tclass("crm:E52_Time-Span", "d/p4/e52"), tprop("crm:P82a_begin_of_the_begin"), tlit("xsd:dateTime"))
	b := tfield("F.2", 0, false, nil, tprop("crm:P4_has_time-span"), tclass("crm:E52_Time-Span", "d/p4/e52"), tprop("crm:P82b_end_of_the_end"), tlit("xsd:dateTime"))
	c := tfield("F.3", 0, false, nil, tprop("crm:P4_has_time-span"), tclass("crm:E52_Time-Span", "other/p4/e52"), tprop("crm:P3_has_note"), tlit("xsd:string"))
	g := buildCounts("crm:E7_Activity", a, b, c)
	root := rootOf(t, g)
	if len(root.Edges) != 2 {
		t.Fatalf("root edges = %d, want 2 (one merged E52, one separate)", len(root.Edges))
	}
	if n := len(nodeOf(t, g, root.Edges[0].TargetKey).Literals); n != 2 {
		t.Fatalf("merged E52 has %d literals, want 2", n)
	}
}

func TestRequiredFieldMakesStepsAboveRequired(t *testing.T) {
	// Required with Min 0 counts as Min 1
	label := tfield("F.1", 0, true, nil, tprop("crm:P4_has_time-span"), tclass("crm:E52_Time-Span", "d/p4/e52"), tprop("rdfs:label"), tlit("xsd:string"))
	g := buildCounts("crm:E7_Activity", label)
	if got := rootOf(t, g).Edges[0].Min; got != 1 {
		t.Fatalf("P4 step min = %d, want 1 (a required field below it)", got)
	}
	// but not across a collection entry: an optional collection with a
	// required field inside stays optional
	snap := nameSnapshot()
	snap.Fields[0].CollectionMinOccurs = 0
	if got := rootOf(t, buildCountsSnap(snap)).Edges[0].Min; got != 0 {
		t.Fatalf("entry min = %d, want 0", got)
	}
}

func TestStepsMatchAcrossCURIEAndFullIRI(t *testing.T) {
	const crm = "http://www.cidoc-crm.org/cidoc-crm/"
	fullIRI := func(pn generators.PathNode) generators.PathNode {
		pn.Element.URI = crm + pn.Element.LocalName
		return pn
	}
	a := tfield("F.1", 0, false, nil, tprop("crm:P4_has_time-span"), tclass("crm:E52_Time-Span", "d/p4/e52"), tprop("crm:P82a_begin_of_the_begin"), tlit("xsd:dateTime"))
	b := tfield("F.2", 0, false, nil, fullIRI(tprop("crm:P4_has_time-span")), fullIRI(tclass("crm:E52_Time-Span", "d/p4/e52")), tprop("crm:P82b_end_of_the_end"), tlit("xsd:dateTime"))
	g := buildCounts("crm:E7_Activity", a, b)
	root := rootOf(t, g)
	if len(root.Edges) != 1 || len(nodeOf(t, g, root.Edges[0].TargetKey).Literals) != 2 {
		t.Fatalf("CURIE and full-IRI steps did not merge: %d root edges", len(root.Edges))
	}
}

// two fields ending on the same class node both keep their constraints
func TestFieldsEndingOnOneNodeKeepTheirOwnCounts(t *testing.T) {
	g := buildCounts("crm:E7_Activity",
		tfield("F.1", 1, false, intp(1), tprop("crm:P1_is_identified_by"), tclass("crm:E41_Appellation", "d/p1/e41")),
		tfield("F.2", 0, false, nil, tprop("crm:P1_is_identified_by"), tclass("crm:E41_Appellation", "d/p1/e41")),
	)
	e := rootEdge(t, g, "crm:P1_is_identified_by")
	if len(rootOf(t, g).Edges) != 1 {
		t.Fatalf("want one shared edge, got %d", len(rootOf(t, g).Edges))
	}
	if len(e.Endings) != 2 || e.Endings[0].Min != 1 || *e.Endings[0].Max != 1 || e.Endings[1].Min != 0 || e.Endings[1].Max != nil {
		t.Fatalf("endings = %+v", e.Endings)
	}
	if e.Endings[0].FieldID != "F.1" || e.Endings[1].FieldID != "F.2" {
		t.Fatalf("endings out of field order: %+v", e.Endings)
	}
	if e.Min != 1 || e.Max == nil || *e.Max != 1 {
		t.Fatalf("edge counts = %d/%v, want the first ending's 1/1", e.Min, e.Max)
	}
}

// R1: a path ending on a property with no leaf element still binds a literal
func TestPathEndingOnPropertyKeepsLiteralWithCounts(t *testing.T) {
	g := buildCounts("crm:E7_Activity",
		tfield("F.1", 0, true, intp(2), tprop("crm:P3_has_note")),
	)
	lits := rootOf(t, g).Literals
	if len(lits) != 1 || lits[0].Predicate.URI != "crm:P3_has_note" || lits[0].End.URI != "" {
		t.Fatalf("literals = %+v", lits)
	}
	if lits[0].Min != 1 || lits[0].Max == nil || *lits[0].Max != 2 {
		t.Fatalf("counts = %d/%v, want 1/2", lits[0].Min, lits[0].Max)
	}
}

// the node Key stays the generated PathNodeID when the element has one,
// while identity follows the PathNode chain
func TestNodeKeyIsPathNodeIDWhenPresent(t *testing.T) {
	cls := tclass("crm:E52_Time-Span", "d/p4/e52")
	cls.Element.PathNodeID = "E52_1"
	g := buildCounts("crm:E7_Activity",
		tfield("F.1", 0, false, nil, tprop("crm:P4_has_time-span"), cls, tprop("crm:P3_has_note"), tlit("xsd:string")),
	)
	if got := rootOf(t, g).Edges[0].TargetKey; got != "E52_1" {
		t.Fatalf("target key = %q, want E52_1", got)
	}
}

// a field with no collection that ends on an anchor step overwrites the
// step's counts with its own, as the reference does
func TestFieldWithoutCollectionEndingOnAnchorOverwritesCounts(t *testing.T) {
	snap := nameSnapshot() // placement 1..3
	typ := tfield("LAF.9", 0, false, intp(2), tprop("crm:P1_is_identified_by"), tclass("crm:E33_E41_Linguistic_Appellation", "c/name/p1/e33"))
	snap.Fields = append(snap.Fields, typ)
	entry := rootOf(t, buildCountsSnap(snap)).Edges[0]
	if entry.Min != 0 || entry.Max == nil || *entry.Max != 2 {
		t.Fatalf("counts = %d/%v, want the field's 0/2", entry.Min, entry.Max)
	}
}
