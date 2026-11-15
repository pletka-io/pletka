package exportgraph_test

import (
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/generators"
	"github.com/pletka-io/pletka/pkg/weave/generators/exportgraph"
)

// nameCollection is a referenced collection as core loads it: members with
// their full paths, the graft prefix P1 -> E33_E41.
func nameCollection() generators.ReferencedCollection {
	p1, e33 := qel("property", "crm:P1_is_identified_by"), qel("class", "crm:E33_E41_Linguistic_Appellation")
	member := func(sem string, min int, max *int, tail ...domain.PathElement) domain.ResolvedField {
		return domain.ResolvedField{ID: sem, SemanticID: sem, MinOccurs: min, MaxOccurs: max, PathElements: append([]domain.PathElement{p1, e33}, tail...)}
	}
	hidden := member("LAF.8", 0, nil, qel("property", "crm:P3_has_note"), qel("literal", "xsd:string"))
	hidden.IsHidden = true
	return generators.ReferencedCollection{
		Collection: &domain.Collection{Entity: domain.Entity{ID: "c1", SemanticID: "LAC.1"}},
		Fields: []domain.ResolvedField{
			member("LAF.6", 1, intp(1), qel("property", "crm:P190_has_symbolic_content"), qel("literal", "xsd:string")),
			member("LAF.7", 0, nil, qel("property", "crm:P72_has_language"), qel("class", "crm:E56_Language")),
			member("LAF.9", 0, nil, qel("property", "crm:P72_has_language"), qel("class", "crm:E56_Language"), qel("property", "rdfs:label"), qel("literal", "xsd:string")),
			hidden,
		},
		SharedPathPrefix: []domain.PathElement{p1, e33},
	}
}

func TestMemberTreeStripsPrefixAndMergesOnClass(t *testing.T) {
	g := exportgraph.BuildMembers(nameCollection(), "LAF.500", nil)
	root := rootOf(t, g)
	if root.Class.URI != "crm:E33_E41_Linguistic_Appellation" {
		t.Fatalf("anchor class = %q", root.Class.URI)
	}
	if len(root.Literals) != 1 || len(root.Edges) != 1 {
		t.Fatalf("literals %d edges %d, want 1 and 1 (content; one merged P72 -> E56; hidden member dropped)", len(root.Literals), len(root.Edges))
	}
	c := root.Literals[0]
	if c.Field.Source.Address != "LAF.500/LAF.6" || c.Min != 1 {
		t.Fatalf("content literal: %+v addr %q", c, c.Field.Source.Address)
	}
	lang := nodeOf(t, g, root.Edges[0].TargetKey)
	if len(lang.Literals) != 1 {
		t.Fatalf("language node did not merge: %+v", lang)
	}
	if len(root.Edges[0].Endings) != 1 || root.Edges[0].Endings[0].FieldID != "LAF.7" {
		t.Fatalf("P72 endings: %+v", root.Edges[0].Endings)
	}
}

func TestMemberTreeAppliesGraftOverrides(t *testing.T) {
	grafts := map[string]domain.GraftOverride{
		"LAF.500/LAF.6": {MaxOccursSet: true, MaxOccurs: intp(2)},
		"LAF.500/LAF.7": {IsHidden: true},
	}
	g := exportgraph.BuildMembers(nameCollection(), "LAF.500", grafts)
	root := rootOf(t, g)
	if got := root.Literals[0].Max; got == nil || *got != 2 {
		t.Fatalf("content max = %v, want 2 from the graft override", got)
	}
	if len(root.Edges[0].Endings) != 0 {
		t.Fatalf("hidden LAF.7 still ends the language step: %+v", root.Edges[0].Endings)
	}
}

func TestMemberAnchorFallsBackToCollectionScope(t *testing.T) {
	rc := nameCollection()
	rc.SharedPathPrefix = nil
	rc.Collection.OntologyScope = qel("class", "crm:E7_Activity")
	if got := rootOf(t, exportgraph.BuildMembers(rc, "X", nil)).Class.URI; got != "crm:E7_Activity" {
		t.Fatalf("anchor = %q", got)
	}
}
