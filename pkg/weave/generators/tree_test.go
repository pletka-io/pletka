package generators

import (
	"strings"
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/views"
)

func TestBuildModelSnapshotPreservesModelViewTree(t *testing.T) {
	model := domain.Model{
		Entity: domain.Entity{
			ID:         "model-1",
			SemanticID: "LA.M.1",
			SystemName: "Person",
			UIName:     domain.Translations{"en": "Person"},
		},
		OntologyScope: pe("class", "crm", "E21_Person", 0),
	}
	collection := &domain.Collection{
		Entity: domain.Entity{
			ID:          "collection-1",
			SemanticID:  "LA.C.1",
			SystemName:  "Birth",
			Description: domain.Translations{"en": "Birth collection"},
		},
		OntologyScope: pe("class", "crm", "E67_Birth", 0),
	}
	view := domain.ModelView{
		ModelID:   model.ID,
		ProjectID: "LA",
		Categories: []domain.CategoryGroup{
			{
				ID:       "cat-identity",
				Name:     domain.Translations{"en": "Identity"},
				Position: 1,
				Collections: []domain.CollectionGroup{
					{
						ID:       directCollectionID,
						Name:     domain.Translations{"en": "Direct fields"},
						Position: 1,
						Fields: []domain.ResolvedField{
							field("field-name", "LA.F.1", "Name", 1, pe("property", "crm", "P1_is_identified_by", 0)),
						},
					},
					{
						ID:               collection.ID,
						Name:             domain.Translations{"en": "Birth"},
						Position:         2,
						SharedPathPrefix: []domain.PathElement{pe("property", "crm", "P98_brought_into_life", 0)},
						Fields: []domain.ResolvedField{
							field("field-date", "LA.F.2", "Birth date", 2, pe("property", "crm", "P98_brought_into_life", 0), pe("class", "crm", "E67_Birth", 1)),
						},
					},
				},
			},
		},
	}

	snapshot := BuildModelSnapshot(ModelSnapshotInput{
		Project:     domain.Project{Entity: domain.Entity{ID: "LA"}},
		Model:       model,
		View:        view,
		Collections: map[string]*domain.Collection{collection.ID: collection},
		Namespaces:  BuildNamespaceSet(domain.Project{Entity: domain.Entity{ID: "LA"}}, nil),
		Options:     Options{Lang: "en"},
	})

	if snapshot.RootKind != EntityModel {
		t.Fatalf("RootKind = %q", snapshot.RootKind)
	}
	if snapshot.Tree.Root.Kind != views.NodeModel {
		t.Fatalf("root kind = %q", snapshot.Tree.Root.Kind)
	}
	if got := snapshot.Tree.Root.Path; got != "la-m-1" {
		t.Fatalf("root relative path = %q", got)
	}
	if len(snapshot.Tree.Root.Children) != 2 {
		t.Fatalf("root children = %d", len(snapshot.Tree.Root.Children))
	}

	directField := snapshot.Tree.Root.Children[0]
	if directField.Kind != views.NodeField {
		t.Fatalf("direct node kind = %q, want field", directField.Kind)
	}

	category := snapshot.Tree.Root.Children[1]
	if category.Kind != views.NodeCategory {
		t.Fatalf("category kind = %q", category.Kind)
	}
	if len(category.Children) != 1 {
		t.Fatalf("category children = %d", len(category.Children))
	}

	collectionNode := category.Children[0]
	if collectionNode.Kind != views.NodeCollection {
		t.Fatalf("collection node kind = %q", collectionNode.Kind)
	}
	if collectionNode.Scope == nil || collectionNode.Scope.LocalName != "E67_Birth" {
		t.Fatalf("collection scope = %#v", collectionNode.Scope)
	}
	if len(collectionNode.PathPrefix) != 1 {
		t.Fatalf("shared prefix length = %d", len(collectionNode.PathPrefix))
	}

	if len(snapshot.Fields) != 2 {
		t.Fatalf("snapshot fields = %d", len(snapshot.Fields))
	}
	if got := snapshot.Fields[1].Path[0].Direction; got != PathDirectionForward {
		t.Fatalf("path direction = %q", got)
	}
}

func TestBuildCollectionSnapshotSortsFields(t *testing.T) {
	collection := domain.Collection{
		Entity:        domain.Entity{ID: "collection-1", SemanticID: "LA.C.1", SystemName: "Birth"},
		OntologyScope: pe("class", "crm", "E67_Birth", 0),
	}
	snapshot := BuildCollectionSnapshot(CollectionSnapshotInput{
		Project:    domain.Project{Entity: domain.Entity{ID: "LA"}},
		Collection: collection,
		Fields: []domain.ResolvedField{
			field("field-late", "LA.F.2", "Late", 20),
			field("field-early", "LA.F.1", "Early", 10),
		},
		Options: Options{Lang: "en"},
	})

	if got := snapshot.Tree.Root.Children[0].ID; got != "field-early" {
		t.Fatalf("first field = %q", got)
	}
	if got := snapshot.Tree.Root.Children[1].ID; got != "field-late" {
		t.Fatalf("second field = %q", got)
	}
}

