package sparql

import (
	"bytes"
	"context"
	"regexp"
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
	// A collection's scope is its identity: ?subject carries every class.
	scope := pe("class", "crmdig", "D1_Digital_Object", 0)
	scope.AdditionalTypes = []domain.TypeRef{
		{URI: "crm:E36_Visual_Item", Prefix: "crm", LocalName: "E36_Visual_Item", ClassCode: "E36"},
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
	snap := generators.BuildCollectionSnapshot(generators.CollectionSnapshotInput{
		Project: domain.Project{Entity: domain.Entity{ID: "LA"}, Namespace: "https://data.example.org/ns/LA/"},
		Collection: domain.Collection{
			Entity:        domain.Entity{ID: "c1", SemanticID: "LAC.1", SystemName: "image"},
			OntologyScope: scope,
		},
		Fields: []domain.ResolvedField{{
			ID: "f1", SemanticID: "LAF.1", SystemName: "label",
			ExpectedValueType: "string",
			PathElements: []domain.PathElement{
				pe("property", "rdfs", "label", 0),
				{Type: "literal", Prefix: "rdf", LocalName: "langString", URI: "rdf:langString", Position: 1},
			},
		}},
		Namespaces: ns,
	})

	got := render(t, snap)
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
	// A set value ends the path on the value itself: the last property's
	// object is the IRI, not a variable with a P2_has_type filter on it.
	got := render(t, fieldSnapshot(t, fieldWithSetValue("http://vocab.getty.edu/aat/300055863")))

	for _, line := range []string{
		"SELECT DISTINCT ?e55_type_1 WHERE {",
		"?subject crm:P2_has_type ?e55_type_1 .",
		"?e55_type_1 a crm:E55_Type .",
		"?e55_type_1 crm:P2_has_type <http://vocab.getty.edu/aat/300055863> .",
	} {
		if !strings.Contains(got, line) {
			t.Errorf("output missing %q\nGOT:\n%s", line, got)
		}
	}
	for _, bad := range []string{"?LAF_12_type crm:P2_has_type", "?value crm:P2_has_type", "cidoc-crm/P2_has_type>", "rdfs:label"} {
		if strings.Contains(got, bad) {
			t.Errorf("output contains %q\nGOT:\n%s", bad, got)
		}
	}
}

func TestSPARQLRenderSetValueCURIE(t *testing.T) {
	got := render(t, fieldSnapshot(t, fieldWithSetValue("la:biographical")))
	if !strings.Contains(got, "?e55_type_1 crm:P2_has_type <https://data.example.org/ns/LA/biographical> .") {
		t.Fatalf("CURIE set value not expanded:\n%s", got)
	}
}

func TestSPARQLRenderHostileNamesStayInert(t *testing.T) {
	snap := fieldSnapshot(t, fieldWithSystemName("x\" } # <img src=x onerror=alert(1)>\nDROP"))
	out := render(t, snap)
	if strings.Contains(out, "\nDROP") || strings.Contains(out, "<img") || strings.Contains(out, `" }`) {
		t.Fatalf("hostile name leaked into the query:\n%s", out)
	}
	if !regexp.MustCompile(`\?LAF_1_x_+`).MatchString(out) {
		t.Fatalf("value variable not sanitised:\n%s", out)
	}
}

func TestSPARQLRenderRefusesNonIRISetValue(t *testing.T) {
	_, err := renderErr(fieldSnapshot(t, fieldWithSetValue("not an iri>")))
	if err == nil || !strings.Contains(err.Error(), "is not an IRI") {
		t.Fatalf("err = %v", err)
	}
}

func TestSPARQLRenderRefusesUnboundCURIESetValue(t *testing.T) {
	_, err := renderErr(fieldSnapshot(t, fieldWithSetValue("aat:300055863")))
	if err == nil || !strings.Contains(err.Error(), "is not an IRI") {
		t.Fatalf("err = %v", err)
	}
}

func TestSPARQLRenderNoClassCheckOnFieldSubject(t *testing.T) {
	out := render(t, fieldSnapshot(t, nameTypeField()))
	if strings.Contains(out, "?subject a ") {
		t.Fatalf("field query types ?subject:\n%s", out)
	}
	// the node in between is typed, the last node is not
	if !strings.Contains(out, "?e33_e41_linguistic_appellation_1 a crm:E33_E41_Linguistic_Appellation .") {
		t.Fatalf("intermediate node not typed:\n%s", out)
	}
	if strings.Contains(out, "a crm:E55_Type") {
		t.Fatalf("last node typed:\n%s", out)
	}
}

func TestSPARQLRenderSharedNodeSharesVariable(t *testing.T) {
	snap := generators.BuildCollectionSnapshot(generators.CollectionSnapshotInput{
		Project: domain.Project{Entity: domain.Entity{ID: "LA"}, Namespace: "https://data.example.org/ns/LA/"},
		Collection: domain.Collection{
			Entity:        domain.Entity{ID: "c1", SemanticID: "LAC.1", SystemName: "name"},
			OntologyScope: pe("class", "crm", "E1_CRM_Entity", 0),
		},
		Fields: []domain.ResolvedField{
			{
				ID: "f-content", SemanticID: "LAF.1", SystemName: "content", ExpectedValueType: "string",
				PathElements: []domain.PathElement{
					pe("property", "crm", "P1_is_identified_by", 0),
					pe("class", "crm", "E33_E41_Linguistic_Appellation", 1),
					pe("property", "crm", "P190_has_symbolic_content", 2),
					{Type: "literal", Prefix: "rdf", LocalName: "langString", URI: "rdf:langString", Position: 3},
				},
			},
			{
				ID: "f-type", SemanticID: "LAF.2", SystemName: "type", ExpectedValueType: "Concept",
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
	got := render(t, snap)
	if n := strings.Count(got, "?subject crm:P1_is_identified_by ?e33_e41_linguistic_appellation_1 ."); n != 2 {
		t.Fatalf("shared node variable used %d times, want 2:\n%s", n, got)
	}
	if n := strings.Count(got, "?subject a crm:E1_CRM_Entity ."); n != 2 {
		t.Fatalf("collection scope typed %d times, want 2:\n%s", n, got)
	}
}

func TestSPARQLVar(t *testing.T) {
	for in, want := range map[string]string{
		"LAF.1_name":  "LAF_1_name",
		"a-b c:d":     "a_b_c_d",
		"1abc":        "v_1abc",
		"":            "v",
		"naïve\x00x}": "na_ve_x_",
	} {
		if got := sparqlVar(in); got != want {
			t.Errorf("sparqlVar(%q) = %q, want %q", in, got, want)
		}
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

func render(t *testing.T, snap *generators.Snapshot) string {
	t.Helper()
	out, err := renderErr(snap)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	return out
}

func renderErr(snap *generators.Snapshot) (string, error) {
	var buf bytes.Buffer
	err := NewRenderer().Render(context.Background(), snap, &buf)
	return buf.String(), err
}

// fieldSnapshot builds a field-level snapshot scoped on the field's first
// path element.
func fieldSnapshot(t *testing.T, rf domain.ResolvedField) *generators.Snapshot {
	t.Helper()
	snap := generators.BuildFieldSnapshot(generators.FieldSnapshotInput{
		Project: domain.Project{Entity: domain.Entity{ID: "LA"}, Namespace: "https://data.example.org/ns/LA/"},
		Field: domain.Field{
			Entity:        domain.Entity{ID: rf.ID, SemanticID: rf.SemanticID, SystemName: rf.SystemName},
			OntologyScope: rf.PathElements[0],
		},
		Resolved:   rf,
		Namespaces: crmNamespaces(),
	})
	if len(snap.Report.Errors) > 0 {
		t.Fatalf("snapshot errors: %v", snap.Report.Errors)
	}
	return snap
}

// fieldWithSystemName is a name-content field (literal terminal).
func fieldWithSystemName(systemName string) domain.ResolvedField {
	return domain.ResolvedField{
		ID: "f1", SemanticID: "LAF.1", SystemName: systemName, ExpectedValueType: "string",
		PathElements: []domain.PathElement{
			pe("class", "crm", "E21_Person", 0),
			pe("property", "crm", "P1_is_identified_by", 1),
			pe("class", "crm", "E33_E41_Linguistic_Appellation", 2),
			pe("property", "crm", "P190_has_symbolic_content", 3),
			{Type: "literal", Prefix: "rdf", LocalName: "langString", URI: "rdf:langString", Position: 4},
		},
	}
}

// fieldWithSetValue is a metatype field: a type of a type, fixed by a set
// value.
func fieldWithSetValue(v string) domain.ResolvedField {
	return domain.ResolvedField{
		ID: "f12", SemanticID: "LAF.12", SystemName: "type", ExpectedValueType: "Concept",
		SetValue: v,
		PathElements: []domain.PathElement{
			pe("class", "crm", "E1_CRM_Entity", 0),
			pe("property", "crm", "P2_has_type", 1),
			pe("class", "crm", "E55_Type", 2),
			pe("property", "crm", "P2_has_type", 3),
			pe("class", "crm", "E55_Type", 4),
		},
	}
}

// nameTypeField is a name-type field (resource terminal behind a node).
func nameTypeField() domain.ResolvedField {
	return domain.ResolvedField{
		ID: "f2", SemanticID: "LAF.2", SystemName: "name_type", ExpectedValueType: "Concept",
		PathElements: []domain.PathElement{
			pe("class", "crm", "E21_Person", 0),
			pe("property", "crm", "P1_is_identified_by", 1),
			pe("class", "crm", "E33_E41_Linguistic_Appellation", 2),
			pe("property", "crm", "P2_has_type", 3),
			pe("class", "crm", "E55_Type", 4),
		},
	}
}

func TestSPARQLRenderRefusesSetValueThroughHostileNamespace(t *testing.T) {
	snap := fieldSnapshot(t, fieldWithSetValue("x:val"))
	snap.Namespaces.Bindings = append(snap.Namespaces.Bindings,
		generators.NamespaceBinding{Prefix: "x", Namespace: "https://data.example.org/x> } #"})
	out, err := renderErr(snap)
	if err == nil || !strings.Contains(err.Error(), "is not an IRI") {
		t.Fatalf("err = %v", err)
	}
	if out != "" {
		t.Fatalf("rendered despite error:\n%s", out)
	}
}

func TestSPARQLRenderRefusesPathPastLiteral(t *testing.T) {
	rf := domain.ResolvedField{
		ID: "f1", SemanticID: "LAF.9", SystemName: "odd", ExpectedValueType: "Concept",
		PathElements: []domain.PathElement{
			pe("class", "crm", "E21_Person", 0),
			pe("property", "crm", "P3_has_note", 1),
			{Type: "literal", Prefix: "rdf", LocalName: "langString", URI: "rdf:langString", Position: 2},
			pe("property", "crm", "P2_has_type", 3),
			pe("class", "crm", "E55_Type", 4),
		},
	}
	_, err := renderErr(fieldSnapshot(t, rf))
	if err == nil || err.Error() != "sparql renderer: field LAF.9: path continues after a literal" {
		t.Fatalf("err = %v", err)
	}
}

func TestSPARQLRenderRefusesCountVariableCollision(t *testing.T) {
	rf := fieldWithSystemName("")
	rf.SemanticID = "count"
	snap := fieldSnapshot(t, rf)
	snap.Options.SPARQLCount = true
	_, err := renderErr(snap)
	if err == nil || err.Error() != "sparql renderer: field count: variable ?count collides" {
		t.Fatalf("err = %v", err)
	}
}

func TestSPARQLRenderRefusesNodeVariableCollision(t *testing.T) {
	rf := fieldWithSetValue("")
	rf.SemanticID, rf.SystemName = "e55_type", "1"
	_, err := renderErr(fieldSnapshot(t, rf))
	if err == nil || err.Error() != "sparql renderer: field e55_type: variable ?e55_type_1 collides" {
		t.Fatalf("err = %v", err)
	}
}

// modelSnapshot builds a model snapshot with the fields placed directly
// under one category.
func modelSnapshot(t *testing.T, fields ...domain.ResolvedField) *generators.Snapshot {
	t.Helper()
	snap := generators.BuildModelSnapshot(generators.ModelSnapshotInput{
		Project: domain.Project{Entity: domain.Entity{ID: "LA"}, Namespace: "https://data.example.org/ns/LA/"},
		Model: domain.Model{
			Entity:        domain.Entity{ID: "m1", SemanticID: "LAM.1", SystemName: "person"},
			OntologyScope: pe("class", "crm", "E21_Person", 0),
		},
		View: domain.ModelView{Categories: []domain.CategoryGroup{{
			ID:          "cat",
			Collections: []domain.CollectionGroup{{ID: "__direct__", Fields: fields}},
		}}},
		Namespaces: crmNamespaces(),
	})
	if len(snap.Report.Errors) > 0 {
		t.Fatalf("snapshot errors: %v", snap.Report.Errors)
	}
	return snap
}

func TestSPARQLRenderModelSkipsRefusedField(t *testing.T) {
	bad := fieldWithSetValue("pletka:concept/01M5HDTNR3Q5Z7SBYF94RGEYZF")
	bad.PathElements[0] = pe("class", "crm", "E21_Person", 0)
	snap := modelSnapshot(t, fieldWithSystemName("name"), bad, nameTypeField())
	out, err := renderErr(snap)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if c := strings.Count(out, "SELECT DISTINCT "); c != 2 {
		t.Errorf("SELECT count = %d, want 2:\n%s", c, out)
	}
	want := "# field LAF.12 skipped: set value \"pletka:concept/01M5HDTNR3Q5Z7SBYF94RGEYZF\" is not an IRI\n"
	if !strings.Contains(out, want) {
		t.Errorf("missing skip comment %q:\n%s", want, out)
	}
}

func TestSPARQLRenderSkipCommentStaysOneLine(t *testing.T) {
	bad := fieldWithSetValue("x\ny")
	bad.SemanticID = "LAF.12\nSELECT"
	bad.PathElements[0] = pe("class", "crm", "E21_Person", 0)
	out := render(t, modelSnapshot(t, bad))
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "SELECT") {
			t.Fatalf("skip comment broke a line:\n%s", out)
		}
	}
	if !strings.Contains(out, "# field LAF.12 SELECT skipped: ") {
		t.Fatalf("missing skip comment:\n%s", out)
	}
}

func TestSPARQLRenderFieldLevelRefusalStillErrors(t *testing.T) {
	_, err := renderErr(fieldSnapshot(t, fieldWithSetValue("pletka:concept/01M5HDTNR3Q5Z7SBYF94RGEYZF")))
	if err == nil || !strings.Contains(err.Error(), "is not an IRI") {
		t.Fatalf("err = %v", err)
	}
}
