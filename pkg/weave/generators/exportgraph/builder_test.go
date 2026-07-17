package exportgraph_test

import (
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/generators"
	"github.com/pletka-io/pletka/pkg/weave/generators/exportgraph"
	"github.com/pletka-io/pletka/pkg/weave/views"
)

func TestBuildMergesSharedStructuralBranchesWithoutInstanceIDs(t *testing.T) {
	model := domain.Model{
		Entity: domain.Entity{
			ID:         "model-person",
			SemanticID: "LAM.9",
			SystemName: "person",
			UIName:     domain.Translations{"en": "Person"},
		},
		OntologyScope: class("E21_Person"),
	}
	birthCollection := domain.Collection{
		Entity: domain.Entity{
			ID:         "collection-birth",
			SemanticID: "LAC.20",
			SystemName: "birth_event",
			UIName:     domain.Translations{"en": "Birth event"},
		},
		OntologyScope: class("E67_Birth"),
	}
	birthPrefix := []domain.PathElement{class("E21_Person"), prop("P98i_was_born"), class("E67_Birth")}
	view := domain.ModelView{
		Categories: []domain.CategoryGroup{{
			ID:       "cat-existence",
			Name:     domain.Translations{"en": "Existence"},
			Position: 1,
			Collections: []domain.CollectionGroup{{
				ID:               birthCollection.ID,
				Name:             domain.Translations{"en": "Birth event"},
				Position:         1,
				SharedPathPrefix: birthPrefix,
				Fields: []domain.ResolvedField{
					resolvedField("field-birth-timespan", "LAF.196", "birth_timespan", appendPath(birthPrefix, prop("P4_has_time-span"), class("E52_Time-Span"))),
					resolvedField("field-birth-location", "LAF.192", "birth_location", appendPath(birthPrefix, prop("P7_took_place_at"), class("E53_Place"))),
				},
			}},
		}},
	}
	snap := generators.BuildModelSnapshot(generators.ModelSnapshotInput{
		Model:       model,
		View:        view,
		Collections: map[string]*domain.Collection{birthCollection.ID: &birthCollection},
		Options:     generators.Options{Lang: "en"},
	})

	graph := exportgraph.Build(snap)
	if graph.Nodes[0].Class.URI != "crm:E21_Person" {
		t.Fatalf("root class = %q, want target model scope crm:E21_Person", graph.Nodes[0].Class.URI)
	}
	birthNodes := nodesByClass(graph, "crm:E67_Birth")
	if len(birthNodes) != 1 {
		t.Fatalf("E67_Birth node count = %d, want 1", len(birthNodes))
	}
	birthNode := birthNodes[0]
	if birthNode.IdentitySource != exportgraph.IdentityStructural {
		t.Fatalf("E67_Birth identity source = %q, want structural", birthNode.IdentitySource)
	}
	if !hasEdgeToClass(graph, birthNode, "crm:E52_Time-Span") {
		t.Fatalf("birth node is missing P4_has_time-span/E52_Time-Span branch")
	}
	if !hasEdgeToClass(graph, birthNode, "crm:E53_Place") {
		t.Fatalf("birth node is missing P7_took_place_at/E53_Place branch")
	}

	category := groupByKind(graph, views.NodeCategory)
	if category == nil {
		t.Fatalf("category visual group was not preserved")
	}
	if category.BranchKey != graph.RootKey {
		t.Fatalf("category branch key = %q, want root %q", category.BranchKey, graph.RootKey)
	}

	collection := groupByKind(graph, views.NodeCollection)
	if collection == nil {
		t.Fatalf("collection visual group was not preserved")
	}
	if collection.BranchKey != birthNode.Key {
		t.Fatalf("collection branch key = %q, want birth node %q", collection.BranchKey, birthNode.Key)
	}
	if len(collection.FieldIDs) != 2 {
		t.Fatalf("collection field bindings = %d, want 2", len(collection.FieldIDs))
	}
	for _, binding := range graph.Fields {
		if len(binding.GroupKeys) != 2 {
			t.Fatalf("field %s group keys = %v, want category and collection", binding.ID, binding.GroupKeys)
		}
	}
}

