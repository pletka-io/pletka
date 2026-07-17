package sparql

import (
	"bytes"
	"context"
	"strings"
	"testing"

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

func TestSPARQLRendererSpec(t *testing.T) {
	spec := NewRenderer().Spec()
	if spec.Format != generators.FormatSPARQL {
		t.Fatalf("format = %q, want sparql", spec.Format)
	}
	if spec.ContentType != "application/sparql-query; charset=utf-8" {
		t.Fatalf("content type = %q", spec.ContentType)
	}
	if spec.FileExtension != ".rq" {
		t.Fatalf("extension = %q", spec.FileExtension)
	}
	if spec.RequiresTree {
		t.Fatal("requires tree = true, want false")
	}
}

func TestSPARQLRenderField(t *testing.T) {
	field := domain.Field{
		Entity:        domain.Entity{ID: "f1", SemanticID: "LAF.1", SystemName: "name"},
		OntologyScope: pe("class", "crm", "E21_Person", 0),
	}
	resolved := domain.ResolvedField{
		ID:                "f1",
		SemanticID:        "LAF.1",
		SystemName:        "name",
		ExpectedValueType: "string",
		PathElements: []domain.PathElement{
			pe("class", "crm", "E21_Person", 0),
			pe("property", "crm", "P1_is_identified_by", 1),
			pe("class", "crm", "E33_E41_Linguistic_Appellation", 2),
			pe("property", "crm", "P190_has_symbolic_content", 3),
			{Type: "literal", Prefix: "rdf", LocalName: "langString", URI: "rdf:langString", Position: 4},
		},
	}
	snap := generators.BuildFieldSnapshot(generators.FieldSnapshotInput{
		Project:    domain.Project{Entity: domain.Entity{ID: "LA"}, Namespace: "https://data.example.org/ns/LA/"},
		Field:      field,
		Resolved:   resolved,
		Namespaces: crmNamespaces(),
	})

	var buf bytes.Buffer
	if err := NewRenderer().Render(context.Background(), snap, &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	got := buf.String()

	for _, line := range []string{
		"PREFIX crm: <http://www.cidoc-crm.org/cidoc-crm/>",
		"PREFIX rdf: <http://www.w3.org/1999/02/22-rdf-syntax-ns#>",
		"PREFIX rdfs: <http://www.w3.org/2000/01/rdf-schema#>",
		"SELECT DISTINCT ?LAF_1_name WHERE {",
		"?subject a crm:E21_Person .",
		"?subject crm:P1_is_identified_by ?e33_e41_linguistic_appellation_1 .",
		"?e33_e41_linguistic_appellation_1 a crm:E33_E41_Linguistic_Appellation .",
		"?e33_e41_linguistic_appellation_1 crm:P190_has_symbolic_content ?LAF_1_name .",
	} {
		if !strings.Contains(got, line) {
			t.Errorf("output missing %q\nGOT:\n%s", line, got)
		}
	}
	if strings.Contains(got, "OPTIONAL { ?LAF_1_name rdfs:label") {
		t.Errorf("expected no rdfs:label OPTIONAL for string-terminal field, got it:\n%s", got)
	}
}

func TestSPARQLRenderEmitsQueryPerSubfieldPath(t *testing.T) {
	// SRD1F.5 shape: primary terminates in symbolic content (literal); the
	// legacy subfield terminates in an E55_Type classification.
	field := domain.Field{
		Entity:        domain.Entity{ID: "f1", SemanticID: "SRD1F.5", SystemName: "name"},
		OntologyScope: pe("class", "crm", "E21_Person", 0),
	}
	resolved := domain.ResolvedField{
		ID:                "f1",
		SemanticID:        "SRD1F.5",
		SystemName:        "name",
		ExpectedValueType: "string",
		PathElements: []domain.PathElement{
			pe("class", "crm", "E21_Person", 0),
			pe("property", "crm", "P1_is_identified_by", 1),
			pe("class", "crm", "E33_E41_Linguistic_Appellation", 2),
			pe("property", "crm", "P190_has_symbolic_content", 3),
			{Type: "literal", Prefix: "rdf", LocalName: "langString", URI: "rdf:langString", Position: 4},
		},
		SubfieldPaths: []domain.SubfieldPath{{
			Source:            "legacy-br-split",
			ExpectedValueType: "crm:E55_Type",
			PathElements: []domain.PathElement{
				pe("property", "crm", "P1_is_identified_by", 0),
				pe("class", "crm", "E33_E41_Linguistic_Appellation", 1),
				pe("property", "crm", "P2_has_type", 2),
				pe("class", "crm", "E55_Type", 3),
			},
		}},
	}
	snap := generators.BuildFieldSnapshot(generators.FieldSnapshotInput{
		Project:    domain.Project{Entity: domain.Entity{ID: "LA"}, Namespace: "https://data.example.org/ns/LA/"},
		Field:      field,
		Resolved:   resolved,
		Namespaces: crmNamespaces(),
	})

	var buf bytes.Buffer
	if err := NewRenderer().Render(context.Background(), snap, &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	got := buf.String()

	// Two independent SELECT queries: primary + one subfield (suffixed _p1).
	if n := strings.Count(got, "SELECT DISTINCT"); n != 2 {
		t.Errorf("want 2 SELECT queries (primary + subfield), got %d\n%s", n, got)
	}
	for _, line := range []string{
		"SELECT DISTINCT ?SRD1F_5_name WHERE {",         // primary
		"crm:P190_has_symbolic_content ?SRD1F_5_name .", // primary terminal
		"SELECT DISTINCT ?SRD1F_5_name_p1 WHERE {",      // subfield query, distinct value var
		"crm:P2_has_type ?SRD1F_5_name_p1 .",            // subfield terminal property → E55_Type value
	} {
		if !strings.Contains(got, line) {
			t.Errorf("output missing %q\nGOT:\n%s", line, got)
		}
	}
}

func TestSPARQLRenderMultiClassScope(t *testing.T) {
	scope := pe("class", "crmdig", "D1_Digital_Object", 0)
	scope.AdditionalTypes = []domain.TypeRef{
		{URI: "crm:E36_Visual_Item", Prefix: "crm", LocalName: "E36_Visual_Item", ClassCode: "E36"},
	}
	field := domain.Field{
		Entity:        domain.Entity{ID: "f1", SemanticID: "LAF.1", SystemName: "label"},
		OntologyScope: scope,
	}
	resolved := domain.ResolvedField{
		ID:                "f1",
		SemanticID:        "LAF.1",
		SystemName:        "label",
		ExpectedValueType: "string",
		PathElements: []domain.PathElement{
			scope,
			pe("property", "rdfs", "label", 1),
			{Type: "literal", Prefix: "rdf", LocalName: "langString", URI: "rdf:langString", Position: 2},
		},
	}
	ns := generators.NamespaceSet{
		ProjectURI:    "https://data.example.org/ns/LA/",
		ProjectPrefix: "la",
		Bindings: []generators.NamespaceBinding{
			{Prefix: "crm", Namespace: "http://www.cidoc-crm.org/cidoc-crm/"},
			{Prefix: "crmdig", Namespace: "http://www.ics.forth.gr/isl/CRMdig/"},
			{Prefix: "la", Namespace: "https://data.example.org/ns/LA/"},
		},
	}
	snap := generators.BuildFieldSnapshot(generators.FieldSnapshotInput{
		Project:    domain.Project{Entity: domain.Entity{ID: "LA"}, Namespace: "https://data.example.org/ns/LA/"},
		Field:      field,
		Resolved:   resolved,
		Namespaces: ns,
	})

	var buf bytes.Buffer
	if err := NewRenderer().Render(context.Background(), snap, &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "?subject a crmdig:D1_Digital_Object, crm:E36_Visual_Item .") {
		t.Fatalf("expected dual rdf:type on ?subject, got:\n%s", got)
	}
	if !strings.Contains(got, "PREFIX crmdig:") {
		t.Fatalf("expected crmdig PREFIX declared, got:\n%s", got)
	}
}

func TestSPARQLRenderResourceTerminalAddsLabelOptional(t *testing.T) {
	field := domain.Field{
		Entity:        domain.Entity{ID: "f1", SemanticID: "LAF.1", SystemName: "actor_name"},
		OntologyScope: pe("class", "crm", "E21_Person", 0),
	}
	resolved := domain.ResolvedField{
		ID:                "f1",
		SemanticID:        "LAF.1",
		SystemName:        "actor_name",
		ExpectedValueType: "Concept",
		PathElements: []domain.PathElement{
			pe("class", "crm", "E21_Person", 0),
			pe("property", "crm", "P2_has_type", 1),
			pe("class", "crm", "E55_Type", 2),
		},
	}
	snap := generators.BuildFieldSnapshot(generators.FieldSnapshotInput{
		Project:    domain.Project{Entity: domain.Entity{ID: "LA"}, Namespace: "https://data.example.org/ns/LA/"},
		Field:      field,
		Resolved:   resolved,
		Namespaces: crmNamespaces(),
	})

	var buf bytes.Buffer
	if err := NewRenderer().Render(context.Background(), snap, &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(buf.String(), "OPTIONAL { ?LAF_1_actor_name rdfs:label ?LAF_1_actor_name_label . }") {
		t.Errorf("expected rdfs:label OPTIONAL clause for resource terminal, got:\n%s", buf.String())
	}
}

func TestSPARQLRenderCount(t *testing.T) {
	field := domain.Field{
		Entity:        domain.Entity{ID: "f1", SemanticID: "LAF.1", SystemName: "name"},
		OntologyScope: pe("class", "crm", "E21_Person", 0),
	}
	resolved := domain.ResolvedField{
		ID:                "f1",
		SemanticID:        "LAF.1",
		SystemName:        "name",
		ExpectedValueType: "string",
		PathElements: []domain.PathElement{
			pe("class", "crm", "E21_Person", 0),
			pe("property", "crm", "P1_is_identified_by", 1),
			{Type: "literal", Prefix: "rdf", LocalName: "langString", URI: "rdf:langString", Position: 2},
		},
	}
	snap := generators.BuildFieldSnapshot(generators.FieldSnapshotInput{
		Project:    domain.Project{Entity: domain.Entity{ID: "LA"}, Namespace: "https://data.example.org/ns/LA/"},
		Field:      field,
		Resolved:   resolved,
		Namespaces: crmNamespaces(),
		Options:    generators.Options{SPARQLCount: true},
	})

	var buf bytes.Buffer
	if err := NewRenderer().Render(context.Background(), snap, &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "SELECT (COUNT(?LAF_1_name) AS ?count) WHERE {") {
		t.Errorf("expected COUNT projection, got:\n%s", got)
	}
	if strings.Contains(got, "LIMIT ") {
		t.Errorf("count query should not emit LIMIT, got:\n%s", got)
	}
}

func TestSPARQLRenderLimit(t *testing.T) {
	field := domain.Field{
		Entity:        domain.Entity{ID: "f1", SemanticID: "LAF.1", SystemName: "name"},
		OntologyScope: pe("class", "crm", "E21_Person", 0),
	}
	resolved := domain.ResolvedField{
		ID:                "f1",
		SemanticID:        "LAF.1",
		SystemName:        "name",
		ExpectedValueType: "string",
		PathElements: []domain.PathElement{
			pe("class", "crm", "E21_Person", 0),
			pe("property", "crm", "P1_is_identified_by", 1),
			{Type: "literal", Prefix: "rdf", LocalName: "langString", URI: "rdf:langString", Position: 2},
		},
	}
	snap := generators.BuildFieldSnapshot(generators.FieldSnapshotInput{
		Project:    domain.Project{Entity: domain.Entity{ID: "LA"}, Namespace: "https://data.example.org/ns/LA/"},
		Field:      field,
		Resolved:   resolved,
		Namespaces: crmNamespaces(),
		Options:    generators.Options{SPARQLLimit: 50},
	})

	var buf bytes.Buffer
	if err := NewRenderer().Render(context.Background(), snap, &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(buf.String(), "LIMIT 50") {
		t.Errorf("expected LIMIT 50, got:\n%s", buf.String())
	}
}

func TestSPARQLRenderCollectionScopeEmitsOnePerField(t *testing.T) {
	snap := generators.BuildCollectionSnapshot(generators.CollectionSnapshotInput{
		Project: domain.Project{Entity: domain.Entity{ID: "LA"}, Namespace: "https://data.example.org/ns/LA/"},
		Collection: domain.Collection{
			Entity:        domain.Entity{ID: "c1", SemanticID: "LAC.1", SystemName: "name"},
			OntologyScope: pe("class", "crm", "E1_CRM_Entity", 0),
		},
		Fields: []domain.ResolvedField{
			{
				ID: "f-content", SemanticID: "LAF.1", SystemName: "name_content",
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

	if c := strings.Count(got, "SELECT DISTINCT "); c != 2 {
		t.Errorf("expected 2 SELECT statements (one per field), got %d:\n%s", c, got)
	}
	for _, v := range []string{"?LAF_1_name_content", "?LAF_2_name_type"} {
		if !strings.Contains(got, "SELECT DISTINCT "+v+" WHERE {") {
			t.Errorf("expected per-field value var %q, got:\n%s", v, got)
		}
	}
	if c := strings.Count(got, "PREFIX crm:"); c != 1 {
		t.Errorf("expected single PREFIX block, got %d:\n%s", c, got)
	}
}

func TestSPARQLRenderSetValueFiltersTerminal(t *testing.T) {
	field := domain.Field{
		Entity:        domain.Entity{ID: "f1", SemanticID: "LAF.1", SystemName: "name"},
		OntologyScope: pe("class", "crm", "E21_Person", 0),
	}
	resolved := domain.ResolvedField{
		ID:                "f1",
		SemanticID:        "LAF.1",
		SystemName:        "name",
		ExpectedValueType: "Concept",
		SetValue:          "http://vocab.example.org/types/biographical",
		PathElements: []domain.PathElement{
			pe("class", "crm", "E21_Person", 0),
			pe("property", "crm", "P2_has_type", 1),
			pe("class", "crm", "E55_Type", 2),
		},
	}
	snap := generators.BuildFieldSnapshot(generators.FieldSnapshotInput{
		Project:    domain.Project{Entity: domain.Entity{ID: "LA"}, Namespace: "https://data.example.org/ns/LA/"},
		Field:      field,
		Resolved:   resolved,
		Namespaces: crmNamespaces(),
	})

	var buf bytes.Buffer
	if err := NewRenderer().Render(context.Background(), snap, &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	want := "?LAF_1_name <http://www.cidoc-crm.org/cidoc-crm/P2_has_type> <http://vocab.example.org/types/biographical> ."
	if !strings.Contains(buf.String(), want) {
		t.Errorf("expected Set_Value type filter %q, got:\n%s", want, buf.String())
	}
}

func TestSPARQLRenderMissingPrefixIsFatal(t *testing.T) {
	field := domain.Field{
		Entity:        domain.Entity{ID: "f1", SemanticID: "LAF.1", SystemName: "name"},
		OntologyScope: pe("class", "frbroo", "F2_Expression", 0),
	}
	resolved := domain.ResolvedField{
		ID:                "f1",
		SemanticID:        "LAF.1",
		SystemName:        "name",
		ExpectedValueType: "string",
		PathElements: []domain.PathElement{
			pe("class", "frbroo", "F2_Expression", 0),
			pe("property", "frbroo", "R3_is_realised_in", 1),
			{Type: "literal", Prefix: "rdf", LocalName: "langString", URI: "rdf:langString", Position: 2},
		},
	}
	snap := generators.BuildFieldSnapshot(generators.FieldSnapshotInput{
		Project:    domain.Project{Entity: domain.Entity{ID: "LA"}, Namespace: "https://data.example.org/ns/LA/"},
		Field:      field,
		Resolved:   resolved,
		Namespaces: crmNamespaces(), // omits frbroo on purpose
	})

	var buf bytes.Buffer
	err := NewRenderer().Render(context.Background(), snap, &buf)
	if err == nil {
		t.Fatal("expected missing-prefix error, got nil")
	}
	if !strings.Contains(err.Error(), "missing namespace binding") {
		t.Errorf("expected missing-namespace error, got %v", err)
	}
}
