package rdf

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/generators"
)

func TestTurtleRendererSpec(t *testing.T) {
	spec := NewTurtleRenderer().Spec()

	if spec.Format != generators.FormatTurtle {
		t.Fatalf("format = %q", spec.Format)
	}
	if spec.ContentType != "text/turtle; charset=utf-8" {
		t.Fatalf("content type = %q", spec.ContentType)
	}
	if spec.FileExtension != ".ttl" {
		t.Fatalf("extension = %q", spec.FileExtension)
	}
	if !spec.RequiresTree {
		t.Fatal("requires tree = false, want true")
	}
}

func TestJSONLDRendererSpec(t *testing.T) {
	spec := NewJSONLDRenderer().Spec()

	if spec.Format != generators.FormatJSONLD {
		t.Fatalf("format = %q", spec.Format)
	}
	if spec.ContentType != "application/ld+json; charset=utf-8" {
		t.Fatalf("content type = %q", spec.ContentType)
	}
	if spec.FileExtension != ".jsonld" {
		t.Fatalf("extension = %q", spec.FileExtension)
	}
	if !spec.RequiresTree {
		t.Fatal("requires tree = false, want true")
	}
}

func TestJSONLDRendererCanRenderBlankNodesWithLabels(t *testing.T) {
	snap := generators.BuildCollectionSnapshot(generators.CollectionSnapshotInput{
		Project: domain.Project{
			Entity:    domain.Entity{ID: "LA"},
			Namespace: "https://data.example.org/ns/LA/",
		},
		Collection: domain.Collection{
			Entity:        domain.Entity{ID: "collection-1", SemanticID: "LAC.1", SystemName: "name"},
			OntologyScope: pe("class", "crm", "E1_CRM_Entity", 0),
		},
		Fields: []domain.ResolvedField{
			{
				ID:          "field-content",
				SemanticID:  "LAF.6",
				SystemName:  "name_content",
				DisplayName: domain.Translations{"en": "Name"},
				PathElements: []domain.PathElement{
					pe("property", "crm", "P1_is_identified_by", 0),
					pe("class", "crm", "E33_E41_Linguistic_Appellation", 1),
					pe("property", "crm", "P190_has_symbolic_content", 2),
					{Type: "literal", Prefix: "rdf", LocalName: "langString", URI: "rdf:langString", Position: 3},
				},
			},
			{
				ID:          "field-type",
				SemanticID:  "LAF.5",
				SystemName:  "name_type",
				DisplayName: domain.Translations{"en": "Name Type"},
				PathElements: []domain.PathElement{
					pe("property", "crm", "P1_is_identified_by", 0),
					pe("class", "crm", "E33_E41_Linguistic_Appellation", 1),
					pe("property", "crm", "P2_has_type", 2),
					pe("class", "crm", "E55_Type", 3),
				},
			},
		},
		Namespaces: namespaces(),
		Options:    generators.Options{RDFNodeMode: generators.RDFNodeModeBlankNode},
	})

	var buf bytes.Buffer
	if err := NewJSONLDRenderer().Render(context.Background(), snap, &buf); err != nil {
		t.Fatalf("Render() error: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatalf("invalid JSON-LD: %v\n%s", err, buf.String())
	}
	graph := doc["@graph"].([]any)
	root := graph[0].(map[string]any)
	identifiedBy := root["crm:P1_is_identified_by"].(map[string]any)
	if _, ok := identifiedBy["@id"]; ok {
		t.Fatalf("blank node unexpectedly has @id: %v", identifiedBy)
	}
	content := identifiedBy["crm:P190_has_symbolic_content"].(map[string]any)
	if got := content["_label"]; got != "LAF.6 Name" {
		t.Fatalf("_label = %v", got)
	}
	nameType := identifiedBy["crm:P2_has_type"].(map[string]any)
	if got := nameType["_label"]; got != "LAF.5 Name Type" {
		t.Fatalf("type _label = %v", got)
	}
}

func TestTurtleRendererRendersCollectionSnapshot(t *testing.T) {
	snap := generators.BuildCollectionSnapshot(generators.CollectionSnapshotInput{
		Project: domain.Project{
			Entity:    domain.Entity{ID: "LA"},
			Namespace: "https://data.example.org/ns/LA/",
		},
		Collection: domain.Collection{
			Entity:        domain.Entity{ID: "collection-1", SemanticID: "LAC.1", SystemName: "birth"},
			OntologyScope: pe("class", "crm", "E67_Birth", 0),
		},
		Fields: []domain.ResolvedField{
			{
				ID:         "field-1",
				SemanticID: "LAF.1",
				SystemName: "birth_date",
				PathElements: []domain.PathElement{
					pe("property", "crm", "P4_has_time-span", 0),
					peWithInstance("class", "crm", "E52_Time-Span", 1, "187_1"),
					pe("property", "crm", "P82_at_some_time_within", 2),
					{Type: "literal", Prefix: "xsd", LocalName: "date", URI: "xsd:date", Position: 3},
				},
			},
		},
		Namespaces: namespaces(),
		Options:    generators.Options{RDFNodeMode: generators.RDFNodeModeResource},
	})

	var buf bytes.Buffer
	if err := NewTurtleRenderer().Render(context.Background(), snap, &buf); err != nil {
		t.Fatalf("Render() error: %v", err)
	}

	assertTurtle(t, buf.String(), `
@base <https://data.example.org/ns/LA/> .
@prefix ex: <https://data.example.org/ns/LA/> .
@prefix crm: <http://www.cidoc-crm.org/cidoc-crm/> .
@prefix rdf: <http://www.w3.org/1999/02/22-rdf-syntax-ns#> .

# Collection LAC.1 birth
<lac-1> rdf:type crm:E67_Birth ;
    crm:P4_has_time-span <lac-1/crm-e52-time-span> .

<lac-1/crm-e52-time-span> rdf:type crm:E52_Time-Span ;
    # LAF.1 birth_date
    crm:P82_at_some_time_within "birth_date_value" .
`)
}

func TestTurtleRendererGroupsFieldsBySharedClassPath(t *testing.T) {
	snap := generators.BuildCollectionSnapshot(generators.CollectionSnapshotInput{
		Project: domain.Project{
			Entity:    domain.Entity{ID: "LA"},
			Namespace: "https://data.example.org/ns/LA/",
		},
		Collection: domain.Collection{
			Entity:        domain.Entity{ID: "collection-1", SemanticID: "LAC.1", SystemName: "name"},
			OntologyScope: pe("class", "crm", "E1_CRM_Entity", 0),
		},
		Fields: []domain.ResolvedField{
			{
				ID:         "field-content",
				SemanticID: "LAF.6",
				SystemName: "name_content",
				Description: domain.Translations{
					"en": "This field records the string value of the name attributed to the documented entity.",
				},
				PathElements: []domain.PathElement{
					pe("property", "crm", "P1_is_identified_by", 0),
					peWithInstance("class", "crm", "E33_E41_Linguistic_Appellation", 1, "4_1"),
					pe("property", "crm", "P190_has_symbolic_content", 2),
					{Type: "literal", Prefix: "rdf", LocalName: "langString", URI: "rdf:langString", Position: 3},
				},
			},
			{
				ID:         "field-type",
				SemanticID: "LAF.5",
				SystemName: "name_type",
				Description: domain.Translations{
					"en": "This field records the type assigned to the name.",
				},
				PathElements: []domain.PathElement{
					pe("property", "crm", "P1_is_identified_by", 0),
					peWithInstance("class", "crm", "E33_E41_Linguistic_Appellation", 1, "4_1"),
					pe("property", "crm", "P2_has_type", 2),
					peWithInstance("class", "crm", "E55_Type", 3, "5_1"),
				},
			},
			{
				ID:         "field-label",
				SemanticID: "LAF.4",
				SystemName: "name_label",
				Description: domain.Translations{
					"en": "This field records a readable label for the name.",
				},
				PathElements: []domain.PathElement{
					pe("property", "crm", "P1_is_identified_by", 0),
					peWithInstance("class", "crm", "E33_E41_Linguistic_Appellation", 1, "4_1"),
					pe("property", "rdfs", "label", 2),
					{Type: "literal", Prefix: "rdf", LocalName: "langString", URI: "rdf:langString", Position: 3},
				},
			},
		},
		Namespaces: namespaces(),
		Options:    generators.Options{RDFNodeMode: generators.RDFNodeModeResource},
	})

	var buf bytes.Buffer
	if err := NewTurtleRenderer().Render(context.Background(), snap, &buf); err != nil {
		t.Fatalf("Render() error: %v", err)
	}

	assertTurtle(t, buf.String(), `
@base <https://data.example.org/ns/LA/> .
@prefix ex: <https://data.example.org/ns/LA/> .
@prefix crm: <http://www.cidoc-crm.org/cidoc-crm/> .
@prefix rdf: <http://www.w3.org/1999/02/22-rdf-syntax-ns#> .
@prefix rdfs: <http://www.w3.org/2000/01/rdf-schema#> .

# Collection LAC.1 name
<lac-1> rdf:type crm:E1_CRM_Entity ;
    crm:P1_is_identified_by <lac-1/crm-e33-e41-linguistic-appellation> .

<lac-1/crm-e33-e41-linguistic-appellation> rdf:type crm:E33_E41_Linguistic_Appellation ;
    # LAF.6 name_content
    # This field records the string value of the name attributed to the documented entity.
    crm:P190_has_symbolic_content "name_content_value" ;
    # LAF.5 name_type
    # This field records the type assigned to the name.
    crm:P2_has_type <lac-1/crm-e33-e41-linguistic-appellation/crm-e55-type> ;
    # LAF.4 name_label
    # This field records a readable label for the name.
    rdfs:label "name_label_value" .

<lac-1/crm-e33-e41-linguistic-appellation/crm-e55-type> rdf:type crm:E55_Type .
`)
}

func TestTurtleRendererDefaultsClassResourcesToBlankNodes(t *testing.T) {
	snap := generators.BuildCollectionSnapshot(generators.CollectionSnapshotInput{
		Project: domain.Project{
			Entity:    domain.Entity{ID: "LA"},
			Namespace: "https://data.example.org/ns/LA/",
		},
		Collection: domain.Collection{
			Entity:        domain.Entity{ID: "collection-1", SemanticID: "LAC.1", SystemName: "name"},
			OntologyScope: pe("class", "crm", "E1_CRM_Entity", 0),
		},
		Fields: []domain.ResolvedField{
			{
				ID:         "field-content",
				SemanticID: "LAF.6",
				SystemName: "name_content",
				Description: domain.Translations{
					"en": "This field records the string value of the name.",
				},
				PathElements: []domain.PathElement{
					pe("property", "crm", "P1_is_identified_by", 0),
					pe("class", "crm", "E33_E41_Linguistic_Appellation", 1),
					pe("property", "crm", "P190_has_symbolic_content", 2),
					{Type: "literal", Prefix: "rdf", LocalName: "langString", URI: "rdf:langString", Position: 3},
				},
			},
			{
				ID:         "field-type",
				SemanticID: "LAF.5",
				SystemName: "name_type",
				Description: domain.Translations{
					"en": "This field records the type assigned to the name.",
				},
				PathElements: []domain.PathElement{
					pe("property", "crm", "P1_is_identified_by", 0),
					pe("class", "crm", "E33_E41_Linguistic_Appellation", 1),
					pe("property", "crm", "P2_has_type", 2),
					pe("class", "crm", "E55_Type", 3),
				},
			},
			{
				ID:         "field-label",
				SemanticID: "LAF.4",
				SystemName: "name_label",
				Description: domain.Translations{
					"en": "This field records a readable label for the name.",
				},
				PathElements: []domain.PathElement{
					pe("property", "crm", "P1_is_identified_by", 0),
					pe("class", "crm", "E33_E41_Linguistic_Appellation", 1),
					pe("property", "rdfs", "label", 2),
					{Type: "literal", Prefix: "rdf", LocalName: "langString", URI: "rdf:langString", Position: 3},
				},
			},
		},
		Namespaces: namespaces(),
	})

	var buf bytes.Buffer
	if err := NewTurtleRenderer().Render(context.Background(), snap, &buf); err != nil {
		t.Fatalf("Render() error: %v", err)
	}

	assertTurtle(t, buf.String(), `
@base <https://data.example.org/ns/LA/> .
@prefix ex: <https://data.example.org/ns/LA/> .
@prefix crm: <http://www.cidoc-crm.org/cidoc-crm/> .
@prefix rdfs: <http://www.w3.org/2000/01/rdf-schema#> .

# Collection LAC.1 name
<lac-1> a crm:E1_CRM_Entity ;
    crm:P1_is_identified_by [
        a crm:E33_E41_Linguistic_Appellation ;
        # LAF.6 name_content
        # This field records the string value of the name.
        crm:P190_has_symbolic_content "name_content_value" ;
        # LAF.5 name_type
        # This field records the type assigned to the name.
        crm:P2_has_type [
            a crm:E55_Type
        ] ;
        # LAF.4 name_label
        # This field records a readable label for the name.
        rdfs:label "name_label_value"
    ] .
`)
}

func TestTurtleRendererRendersModelCollectionsAsResourceGroups(t *testing.T) {
	snap := generators.BuildModelSnapshot(generators.ModelSnapshotInput{
		Project: domain.Project{
			Entity:    domain.Entity{ID: "LA"},
			Namespace: "https://data.example.org/ns/LA/",
		},
		Model: domain.Model{
			Entity:        domain.Entity{ID: "model-1", SemanticID: "LAM.1", SystemName: "person"},
			OntologyScope: pe("class", "crm", "E21_Person", 0),
		},
		View: domain.ModelView{
			Categories: []domain.CategoryGroup{
				{
					ID:       "identity",
					Name:     domain.Translations{"en": "Identity"},
					Position: 1,
					Collections: []domain.CollectionGroup{
						{
							ID:       "collection-1",
							Name:     domain.Translations{"en": "Birth events"},
							Position: 1,
							Fields: []domain.ResolvedField{
								{
									ID:         "field-1",
									SemanticID: "LAF.1",
									SystemName: "birth_place",
									PathElements: []domain.PathElement{
										pe("property", "crm", "P98i_was_born", 0),
										pe("class", "crm", "E67_Birth", 1),
										pe("property", "crm", "P7_took_place_at", 2),
										pe("class", "crm", "E53_Place", 3),
									},
								},
								{
									ID:         "field-2",
									SemanticID: "LAF.2",
									SystemName: "birth_label",
									PathElements: []domain.PathElement{
										pe("property", "crm", "P98i_was_born", 0),
										pe("class", "crm", "E67_Birth", 1),
										pe("property", "rdfs", "label", 2),
										{Type: "literal", Prefix: "rdf", LocalName: "langString", URI: "rdf:langString", Position: 3},
									},
									SetValue: "Birth",
								},
							},
						},
					},
				},
			},
		},
		Collections: map[string]*domain.Collection{
			"collection-1": {
				Entity:        domain.Entity{ID: "collection-1", SemanticID: "LAC.1", SystemName: "birth"},
				OntologyScope: pe("class", "crm", "E67_Birth", 0),
			},
		},
		Namespaces: namespaces(),
		Options:    generators.Options{RDFNodeMode: generators.RDFNodeModeResource},
	})

	var buf bytes.Buffer
	if err := NewTurtleRenderer().Render(context.Background(), snap, &buf); err != nil {
		t.Fatalf("Render() error: %v", err)
	}

	assertTurtle(t, buf.String(), `
@base <https://data.example.org/ns/LA/> .
@prefix ex: <https://data.example.org/ns/LA/> .
@prefix crm: <http://www.cidoc-crm.org/cidoc-crm/> .
@prefix rdf: <http://www.w3.org/1999/02/22-rdf-syntax-ns#> .
@prefix rdfs: <http://www.w3.org/2000/01/rdf-schema#> .

<lam-1> rdf:type crm:E21_Person ;
    crm:P98i_was_born <lam-1/crm-p98i-was-born/crm-e67-birth> .

<lam-1/crm-p98i-was-born/crm-e67-birth>
    # Collection LAC.1 Birth events
    rdf:type crm:E67_Birth ;
    # LAF.1 birth_place
    crm:P7_took_place_at <lam-1/crm-p98i-was-born/crm-e67-birth/crm-p7-took-place-at/crm-e53-place> ;
    # LAF.2 birth_label
    rdfs:label "Birth" .

<lam-1/crm-p98i-was-born/crm-e67-birth/crm-p7-took-place-at/crm-e53-place> rdf:type crm:E53_Place .
`)
}

func TestTurtleRendererUsesSnapshotTreeOrder(t *testing.T) {
	snap := generators.BuildCollectionSnapshot(generators.CollectionSnapshotInput{
		Project: domain.Project{
			Entity:    domain.Entity{ID: "LA"},
			Namespace: "https://data.example.org/ns/LA/",
		},
		Collection: domain.Collection{
			Entity:        domain.Entity{ID: "collection-1", SemanticID: "LAC.1", SystemName: "thing"},
			OntologyScope: pe("class", "crm", "E1_CRM_Entity", 0),
		},
		Fields: []domain.ResolvedField{
			{
				ID:         "field-first",
				SemanticID: "LAF.1",
				SystemName: "first",
				Position:   1,
				PathElements: []domain.PathElement{
					pe("property", "crm", "P1_is_identified_by", 0),
					pe("class", "crm", "E42_Identifier", 1),
				},
			},
			{
				ID:         "field-second",
				SemanticID: "LAF.2",
				SystemName: "second",
				Position:   2,
				PathElements: []domain.PathElement{
					pe("property", "crm", "P2_has_type", 0),
					pe("class", "crm", "E55_Type", 1),
				},
			},
		},
		Namespaces: namespaces(),
	})
	snap.Fields[0], snap.Fields[1] = snap.Fields[1], snap.Fields[0]

	var buf bytes.Buffer
	if err := NewTurtleRenderer().Render(context.Background(), snap, &buf); err != nil {
		t.Fatalf("Render() error: %v", err)
	}

	out := buf.String()
	first := strings.Index(out, "crm:P1_is_identified_by")
	second := strings.Index(out, "crm:P2_has_type")
	if first == -1 || second == -1 {
		t.Fatalf("missing expected predicates in\n%s", out)
	}
	if first > second {
		t.Fatalf("renderer ignored Snapshot.Tree order\n%s", out)
	}
}

func TestTurtleRendererDeclaresKnownUsedPrefixes(t *testing.T) {
	snap := generators.BuildCollectionSnapshot(generators.CollectionSnapshotInput{
		Project: domain.Project{
			Entity:    domain.Entity{ID: "LA"},
			Namespace: "https://data.example.org/ns/LA/",
		},
		Collection: domain.Collection{
			Entity:        domain.Entity{ID: "collection-1", SemanticID: "LAC.1", SystemName: "thing"},
			OntologyScope: pe("class", "crm", "E1_CRM_Entity", 0),
		},
		Fields: []domain.ResolvedField{
			{
				ID:         "field-1",
				SemanticID: "LAF.1",
				SystemName: "same_as",
				PathElements: []domain.PathElement{
					pe("property", "la", "equivalent", 0),
					pe("class", "crm", "E1_CRM_Entity", 1),
				},
			},
			{
				ID:         "field-2",
				SemanticID: "LAF.2",
				SystemName: "label",
				PathElements: []domain.PathElement{
					pe("property", "rdfs", "label", 0),
					{Type: "literal", Prefix: "rdf", LocalName: "langString", URI: "rdf:langString", Position: 1},
				},
				SetValue: "Label",
			},
		},
		Namespaces: generators.BuildNamespaceSet(
			domain.Project{
				Entity:    domain.Entity{ID: "LA"},
				Namespace: "https://data.example.org/ns/LA/",
			},
			[]*domain.NamespaceBinding{
				{Prefix: "crm", Namespace: "http://www.cidoc-crm.org/cidoc-crm/"},
				{Prefix: "la", Namespace: "https://linked.art/ns/terms/"},
				{Prefix: "rdf", Namespace: "http://www.w3.org/1999/02/22-rdf-syntax-ns#"},
				{Prefix: "rdfs", Namespace: "http://www.w3.org/2000/01/rdf-schema#"},
			},
		),
	})

	var buf bytes.Buffer
	if err := NewTurtleRenderer().Render(context.Background(), snap, &buf); err != nil {
		t.Fatalf("Render() error: %v", err)
	}
	out := buf.String()
	for _, prefix := range []string{"@prefix la:", "@prefix rdfs:"} {
		if !strings.Contains(out, prefix) {
			t.Fatalf("missing %s in\n%s", prefix, out)
		}
	}
}

func TestTurtleRendererRejectsMissingNamespaceBinding(t *testing.T) {
	snap := generators.BuildCollectionSnapshot(generators.CollectionSnapshotInput{
		Project: domain.Project{
			Entity:    domain.Entity{ID: "LA"},
			Namespace: "https://data.example.org/ns/LA/",
		},
		Collection: domain.Collection{
			Entity:        domain.Entity{ID: "collection-1", SemanticID: "LAC.1", SystemName: "thing"},
			OntologyScope: pe("class", "crm", "E1_CRM_Entity", 0),
		},
		Fields: []domain.ResolvedField{
			{
				ID:         "field-1",
				SemanticID: "LAF.1",
				SystemName: "label",
				PathElements: []domain.PathElement{
					pe("property", "rdfs", "label", 0),
					{Type: "literal", Prefix: "rdf", LocalName: "langString", URI: "rdf:langString", Position: 1},
				},
			},
		},
		Namespaces: generators.BuildNamespaceSet(
			domain.Project{
				Entity:    domain.Entity{ID: "LA"},
				Namespace: "https://data.example.org/ns/LA/",
			},
			[]*domain.NamespaceBinding{
				{Prefix: "crm", Namespace: "http://www.cidoc-crm.org/cidoc-crm/"},
				{Prefix: "rdf", Namespace: "http://www.w3.org/1999/02/22-rdf-syntax-ns#"},
			},
		),
	})

	var buf bytes.Buffer
	err := NewTurtleRenderer().Render(context.Background(), snap, &buf)
	if err == nil || !strings.Contains(err.Error(), "missing namespace binding(s) for prefix: rdfs") {
		t.Fatalf("Render() error = %v, want missing rdfs namespace", err)
	}
}

func TestJSONLDRendererRejectsMissingNamespaceBinding(t *testing.T) {
	snap := generators.BuildCollectionSnapshot(generators.CollectionSnapshotInput{
		Project: domain.Project{
			Entity:    domain.Entity{ID: "LA"},
			Namespace: "https://data.example.org/ns/LA/",
		},
		Collection: domain.Collection{
			Entity:        domain.Entity{ID: "collection-1", SemanticID: "LAC.1", SystemName: "thing"},
			OntologyScope: pe("class", "crm", "E1_CRM_Entity", 0),
		},
		Fields: []domain.ResolvedField{
			{
				ID:         "field-1",
				SemanticID: "LAF.1",
				SystemName: "label",
				PathElements: []domain.PathElement{
					pe("property", "rdfs", "label", 0),
					{Type: "literal", Prefix: "rdf", LocalName: "langString", URI: "rdf:langString", Position: 1},
				},
			},
		},
		Namespaces: generators.BuildNamespaceSet(
			domain.Project{
				Entity:    domain.Entity{ID: "LA"},
				Namespace: "https://data.example.org/ns/LA/",
			},
			[]*domain.NamespaceBinding{
				{Prefix: "crm", Namespace: "http://www.cidoc-crm.org/cidoc-crm/"},
				{Prefix: "rdf", Namespace: "http://www.w3.org/1999/02/22-rdf-syntax-ns#"},
			},
		),
	})

	var buf bytes.Buffer
	err := NewJSONLDRenderer().Render(context.Background(), snap, &buf)
	if err == nil || !strings.Contains(err.Error(), `missing namespace binding for prefix "rdfs"`) {
		t.Fatalf("Render() error = %v, want missing rdfs namespace", err)
	}
}

func TestTurtleRendererUsesFieldIdentityCommentWhenDescriptionMissing(t *testing.T) {
	snap := generators.BuildCollectionSnapshot(generators.CollectionSnapshotInput{
		Project: domain.Project{
			Entity:    domain.Entity{ID: "LA"},
			Namespace: "https://data.example.org/ns/LA/",
		},
		Collection: domain.Collection{
			Entity:        domain.Entity{ID: "collection-1", SemanticID: "LAC.1", SystemName: "thing"},
			OntologyScope: pe("class", "crm", "E1_CRM_Entity", 0),
		},
		Fields: []domain.ResolvedField{
			{
				ID:          "field-1",
				SemanticID:  "LAF.6",
				SystemName:  "name_content",
				DisplayName: domain.Translations{"en": "Name"},
				PathElements: []domain.PathElement{
					pe("property", "crm", "P1_is_identified_by", 0),
					pe("class", "crm", "E33_E41_Linguistic_Appellation", 1),
					pe("property", "crm", "P190_has_symbolic_content", 2),
					{Type: "literal", Prefix: "rdf", LocalName: "langString", URI: "rdf:langString", Position: 3},
				},
			},
		},
		Namespaces: namespaces(),
	})

	var buf bytes.Buffer
	if err := NewTurtleRenderer().Render(context.Background(), snap, &buf); err != nil {
		t.Fatalf("Render() error: %v", err)
	}
	if !strings.Contains(buf.String(), "# LAF.6 Name") {
		t.Fatalf("missing fallback field comment in\n%s", buf.String())
	}
}

func TestTurtleRendererIncludesFieldIdentityBeforeDescription(t *testing.T) {
	snap := generators.BuildCollectionSnapshot(generators.CollectionSnapshotInput{
		Project: domain.Project{
			Entity:    domain.Entity{ID: "LA"},
			Namespace: "https://data.example.org/ns/LA/",
		},
		Collection: domain.Collection{
			Entity:        domain.Entity{ID: "collection-1", SemanticID: "LAC.1", SystemName: "thing"},
			OntologyScope: pe("class", "crm", "E1_CRM_Entity", 0),
		},
		Fields: []domain.ResolvedField{
			{
				ID:          "field-1",
				SemanticID:  "LAF.6",
				SystemName:  "statement_language",
				DisplayName: domain.Translations{"en": "Statement Language"},
				Description: domain.Translations{"en": "This field is used to record the language of the statement describing the documented entity."},
				PathElements: []domain.PathElement{
					pe("property", "crm", "P67i_is_referred_to_by", 0),
					pe("class", "crm", "E33_Linguistic_Object", 1),
					pe("property", "crm", "P72_has_language", 2),
					pe("class", "crm", "E56_Language", 3),
				},
			},
		},
		Namespaces: namespaces(),
	})

	var buf bytes.Buffer
	if err := NewTurtleRenderer().Render(context.Background(), snap, &buf); err != nil {
		t.Fatalf("Render() error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "# LAF.6 Statement Language") {
		t.Fatalf("missing identity field comment in\n%s", out)
	}
	if !strings.Contains(out, "# This field is used to record the language of the statement describing the documented entity.") {
		t.Fatalf("missing description field comment in\n%s", out)
	}
}

func TestTurtleRendererRejectsSnapshotErrors(t *testing.T) {
	snap := &generators.Snapshot{
		Namespaces: namespaces(),
		Report: generators.Report{
			Errors: []generators.Diagnostic{{Code: "legacy_inverse_path"}},
		},
	}

	var buf bytes.Buffer
	if err := NewTurtleRenderer().Render(context.Background(), snap, &buf); err == nil {
		t.Fatal("Render() error = nil, want error")
	}
}

func TestTermFallsBackToUnknownURNWhenUnresolved(t *testing.T) {
	// An element with no prefixed name (missing Prefix/LocalName) and no URI
	// cannot be resolved to a term, so term() must fall back to a stable
	// placeholder URN rather than emitting an empty or malformed IRI.
	element := domain.PathElement{Type: "property", Position: 0}

	got := term(element)

	if !strings.Contains(got, "urn:pletka:unknown") {
		t.Fatalf("term() = %q, want it to contain %q", got, "urn:pletka:unknown")
	}
}

func TestTurtleRendererEmitsMultiClassScope(t *testing.T) {
	scope := pe("class", "crmdig", "D1_Digital_Object", 0)
	scope.AdditionalTypes = []domain.TypeRef{
		{URI: "crm:E36_Visual_Item", Prefix: "crm", LocalName: "E36_Visual_Item", ClassCode: "E36"},
	}
	snap := generators.BuildCollectionSnapshot(generators.CollectionSnapshotInput{
		Project: domain.Project{
			Entity:    domain.Entity{ID: "LA"},
			Namespace: "https://data.example.org/ns/LA/",
		},
		Collection: domain.Collection{
			Entity:        domain.Entity{ID: "collection-1", SemanticID: "LAC.1", SystemName: "image"},
			OntologyScope: scope,
		},
		Fields: []domain.ResolvedField{
			{
				ID:         "field-1",
				SemanticID: "LAF.1",
				SystemName: "label",
				PathElements: []domain.PathElement{
					pe("property", "rdfs", "label", 0),
					{Type: "literal", Prefix: "rdf", LocalName: "langString", URI: "rdf:langString", Position: 1},
				},
			},
		},
		Namespaces: generators.BuildNamespaceSet(
			domain.Project{
				Entity:    domain.Entity{ID: "LA"},
				Namespace: "https://data.example.org/ns/LA/",
			},
			[]*domain.NamespaceBinding{
				{Prefix: "crm", Namespace: "http://www.cidoc-crm.org/cidoc-crm/"},
				{Prefix: "crmdig", Namespace: "http://www.ics.forth.gr/isl/CRMdig/"},
				{Prefix: "rdf", Namespace: "http://www.w3.org/1999/02/22-rdf-syntax-ns#"},
				{Prefix: "rdfs", Namespace: "http://www.w3.org/2000/01/rdf-schema#"},
			},
		),
		Options: generators.Options{RDFNodeMode: generators.RDFNodeModeResource},
	})

	var buf bytes.Buffer
	if err := NewTurtleRenderer().Render(context.Background(), snap, &buf); err != nil {
		t.Fatalf("Render() error: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "rdf:type crmdig:D1_Digital_Object,") || !strings.Contains(got, "crm:E36_Visual_Item ;") {
		t.Fatalf("expected dual rdf:type for D1 + E36, got:\n%s", got)
	}
}

func TestJSONLDRendererEmitsMultiClassScope(t *testing.T) {
	scope := pe("class", "crmdig", "D1_Digital_Object", 0)
	scope.AdditionalTypes = []domain.TypeRef{
		{URI: "crm:E36_Visual_Item", Prefix: "crm", LocalName: "E36_Visual_Item", ClassCode: "E36"},
	}
	snap := generators.BuildCollectionSnapshot(generators.CollectionSnapshotInput{
		Project: domain.Project{
			Entity:    domain.Entity{ID: "LA"},
			Namespace: "https://data.example.org/ns/LA/",
		},
		Collection: domain.Collection{
			Entity:        domain.Entity{ID: "collection-1", SemanticID: "LAC.1", SystemName: "image"},
			OntologyScope: scope,
		},
		Fields: []domain.ResolvedField{
			{
				ID:         "field-1",
				SemanticID: "LAF.1",
				SystemName: "label",
				PathElements: []domain.PathElement{
					pe("property", "rdfs", "label", 0),
					{Type: "literal", Prefix: "rdf", LocalName: "langString", URI: "rdf:langString", Position: 1},
				},
			},
		},
		Namespaces: generators.BuildNamespaceSet(
			domain.Project{
				Entity:    domain.Entity{ID: "LA"},
				Namespace: "https://data.example.org/ns/LA/",
			},
			[]*domain.NamespaceBinding{
				{Prefix: "crm", Namespace: "http://www.cidoc-crm.org/cidoc-crm/"},
				{Prefix: "crmdig", Namespace: "http://www.ics.forth.gr/isl/CRMdig/"},
				{Prefix: "rdf", Namespace: "http://www.w3.org/1999/02/22-rdf-syntax-ns#"},
				{Prefix: "rdfs", Namespace: "http://www.w3.org/2000/01/rdf-schema#"},
			},
		),
		Options: generators.Options{RDFNodeMode: generators.RDFNodeModeResource},
	})

	var buf bytes.Buffer
	if err := NewJSONLDRenderer().Render(context.Background(), snap, &buf); err != nil {
		t.Fatalf("Render() error: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatalf("invalid JSON-LD: %v\n%s", err, buf.String())
	}
	root := doc["@graph"].([]any)[0].(map[string]any)
	got, ok := root["@type"].([]any)
	if !ok {
		t.Fatalf("@type is not an array: %v", root["@type"])
	}
	if len(got) != 2 || got[0] != "crmdig:D1_Digital_Object" || got[1] != "crm:E36_Visual_Item" {
		t.Fatalf("@type = %v, want [D1, E36]", got)
	}
}

func namespaces() generators.NamespaceSet {
	return generators.BuildNamespaceSet(
		domain.Project{
			Entity:    domain.Entity{ID: "LA"},
			Namespace: "https://data.example.org/ns/LA/",
		},
		[]*domain.NamespaceBinding{
			{Prefix: "crm", Namespace: "http://www.cidoc-crm.org/cidoc-crm/"},
			{Prefix: "dcterms", Namespace: "http://purl.org/dc/terms/"},
			{Prefix: "rdf", Namespace: "http://www.w3.org/1999/02/22-rdf-syntax-ns#"},
			{Prefix: "rdfs", Namespace: "http://www.w3.org/2000/01/rdf-schema#"},
			{Prefix: "xsd", Namespace: "http://www.w3.org/2001/XMLSchema#"},
		},
	)
}

func pe(kind, prefix, localName string, position int) domain.PathElement {
	return peWithInstance(kind, prefix, localName, position, "")
}

func peWithInstance(kind, prefix, localName string, position int, instanceID string) domain.PathElement {
	return domain.PathElement{
		Type:       kind,
		URI:        prefix + ":" + localName,
		Prefix:     prefix,
		LocalName:  localName,
		Position:   position,
		InstanceID: instanceID,
	}
}

func assertTurtle(t *testing.T, got string, want string) {
	t.Helper()
	if strings.TrimSpace(got) != strings.TrimSpace(want) {
		t.Fatalf("turtle mismatch\n--- got ---\n%s\n--- want ---\n%s", got, strings.TrimSpace(want))
	}
}