// TestBuildMergesIdenticalStructuralBranches: two fields whose ontology
// paths are structurally identical resolve to a SINGLE export-graph
// node. The node identity is the generated path_node_id, which is
// path-derived — the legacy hand-coded instance_id no
// longer splits them. Genuine repeated placement (the same collection
// placed twice) will be re-split via the ADR-0002 placement identity.
func TestBuildMergesIdenticalStructuralBranches(t *testing.T) {
	model := domain.Model{
		Entity: domain.Entity{
			ID:         "model-person",
			SemanticID: "LAM.9",
			SystemName: "person",
		},
		OntologyScope: class("E21_Person"),
	}
	primaryName := class("E41_Appellation")
	primaryName.InstanceID = "primary_name"
	alternateName := class("E41_Appellation")
	alternateName.InstanceID = "alternate_name"

	view := domain.ModelView{
		Categories: []domain.CategoryGroup{{
			ID:   "cat-names",
			Name: domain.Translations{"en": "Names"},
			Collections: []domain.CollectionGroup{{
				ID: "__direct__",
				Fields: []domain.ResolvedField{
					resolvedField("field-primary-name", "LAF.1", "primary_name", []domain.PathElement{class("E21_Person"), prop("P1_is_identified_by"), primaryName}),
					resolvedField("field-alt-name", "LAF.2", "alternate_name", []domain.PathElement{class("E21_Person"), prop("P1_is_identified_by"), alternateName}),
				},
			}},
		}},
	}
	snap := generators.BuildModelSnapshot(generators.ModelSnapshotInput{
		Model:   model,
		View:    view,
		Options: generators.Options{Lang: "en"},
	})

	graph := exportgraph.Build(snap)
	nameNodes := nodesByClass(graph, "crm:E41_Appellation")
	if len(nameNodes) != 1 {
		t.Fatalf("E41_Appellation node count = %d, want 1 — identical structural paths merge", len(nameNodes))
	}
	if nameNodes[0].IdentitySource != exportgraph.IdentityStructural {
		t.Fatalf("merged node identity source = %q, want structural", nameNodes[0].IdentitySource)
	}
	// The node merges, but each field stays a distinct entry on it —
	// the fields differ in overrides/targets even when the path is shared.
	if got := len(nameNodes[0].Fields); got != 2 {
		t.Errorf("merged node field bindings = %d, want 2 (one entry per field)", got)
	}
}

func class(local string) domain.PathElement {
	return domain.PathElement{
		Type:      "class",
		URI:       "crm:" + local,
		Prefix:    "crm",
		LocalName: local,
		ClassCode: local[:3],
	}
}

func prop(local string) domain.PathElement {
	return domain.PathElement{
		Type:      "property",
		URI:       "crm:" + local,
		Prefix:    "crm",
		LocalName: local,
	}
}

func appendPath(prefix []domain.PathElement, rest ...domain.PathElement) []domain.PathElement {
	out := append([]domain.PathElement(nil), prefix...)
	return append(out, rest...)
}

func resolvedField(id, semanticID, systemName string, path []domain.PathElement) domain.ResolvedField {
	return domain.ResolvedField{
		ID:           id,
		SemanticID:   semanticID,
		SystemName:   systemName,
		DisplayName:  domain.Translations{"en": systemName},
		PathElements: path,
	}
}

func nodesByClass(graph *exportgraph.Graph, uri string) []*exportgraph.ResourceNode {
	var out []*exportgraph.ResourceNode
	for _, node := range graph.Nodes {
		if node.Class.URI == uri {
			out = append(out, node)
		}
	}
	return out
}

func hasEdgeToClass(graph *exportgraph.Graph, source *exportgraph.ResourceNode, uri string) bool {
	for _, edge := range source.Edges {
		for _, node := range graph.Nodes {
			if node.Key == edge.TargetKey && node.Class.URI == uri {
				return true
			}
		}
	}
	return false
}

func groupByKind(graph *exportgraph.Graph, kind views.NodeKind) *exportgraph.VisualGroup {
	for _, group := range graph.Groups {
		if group.Kind == kind {
			return group
		}
	}
	return nil
}