func TestBuildFieldSnapshotUsesFieldAsRoot(t *testing.T) {
	base := domain.Field{
		Entity:        domain.Entity{ID: "field-1", SemanticID: "LA.F.1", SystemName: "Name"},
		OntologyScope: pe("class", "crm", "E1_CRM_Entity", 0),
	}
	snapshot := BuildFieldSnapshot(FieldSnapshotInput{
		Project: domain.Project{Entity: domain.Entity{ID: "LA"}},
		Field:   base,
		Resolved: field(
			base.ID,
			base.SemanticID,
			base.SystemName,
			1,
			pe("property", "crm", "P1_is_identified_by", 0),
			pe("class", "crm", "E33_E41_Linguistic_Appellation", 1),
		),
		Options: Options{Lang: "en"},
	})

	if got := snapshot.Tree.Root.Path; got != "la-f-1" {
		t.Fatalf("field root path = %q", got)
	}
	if got := snapshot.Fields[0].RelativePath; got != "la-f-1" {
		t.Fatalf("field relative path = %q", got)
	}
	if got := snapshot.Fields[0].Path[1].RelativePath; got != "la-f-1/path/crm-e33-e41-linguistic-appellation" {
		t.Fatalf("path node relative path = %q", got)
	}
}

func TestBuildCollectionSnapshotReportsLegacyInversePathByDefault(t *testing.T) {
	collection := domain.Collection{
		Entity:        domain.Entity{ID: "collection-1", SemanticID: "LA.C.1"},
		OntologyScope: pe("class", "crm", "E67_Birth", 0),
	}
	snapshot := BuildCollectionSnapshot(CollectionSnapshotInput{
		Project:    domain.Project{Entity: domain.Entity{ID: "LA"}},
		Collection: collection,
		Fields: []domain.ResolvedField{
			field("field-inverse", "LA.F.1", "Inverse", 1, pe("property", "crm", "^P140_assigned_attribute_to", 0)),
		},
	})

	if len(snapshot.Report.Errors) != 1 {
		t.Fatalf("errors = %d, want 1", len(snapshot.Report.Errors))
	}
	if got := snapshot.Report.Errors[0].Code; got != "legacy_inverse_path" {
		t.Fatalf("error code = %q", got)
	}
	if got := snapshot.Fields[0].Path[0].Direction; got != PathDirectionForward {
		t.Fatalf("direction = %q", got)
	}
}

func TestBuildCollectionSnapshotCanNormalizeLegacyInversePath(t *testing.T) {
	collection := domain.Collection{
		Entity:        domain.Entity{ID: "collection-1", SemanticID: "LA.C.1"},
		OntologyScope: pe("class", "crm", "E67_Birth", 0),
	}
	snapshot := BuildCollectionSnapshot(CollectionSnapshotInput{
		Project:    domain.Project{Entity: domain.Entity{ID: "LA"}},
		Collection: collection,
		Fields: []domain.ResolvedField{
			field("field-inverse", "LA.F.1", "Inverse", 1, pe("property", "crm", "^P140_assigned_attribute_to", 0)),
		},
		Options: Options{LegacyInverseMode: LegacyInverseNormalize},
	})

	if len(snapshot.Report.Errors) != 0 {
		t.Fatalf("errors = %d, want 0", len(snapshot.Report.Errors))
	}
	if len(snapshot.Report.Warnings) != 1 {
		t.Fatalf("warnings = %d, want 1", len(snapshot.Report.Warnings))
	}
	if got := snapshot.Fields[0].Path[0].Element.LocalName; got != "P140_assigned_attribute_to" {
		t.Fatalf("local name = %q", got)
	}
}

func pe(kind, prefix, localName string, position int) domain.PathElement {
	return domain.PathElement{
		Type:      kind,
		URI:       prefix + ":" + localName,
		Prefix:    prefix,
		LocalName: localName,
		Position:  position,
	}
}

func field(id, semanticID, name string, position int, path ...domain.PathElement) domain.ResolvedField {
	return domain.ResolvedField{
		ID:           id,
		SemanticID:   semanticID,
		SystemName:   name,
		DisplayName:  domain.Translations{"en": name},
		Description:  domain.Translations{"en": name + " description"},
		Position:     position,
		PathElements: path,
	}
}

