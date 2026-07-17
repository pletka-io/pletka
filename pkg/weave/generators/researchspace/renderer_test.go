package researchspace

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/generators"
)

func pe(kind, prefix, localName string, position int) domain.PathElement {
	return domain.PathElement{
		Type:      kind,
		Prefix:    prefix,
		LocalName: localName,
		URI:       prefix + ":" + localName,
		Position:  position,
	}
}

func crmNamespaces() generators.NamespaceSet {
	return generators.NamespaceSet{
		ProjectURI:    "https://data.example.org/ns/LA/",
		ProjectPrefix: "la",
		Bindings: []generators.NamespaceBinding{
			{Prefix: "crm", Namespace: "http://www.cidoc-crm.org/cidoc-crm/"},
			{Prefix: "la", Namespace: "https://data.example.org/ns/LA/"},
		},
	}
}

func TestRSRendererSpec(t *testing.T) {
	spec := NewRenderer().Spec()
	if spec.Format != generators.FormatResearchSpace {
		t.Errorf("format = %q", spec.Format)
	}
	if spec.FileExtension != ".yml" {
		t.Errorf("ext = %q", spec.FileExtension)
	}
	if spec.ContentType != "application/yaml; charset=utf-8" {
		t.Errorf("content type = %q", spec.ContentType)
	}
}

func TestRSRenderEmitsYAMLEnvelopeWithFieldsAndSparql(t *testing.T) {
	snap := generators.BuildCollectionSnapshot(generators.CollectionSnapshotInput{
		Project: domain.Project{
			Entity:    domain.Entity{ID: "LA", UIName: domain.Translations{"en": "LA"}},
			Namespace: "https://data.example.org/ns/LA/",
		},
		Collection: domain.Collection{
			Entity:        domain.Entity{ID: "c-name", SemanticID: "LAC.1", SystemName: "name"},
			OntologyScope: pe("class", "crm", "E1_CRM_Entity", 0),
		},
		Fields: []domain.ResolvedField{
			{
				ID: "f-content", SemanticID: "LAF.1", SystemName: "name_content",
				DisplayName:       domain.Translations{"en": "Name Content"},
				Description:       domain.Translations{"en": "The literal content of the name."},
				ExpectedValueType: "string",
				PathElements: []domain.PathElement{
					pe("property", "crm", "P1_is_identified_by", 0),
					pe("class", "crm", "E33_E41_Linguistic_Appellation", 1),
					pe("property", "crm", "P190_has_symbolic_content", 2),
					{Type: "literal", Prefix: "rdf", LocalName: "langString", URI: "rdf:langString", Position: 3},
				},
			},
			{
				ID: "f-type", SemanticID: "LAF.2", SystemName: "name_type",
				DisplayName:       domain.Translations{"en": "Name Type"},
				ExpectedValueType: "Concept",
				PathElements: []domain.PathElement{
					pe("property", "crm", "P1_is_identified_by", 0),
					pe("class", "crm", "E33_E41_Linguistic_Appellation", 1),
					pe("property", "crm", "P2_has_type", 2),
					pe("class", "crm", "E55_Type", 3),
				},
			},
		},
		Namespaces: crmNamespaces(),
	})

	var buf bytes.Buffer
	if err := NewRenderer().Render(context.Background(), snap, &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	got := buf.String()

	// Quick string assertions before the structured decode.
	for _, want := range []string{
		"prefix:",
		"container:",
		"namespaces:",
		"crm: http://www.cidoc-crm.org/cidoc-crm/",
		"fields:",
		"id: name_content",
		"label: Name Content",
		"datatype: xsd:string",
		"id: name_type",
		"datatype: xsd:anyURI",
		"queries:",
		"PREFIX crm:",
		"SELECT DISTINCT ?LAF_1_name_content WHERE {",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q\n%s", want, got)
		}
	}

	// Structural assertion: the YAML must round-trip into the same shape.
	var doc rsDocument
	if err := yaml.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, got)
	}
	if len(doc.Fields) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(doc.Fields))
	}
	if doc.Fields[0].ID != "name_content" {
		t.Errorf("field[0].id = %q", doc.Fields[0].ID)
	}
	if !strings.Contains(string(doc.Fields[0].Queries[0].Select), "?subject a crm:E1_CRM_Entity") {
		t.Errorf("name_content SPARQL missing scope-class triple:\n%s", doc.Fields[0].Queries[0].Select)
	}
	if doc.Namespaces["crm"] != "http://www.cidoc-crm.org/cidoc-crm/" {
		t.Errorf("crm namespace = %q", doc.Namespaces["crm"])
	}
}

func TestRSDatatypeMapping(t *testing.T) {
	cases := map[string]string{
		"string":          "xsd:string",
		"date":            "xsd:date",
		"dateTime":        "xsd:dateTime",
		"integer":         "xsd:integer",
		"boolean":         "xsd:boolean",
		"decimal":         "xsd:decimal",
		"Concept":         "xsd:anyURI",
		"Reference Model": "xsd:anyURI",
		"URI":             "xsd:anyURI",
		"":                "xsd:anyURI",
		"random":          "xsd:anyURI",
	}
	for in, want := range cases {
		if got := datatypeFor(in); got != want {
			t.Errorf("datatypeFor(%q) = %q, want %q", in, got, want)
		}
	}
}
