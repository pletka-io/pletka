package x3ml

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
			{Prefix: "rdf", Namespace: "http://www.w3.org/1999/02/22-rdf-syntax-ns#"},
		},
	}
}

func TestX3MLRendererSpecs(t *testing.T) {
	if got := NewRendererA().Spec().Format; got != generators.FormatX3ML {
		t.Errorf("form A format = %q, want %q", got, generators.FormatX3ML)
	}
	if got := NewRendererA().Spec().FileExtension; got != ".a.x3ml" {
		t.Errorf("form A extension = %q", got)
	}
	if got := NewRendererB().Spec().Format; got != generators.FormatX3MLB {
		t.Errorf("form B format = %q, want %q", got, generators.FormatX3MLB)
	}
	if got := NewRendererB().Spec().FileExtension; got != ".b.x3ml" {
		t.Errorf("form B extension = %q", got)
	}
}

func collectionSnap(t *testing.T) *generators.Snapshot {
	t.Helper()
	return generators.BuildCollectionSnapshot(generators.CollectionSnapshotInput{
		Project: domain.Project{
			Entity:    domain.Entity{ID: "LA", SystemName: "la_project", UIName: domain.Translations{"en": "LA"}},
			Namespace: "https://data.example.org/ns/LA/",
		},
		Collection: domain.Collection{
			Entity:        domain.Entity{ID: "c-name", SemanticID: "LAC.1", SystemName: "name"},
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
}

func TestX3MLFormARendersRootDomainAndFullPathLinks(t *testing.T) {
	snap := collectionSnap(t)
	var buf bytes.Buffer
	if err := NewRendererA().Render(context.Background(), snap, &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	got := buf.String()

	// Intermediate appellation node shared by both fields:
	// the variable is the generated path_node_id, identical for every
	// field reaching the node through the same prefix.
	sharedAppellation := `e33_e41_linguistic_appellation_1`

	for _, want := range []string{
		`<?xml version="1.0" encoding="UTF-8"?>`,
		`<x3ml`,
		`xsi:noNamespaceSchemaLocation="x3ml_v1.5.xsd"`,
		`<namespaces>`,
		`<namespace prefix="crm" uri="http://www.cidoc-crm.org/cidoc-crm/"`,
		`<mappings>`,
		// Templates keep the dot — 3M renders them as the human label.
		`<domain template="LAC.1_name">`,
		`<type>crm:E1_CRM_Entity</type>`,
		`<link template="LAF.1_name_content">`,
		`<relationship>crm:P1_is_identified_by</relationship>`,
		`<entity variable="` + sharedAppellation + `"`,
		`<type>crm:E33_E41_Linguistic_Appellation</type>`,
		`<relationship>crm:P190_has_symbolic_content</relationship>`,
		`<range>`,
		`<type>http://www.w3.org/2001/XMLSchema#string</type>`,
		`<arg name="text" type="xpath">text()</arg>`,
		`<arg name="language" type="constant">en</arg>`,
		`<link template="LAF.2_name_type">`,
		`<relationship>crm:P2_has_type</relationship>`,
		// Terminal class node — variable is its own path_node_id.
		`<entity variable="e55_type_1"`,
		`<type>crm:E55_Type</type>`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("form A output missing %q", want)
		}
	}
	// Both fields pass through the same appellation node, so the shared
	// variable must appear exactly twice (once per <link>) — proves spine
	// coreference rather than two separate per-field nodes.
	if c := strings.Count(got, `variable="`+sharedAppellation+`"`); c != 2 {
		t.Errorf("expected shared appellation variable twice, got %d\n%s", c, got)
	}
	// Form A must produce exactly one <mapping>.
	if c := strings.Count(got, "<mapping>"); c != 1 {
		t.Errorf("form A: expected 1 mapping, got %d\n%s", c, got)
	}
}

func TestX3MLFormATerminalLiteralUsesXSDString(t *testing.T) {
	snap := generators.BuildFieldSnapshot(generators.FieldSnapshotInput{
		Project: domain.Project{
			Entity:    domain.Entity{ID: "LA", UIName: domain.Translations{"en": "LA"}},
			Namespace: "https://data.example.org/ns/LA/",
		},
		Field: domain.Field{
			Entity:        domain.Entity{ID: "f1", SemanticID: "LAF.7", SystemName: "label"},
			OntologyScope: pe("class", "crm", "E21_Person", 0),
		},
		Resolved: domain.ResolvedField{
			ID: "f1", SemanticID: "LAF.7", SystemName: "label",
			ExpectedValueType: "string",
			PathElements: []domain.PathElement{
				pe("class", "crm", "E21_Person", 0),
				pe("property", "rdfs", "label", 1),
				{Type: "literal", Prefix: "rdf", LocalName: "literal", URI: "rdf:literal", Position: 2},
			},
		},
		Namespaces: crmNamespaces(),
	})

	var buf bytes.Buffer
	if err := NewRendererA().Render(context.Background(), snap, &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "http://www.w3.org/2001/XMLSchema#string") {
		t.Errorf("expected xsd:string literal type, got:\n%s", got)
	}
	if !strings.Contains(got, `<arg name="text" type="xpath">text()</arg>`) {
		t.Errorf("expected literal instance generator args, got:\n%s", got)
	}
}

func TestX3MLFormAEmitsLinkPerSubfieldPath(t *testing.T) {
	// SRD1F.5 shape — one primary link + one legacy subfield link.
	snap := generators.BuildFieldSnapshot(generators.FieldSnapshotInput{
		Project: domain.Project{
			Entity:    domain.Entity{ID: "LA", UIName: domain.Translations{"en": "LA"}},
			Namespace: "https://data.example.org/ns/LA/",
		},
		Field: domain.Field{
			Entity:        domain.Entity{ID: "f1", SemanticID: "SRD1F.5", SystemName: "name"},
			OntologyScope: pe("class", "crm", "E21_Person", 0),
		},
		Resolved: domain.ResolvedField{
			ID: "f1", SemanticID: "SRD1F.5", SystemName: "name",
			ExpectedValueType: "string",
			PathElements: []domain.PathElement{
				pe("class", "crm", "E21_Person", 0),
				pe("property", "crm", "P1_is_identified_by", 1),
				pe("class", "crm", "E33_E41_Linguistic_Appellation", 2),
				pe("property", "crm", "P190_has_symbolic_content", 3),
				{Type: "literal", Prefix: "rdf", LocalName: "literal", URI: "rdf:literal", Position: 4},
			},
			SubfieldPaths: []domain.SubfieldPath{{
				Source: "legacy-br-split",
				PathElements: []domain.PathElement{
					pe("property", "crm", "P1_is_identified_by", 0),
					pe("class", "crm", "E33_E41_Linguistic_Appellation", 1),
					pe("property", "crm", "P2_has_type", 2),
					pe("class", "crm", "E55_Type", 3),
				},
			}},
		},
		Namespaces: crmNamespaces(),
	})

	var buf bytes.Buffer
	if err := NewRendererA().Render(context.Background(), snap, &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	got := buf.String()
	for _, want := range []string{
		`<link template="SRD1F.5_name">`,    // primary
		`<link template="SRD1F.5_name_p1">`, // legacy subfield
		"P2_has_type",                       // subfield-only property emitted
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q\nGOT:\n%s", want, got)
		}
	}
}

func TestX3MLEmitsMappingsBlockEvenWhenNoFields(t *testing.T) {
	snap := generators.BuildCollectionSnapshot(generators.CollectionSnapshotInput{
		Project: domain.Project{
			Entity:    domain.Entity{ID: "LA"},
			Namespace: "https://data.example.org/ns/LA/",
		},
		Collection: domain.Collection{
			Entity:        domain.Entity{ID: "c-empty", SemanticID: "LAC.99", SystemName: "empty"},
			OntologyScope: pe("class", "crm", "E1_CRM_Entity", 0),
		},
		Namespaces: crmNamespaces(),
	})
	var buf bytes.Buffer
	if err := NewRendererA().Render(context.Background(), snap, &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "<mappings>") {
		t.Errorf("expected empty <mappings> element, got:\n%s", got)
	}
}

// The <info> skeleton must always be present in full — 3M's importer
// rejects documents missing any of these elements.
func TestX3MLEmitsFullInfoSkeleton(t *testing.T) {
	snap := generators.BuildCollectionSnapshot(generators.CollectionSnapshotInput{
		Project: domain.Project{
			Entity:    domain.Entity{ID: "LA", UIName: domain.Translations{"en": "LA"}},
			Namespace: "https://data.example.org/ns/LA/",
		},
		Collection: domain.Collection{
			Entity:        domain.Entity{ID: "c-empty", SemanticID: "LAC.99", SystemName: "empty"},
			OntologyScope: pe("class", "crm", "E1_CRM_Entity", 0),
		},
		Namespaces: crmNamespaces(),
	})
	var buf bytes.Buffer
	if err := NewRendererA().Render(context.Background(), snap, &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	got := buf.String()
	for _, want := range []string{
		`version="1.0"`,
		`<title>LAC.99 - empty</title>`,
		`<general_description>`,
		`<source><source_info><source_schema type="" version=""></source_schema><namespaces><namespace prefix="" uri=""></namespace></namespaces></source_info><source_collection></source_collection></source>`,
		`<target><target_info><target_schema type="rdfs" version="7.1.1">CIDOC CRM</target_schema>`,
		`</target_info><target_collection></target_collection></target>`,
		`<mapping_info><mapping_created_by_org></mapping_created_by_org>`,
		`<example_data_info><example_data_from></example_data_from>`,
		`<thesaurus_info></thesaurus_info>`,
	} {
		if !strings.Contains(spaceless(got), spaceless(want)) {
			t.Errorf("info skeleton missing %q\n%s", want, got)
		}
	}
}

// spaceless strips whitespace between tags so assertions ignore the
// encoder's indentation.
func spaceless(s string) string {
	var b strings.Builder
	for _, line := range strings.Split(s, "\n") {
		b.WriteString(strings.TrimSpace(line))
	}
	return b.String()
}

// modelSnapshotForFormB builds a Person model snapshot: one category
// holding the LAC.1 "Name" collection (two fields) plus one standalone
// field placed directly on the model.
func modelSnapshotForFormB(t *testing.T) *generators.Snapshot {
	t.Helper()
	nameContent := domain.ResolvedField{
		ID: "f6", SemanticID: "LAF.6", SystemName: "name_content",
		ExpectedValueType: "string",
		PathElements: []domain.PathElement{
			pe("property", "crm", "P1_is_identified_by", 0),
			pe("class", "crm", "E33_E41_Linguistic_Appellation", 1),
			pe("property", "crm", "P190_has_symbolic_content", 2),
			{Type: "literal", Prefix: "rdf", LocalName: "langString", URI: "rdf:langString", Position: 3},
		},
	}
	nameType := domain.ResolvedField{
		ID: "f5", SemanticID: "LAF.5", SystemName: "name_type",
		ExpectedValueType: "Concept",
		PathElements: []domain.PathElement{
			pe("property", "crm", "P1_is_identified_by", 0),
			pe("class", "crm", "E33_E41_Linguistic_Appellation", 1),
			pe("property", "crm", "P2_has_type", 2),
			pe("class", "crm", "E55_Type", 3),
		},
	}
	internalLabel := domain.ResolvedField{
		ID: "f54", SemanticID: "LAF.54", SystemName: "internal_label",
		ExpectedValueType: "string",
		PathElements: []domain.PathElement{
			pe("property", "rdfs", "label", 0),
			{Type: "literal", Prefix: "rdf", LocalName: "literal", URI: "rdf:literal", Position: 1},
		},
	}
	sharedPrefix := []domain.PathElement{
		pe("property", "crm", "P1_is_identified_by", 0),
		pe("class", "crm", "E33_E41_Linguistic_Appellation", 1),
	}
	return generators.BuildModelSnapshot(generators.ModelSnapshotInput{
		Project: domain.Project{
			Entity:    domain.Entity{ID: "LA", SystemName: "la_project", UIName: domain.Translations{"en": "LA"}},
			Namespace: "https://data.example.org/ns/LA/",
		},
		Model: domain.Model{
			Entity:        domain.Entity{ID: "m1", SemanticID: "LAM.1", SystemName: "person", UIName: domain.Translations{"en": "Person"}},
			OntologyScope: pe("class", "crm", "E21_Person", 0),
		},
		View: domain.ModelView{
			ModelID: "m1", ProjectID: "LA",
			Categories: []domain.CategoryGroup{{
				ID: "cat1", Name: domain.Translations{"en": "Identity"}, Position: 0,
				Collections: []domain.CollectionGroup{
					{
						ID: "c-name", Name: domain.Translations{"en": "Name"}, Position: 0,
						SharedPathPrefix: sharedPrefix,
						Fields:           []domain.ResolvedField{nameContent, nameType},
					},
					{
						ID:     "__direct__",
						Fields: []domain.ResolvedField{internalLabel},
					},
				},
			}},
		},
		Collections: map[string]*domain.Collection{
			"c-name": {
				Entity:        domain.Entity{ID: "c-name", SemanticID: "LAC.1", SystemName: "name", UIName: domain.Translations{"en": "Name"}},
				OntologyScope: pe("class", "crm", "E33_E41_Linguistic_Appellation", 0),
			},
		},
		Namespaces: crmNamespaces(),
	})
}

func TestX3MLFormBBucketsByCollection(t *testing.T) {
	snap := modelSnapshotForFormB(t)
	var buf bytes.Buffer
	if err := NewRendererB().Render(context.Background(), snap, &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	got := buf.String()

	// One primary mapping + one secondary mapping for the Name collection.
	if c := strings.Count(got, "<mapping>"); c != 2 {
		t.Errorf("form B: expected 2 mappings, got %d\n%s", c, got)
	}
	for _, want := range []string{
		// Primary mapping anchors on the model.
		`<domain template="LAM.1_person">`,
		`<type>crm:E21_Person</type>`,
		// Collection placement link in the primary mapping.
		`<link template="LAC.1_name_1">`,
		`<relationship>crm:P1_is_identified_by</relationship>`,
		// Standalone field is a full-path link in the primary mapping.
		`<link template="LAF.54_internal_label">`,
		`<relationship>rdfs:label</relationship>`,
		// Secondary mapping is domain-anchored on the collection scope,
		// joined by the same template string.
		`<domain template="LAC.1_name_1">`,
		`<type>crm:E33_E41_Linguistic_Appellation</type>`,
		// Collection fields live in the secondary mapping.
		`<link template="LAF.6_name_content">`,
		`<relationship>crm:P190_has_symbolic_content</relationship>`,
		`<link template="LAF.5_name_type">`,
		`<relationship>crm:P2_has_type</relationship>`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("form B output missing %q", want)
		}
	}
	// The primary link template and the secondary domain template must
	// match exactly — that string is the join 3M follows.
	if c := strings.Count(got, `template="LAC.1_name_1"`); c != 2 {
		t.Errorf("expected the placement template twice (link + secondary domain), got %d\n%s", c, got)
	}
	// Collection links come first in the primary mapping; standalone
	// (direct) field links follow after every collection link.
	if strings.Index(got, `<link template="LAC.1_name_1">`) > strings.Index(got, `<link template="LAF.54_internal_label">`) {
		t.Errorf("collection link must precede the standalone field link\n%s", got)
	}
}

func TestX3MLFormBNumbersRepeatedCollections(t *testing.T) {
	birthName := domain.ResolvedField{
		ID: "f6", SemanticID: "LAF.6", SystemName: "name_content",
		ExpectedValueType: "string",
		PathElements: []domain.PathElement{
			pe("property", "crm", "P1_is_identified_by", 0),
			pe("class", "crm", "E33_E41_Linguistic_Appellation", 1),
			pe("property", "crm", "P190_has_symbolic_content", 2),
			{Type: "literal", Prefix: "rdf", LocalName: "langString", URI: "rdf:langString", Position: 3},
		},
	}
	sharedPrefix := []domain.PathElement{
		pe("property", "crm", "P1_is_identified_by", 0),
		pe("class", "crm", "E33_E41_Linguistic_Appellation", 1),
	}
	nameGroup := func() domain.CollectionGroup {
		return domain.CollectionGroup{
			ID: "c-name", Name: domain.Translations{"en": "Name"},
			SharedPathPrefix: sharedPrefix,
			Fields:           []domain.ResolvedField{birthName},
		}
	}
	snap := generators.BuildModelSnapshot(generators.ModelSnapshotInput{
		Project: domain.Project{
			Entity:    domain.Entity{ID: "LA", UIName: domain.Translations{"en": "LA"}},
			Namespace: "https://data.example.org/ns/LA/",
		},
		Model: domain.Model{
			Entity:        domain.Entity{ID: "m1", SemanticID: "LAM.1", SystemName: "person"},
			OntologyScope: pe("class", "crm", "E21_Person", 0),
		},
		View: domain.ModelView{
			ModelID: "m1", ProjectID: "LA",
			Categories: []domain.CategoryGroup{
				{ID: "cat1", Name: domain.Translations{"en": "Birth"}, Position: 0,
					Collections: []domain.CollectionGroup{nameGroup()}},
				{ID: "cat2", Name: domain.Translations{"en": "Legal"}, Position: 1,
					Collections: []domain.CollectionGroup{nameGroup()}},
			},
		},
		Collections: map[string]*domain.Collection{
			"c-name": {
				Entity:        domain.Entity{ID: "c-name", SemanticID: "LAC.1", SystemName: "name"},
				OntologyScope: pe("class", "crm", "E33_E41_Linguistic_Appellation", 0),
			},
		},
		Namespaces: crmNamespaces(),
	})
	var buf bytes.Buffer
	if err := NewRendererB().Render(context.Background(), snap, &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	got := buf.String()

	// One primary + two secondary mappings (one per placement).
	if c := strings.Count(got, "<mapping>"); c != 3 {
		t.Errorf("expected 3 mappings, got %d\n%s", c, got)
	}
	for _, want := range []string{
		`<link template="LAC.1_name_1">`,
		`<link template="LAC.1_name_2">`,
		`<domain template="LAC.1_name_1">`,
		`<domain template="LAC.1_name_2">`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q", want)
		}
	}
}

func TestX3MLFormBEmptyCollectionHasNoSecondary(t *testing.T) {
	nameContent := domain.ResolvedField{
		ID: "f6", SemanticID: "LAF.6", SystemName: "name_content",
		ExpectedValueType: "string",
		PathElements: []domain.PathElement{
			pe("property", "crm", "P1_is_identified_by", 0),
			pe("class", "crm", "E33_E41_Linguistic_Appellation", 1),
			pe("property", "crm", "P190_has_symbolic_content", 2),
			{Type: "literal", Prefix: "rdf", LocalName: "langString", URI: "rdf:langString", Position: 3},
		},
	}
	prefix := []domain.PathElement{
		pe("property", "crm", "P1_is_identified_by", 0),
		pe("class", "crm", "E33_E41_Linguistic_Appellation", 1),
	}
	snap := generators.BuildModelSnapshot(generators.ModelSnapshotInput{
		Project: domain.Project{
			Entity:    domain.Entity{ID: "LA", UIName: domain.Translations{"en": "LA"}},
			Namespace: "https://data.example.org/ns/LA/",
		},
		Model: domain.Model{
			Entity:        domain.Entity{ID: "m1", SemanticID: "LAM.1", SystemName: "person"},
			OntologyScope: pe("class", "crm", "E21_Person", 0),
		},
		View: domain.ModelView{
			ModelID: "m1", ProjectID: "LA",
			Categories: []domain.CategoryGroup{{
				ID: "cat1", Name: domain.Translations{"en": "Identity"}, Position: 0,
				Collections: []domain.CollectionGroup{
					{
						ID: "c-name", Name: domain.Translations{"en": "Name"}, Position: 0,
						SharedPathPrefix: prefix,
						Fields:           []domain.ResolvedField{nameContent},
					},
					{
						ID: "c-empty", Name: domain.Translations{"en": "Empty"}, Position: 1,
						SharedPathPrefix: prefix,
						Fields:           nil,
					},
				},
			}},
		},
		Collections: map[string]*domain.Collection{
			"c-name": {
				Entity:        domain.Entity{ID: "c-name", SemanticID: "LAC.1", SystemName: "name"},
				OntologyScope: pe("class", "crm", "E33_E41_Linguistic_Appellation", 0),
			},
			"c-empty": {
				Entity:        domain.Entity{ID: "c-empty", SemanticID: "LAC.9", SystemName: "empty"},
				OntologyScope: pe("class", "crm", "E33_E41_Linguistic_Appellation", 0),
			},
		},
		Namespaces: crmNamespaces(),
	})
	var buf bytes.Buffer
	if err := NewRendererB().Render(context.Background(), snap, &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	got := buf.String()
	// Primary + one secondary (for the populated collection only).
	if c := strings.Count(got, "<mapping>"); c != 2 {
		t.Errorf("expected 2 mappings, got %d\n%s", c, got)
	}
	// Both collections get a primary link.
	if !strings.Contains(got, `<link template="LAC.1_name_1">`) {
		t.Errorf("expected populated collection's primary link\n%s", got)
	}
	if !strings.Contains(got, `<link template="LAC.9_empty_1">`) {
		t.Errorf("expected empty collection's primary link\n%s", got)
	}
	// Only the populated collection gets a secondary mapping.
	if !strings.Contains(got, `<domain template="LAC.1_name_1">`) {
		t.Errorf("expected populated collection's secondary mapping\n%s", got)
	}
	if strings.Contains(got, `<domain template="LAC.9_empty_1">`) {
		t.Errorf("empty collection must not produce a secondary mapping\n%s", got)
	}
}

func TestX3MLFormBSkipsDegeneratePlacement(t *testing.T) {
	orphan := domain.ResolvedField{
		ID: "f1", SemanticID: "LAF.1", SystemName: "orphan",
		ExpectedValueType: "string",
		PathElements: []domain.PathElement{
			pe("property", "crm", "P190_has_symbolic_content", 0),
			{Type: "literal", Prefix: "rdf", LocalName: "literal", URI: "rdf:literal", Position: 1},
		},
	}
	snap := generators.BuildModelSnapshot(generators.ModelSnapshotInput{
		Project: domain.Project{
			Entity:    domain.Entity{ID: "LA", UIName: domain.Translations{"en": "LA"}},
			Namespace: "https://data.example.org/ns/LA/",
		},
		Model: domain.Model{
			Entity:        domain.Entity{ID: "m1", SemanticID: "LAM.1", SystemName: "person"},
			OntologyScope: pe("class", "crm", "E21_Person", 0),
		},
		View: domain.ModelView{
			ModelID: "m1", ProjectID: "LA",
			Categories: []domain.CategoryGroup{{
				ID: "cat1", Name: domain.Translations{"en": "Identity"}, Position: 0,
				Collections: []domain.CollectionGroup{{
					ID: "c-degen", Name: domain.Translations{"en": "Degenerate"}, Position: 0,
					SharedPathPrefix: nil,
					Fields:           []domain.ResolvedField{orphan},
				}},
			}},
		},
		Collections: map[string]*domain.Collection{
			"c-degen": {
				Entity:        domain.Entity{ID: "c-degen", SemanticID: "LAC.8", SystemName: "degenerate"},
				OntologyScope: pe("class", "crm", "E33_E41_Linguistic_Appellation", 0),
			},
		},
		Namespaces: crmNamespaces(),
	})
	var buf bytes.Buffer
	if err := NewRendererB().Render(context.Background(), snap, &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	got := buf.String()
	// A collection placement with no join path (empty PathPrefix) is skipped.
	if c := strings.Count(got, "<mapping>"); c != 1 {
		t.Errorf("expected 1 mapping (primary only), got %d\n%s", c, got)
	}
	if strings.Contains(got, "LAC.8_degenerate") {
		t.Errorf("degenerate placement must be skipped, found its template\n%s", got)
	}
}