// TestAssignShortPathNodeIDsSharesNodes verifies the generated node
// identity: class nodes reached through the same path prefix get the
// same path_node chain and the same short path_node_id, while distinct
// nodes get distinct ids — the coreference contract the X3ML variables
// and the export tree depend on.
func TestAssignShortPathNodeIDsSharesNodes(t *testing.T) {
	cls := func(code, localName string, pos int) domain.PathElement {
		return domain.PathElement{
			Type: "class", Prefix: "crm", LocalName: localName,
			URI: "crm:" + localName, Position: pos, ClassCode: code,
		}
	}
	view := domain.ModelView{
		ModelID: "m1", ProjectID: "LA",
		Categories: []domain.CategoryGroup{{
			ID: "cat-1", Name: domain.Translations{"en": "Identity"}, Position: 1,
			Collections: []domain.CollectionGroup{{
				ID: directCollectionID, Name: domain.Translations{"en": "Direct"}, Position: 1,
				Fields: []domain.ResolvedField{
					field("f-content", "LA.F.1", "Name content", 1,
						pe("property", "crm", "P1_is_identified_by", 0),
						cls("E33_E41", "E33_E41_Linguistic_Appellation", 1),
						pe("property", "crm", "P190_has_symbolic_content", 2),
						domain.PathElement{Type: "literal", Prefix: "rdf", LocalName: "literal", URI: "rdf:literal", Position: 3},
					),
					field("f-type", "LA.F.2", "Name type", 2,
						pe("property", "crm", "P1_is_identified_by", 0),
						cls("E33_E41", "E33_E41_Linguistic_Appellation", 1),
						pe("property", "crm", "P2_has_type", 2),
						cls("E55", "E55_Type", 3),
					),
					field("f-place", "LA.F.3", "Birth place", 3,
						pe("property", "crm", "P98i_was_born", 0),
						cls("E67", "E67_Birth", 1),
						pe("property", "crm", "P7_took_place_at", 2),
						cls("E53", "E53_Place", 3),
					),
				},
			}},
		}},
	}
	snap := BuildModelSnapshot(ModelSnapshotInput{
		Project: domain.Project{Entity: domain.Entity{ID: "LA"}},
		Model: domain.Model{
			Entity:        domain.Entity{ID: "m1", SemanticID: "LA.M.1", SystemName: "person"},
			OntologyScope: pe("class", "crm", "E21_Person", 0),
		},
		View:       view,
		Namespaces: BuildNamespaceSet(domain.Project{Entity: domain.Entity{ID: "LA"}}, nil),
		Options:    Options{Lang: "en"},
	})

	elem := func(fieldID string, idx int) domain.PathElement {
		for _, fn := range snap.Fields {
			if fn.Field.ID == fieldID {
				return fn.Path[idx].Element
			}
		}
		t.Fatalf("field %s not found", fieldID)
		return domain.PathElement{}
	}

	// Both name fields reach the same appellation node (index 1) through
	// the same prefix → identical path_node + path_node_id.
	content := elem("f-content", 1)
	typeAppel := elem("f-type", 1)
	if content.PathNode == "" || content.PathNodeID == "" {
		t.Fatalf("appellation node missing path_node/path_node_id: %+v", content)
	}
	if content.PathNode != typeAppel.PathNode {
		t.Errorf("shared node path_node differs: %q vs %q", content.PathNode, typeAppel.PathNode)
	}
	if content.PathNodeID != typeAppel.PathNodeID {
		t.Errorf("shared node path_node_id differs: %q vs %q", content.PathNodeID, typeAppel.PathNodeID)
	}
	if want := "cat-1/__direct__/crm_p1_is_identified_by/crm_e33_e41_linguistic_appellation"; content.PathNode != want {
		t.Errorf("path_node chain = %q, want %q", content.PathNode, want)
	}
	if !strings.HasPrefix(content.PathNodeID, "e33_e41_") {
		t.Errorf("path_node_id %q lacks the e33_e41_ class-code stem", content.PathNodeID)
	}

	// Terminal E55_Type and E53_Place are distinct nodes → distinct ids,
	// neither colliding with the shared appellation.
	e55 := elem("f-type", 3)
	e53 := elem("f-place", 3)
	for _, id := range []string{e55.PathNodeID, e53.PathNodeID} {
		if id == "" {
			t.Fatal("terminal node missing path_node_id")
		}
		if id == content.PathNodeID {
			t.Errorf("terminal node collides with the appellation node: %q", id)
		}
	}
	if e55.PathNodeID == e53.PathNodeID {
		t.Errorf("distinct terminal nodes share path_node_id %q", e55.PathNodeID)
	}

	// path_node_id must be 1:1 with path_node across the whole snapshot.
	chainByID := map[string]string{}
	for _, fn := range snap.Fields {
		for _, pn := range fn.Path {
			e := pn.Element
			if e.Type != "class" {
				continue
			}
			if prev, ok := chainByID[e.PathNodeID]; ok && prev != e.PathNode {
				t.Errorf("path_node_id %q maps to two chains: %q and %q", e.PathNodeID, prev, e.PathNode)
			}
			chainByID[e.PathNodeID] = e.PathNode
		}
	}
}
