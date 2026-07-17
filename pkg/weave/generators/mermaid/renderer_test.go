package mermaid

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/generators"
)

func TestRendererSpec(t *testing.T) {
	spec := NewRenderer().Spec()

	if spec.Format != generators.FormatMermaid {
		t.Fatalf("format = %q", spec.Format)
	}
	if spec.ContentType != "text/vnd.mermaid; charset=utf-8" {
		t.Fatalf("content type = %q", spec.ContentType)
	}
	if spec.FileExtension != ".mmd" {
		t.Fatalf("extension = %q", spec.FileExtension)
	}
	if !spec.RequiresTree {
		t.Fatal("requires tree = false, want true")
	}
}

func TestRendererRendersLAC1OntologyGraph(t *testing.T) {
	snap := generators.BuildCollectionSnapshot(generators.CollectionSnapshotInput{
		Project: domain.Project{
			Entity:    domain.Entity{ID: "LA"},
			Namespace: "https://data.example.org/ns/LA/",
		},
		Collection: domain.Collection{
			Entity:        domain.Entity{ID: "collection-1", SemanticID: "LAC.1", SystemName: "name"},
			OntologyScope: pe("class", "crm", "E1_CRM_Entity", "E1", 0),
		},
		Fields: []domain.ResolvedField{
			{
				ID:         "field-content",
				SemanticID: "LAF.6",
				SystemName: "name_content",
				Position:   1,
				PathElements: []domain.PathElement{
					pe("property", "crm", "P1_is_identified_by", "", 0),
					peWithInstance("class", "crm", "E33_E41_Linguistic_Appellation", "E33_E41", 1, "4_1"),
					pe("property", "crm", "P190_has_symbolic_content", "", 2),
					{Type: "literal", Prefix: "rdf", LocalName: "literal", URI: "rdf:literal", Datatype: "rdf:literal", Position: 3},
				},
			},
			{
				ID:         "field-type",
				SemanticID: "LAF.5",
				SystemName: "name_type",
				Position:   2,
				PathElements: []domain.PathElement{
					pe("property", "crm", "P1_is_identified_by", "", 0),
					peWithInstance("class", "crm", "E33_E41_Linguistic_Appellation", "E33_E41", 1, "4_1"),
					pe("property", "crm", "P2_has_type", "", 2),
					peWithInstance("class", "crm", "E55_Type", "E55", 3, "5_1"),
				},
			},
			{
				ID:         "field-language",
				SemanticID: "LAF.7",
				SystemName: "name_language",
				Position:   3,
				PathElements: []domain.PathElement{
					pe("property", "crm", "P1_is_identified_by", "", 0),
					peWithInstance("class", "crm", "E33_E41_Linguistic_Appellation", "E33_E41", 1, "4_1"),
					pe("property", "crm", "P72_has_language", "", 2),
					peWithInstance("class", "crm", "E56_Language", "E56", 3, "7_1"),
				},
			},
			{
				ID:         "field-source",
				SemanticID: "LAF.44",
				SystemName: "name_source_reference",
				Position:   4,
				PathElements: []domain.PathElement{
					pe("property", "crm", "P1_is_identified_by", "", 0),
					peWithInstance("class", "crm", "E33_E41_Linguistic_Appellation", "E33_E41", 1, "4_1"),
					pe("property", "crm", "P67i_is_referred_to_by", "", 2),
					peWithInstance("class", "crm", "E33_Linguistic_Object", "E33", 3, "44_1"),
				},
			},
			{
				ID:         "field-label",
				SemanticID: "LAF.4",
				SystemName: "name_label",
				Position:   5,
				PathElements: []domain.PathElement{
					pe("property", "crm", "P1_is_identified_by", "", 0),
					peWithInstance("class", "crm", "E33_E41_Linguistic_Appellation", "E33_E41", 1, "4_1"),
					pe("property", "rdfs", "label", "", 2),
					{Type: "literal", Prefix: "rdf", LocalName: "literal", URI: "rdf:literal", Datatype: "rdf:literal", Position: 3},
				},
			},
			{
				ID:         "field-part",
				SemanticID: "LAF.500",
				SystemName: "name_part",
				Position:   6,
				PathElements: []domain.PathElement{
					pe("property", "crm", "P1_is_identified_by", "", 0),
					peWithInstance("class", "crm", "E33_E41_Linguistic_Appellation", "E33_E41", 1, "4_1"),
					pe("property", "crm", "P106_is_composed_of", "", 2),
					peWithInstance("class", "crm", "E33_E41_Linguistic_Appellation", "E33_E41", 3, "500_1"),
				},
			},
		},
	})

	var buf bytes.Buffer
	if err := NewRenderer().Render(context.Background(), snap, &buf); err != nil {
		t.Fatalf("Render() error: %v", err)
	}

	assertMermaidGolden(t, buf.String(), "testdata/lac1_ontology.mmd")
}

func TestRendererMergesSharedOntologyBranchesWithoutInstanceIDs(t *testing.T) {
	snap := generators.BuildCollectionSnapshot(generators.CollectionSnapshotInput{
		Project: domain.Project{
			Entity:    domain.Entity{ID: "LA"},
			Namespace: "https://data.example.org/ns/LA/",
		},
		Collection: domain.Collection{
			Entity:        domain.Entity{ID: "collection-1", SemanticID: "LAC.1", SystemName: "person"},
			OntologyScope: pe("class", "crm", "E21_Person", "", 0),
		},
		Fields: []domain.ResolvedField{
			{
				ID:         "field-nationality",
				SemanticID: "LAF.1",
				SystemName: "nationality",
				Position:   1,
				PathElements: []domain.PathElement{
					pe("property", "crm", "P2_has_type", "", 0),
					pe("class", "crm", "E55_Type", "", 1),
				},
			},
			{
				ID:         "field-gender",
				SemanticID: "LAF.2",
				SystemName: "gender",
				Position:   2,
				PathElements: []domain.PathElement{
					pe("property", "crm", "P2_has_type", "", 0),
					pe("class", "crm", "E55_Type", "", 1),
				},
			},
			{
				ID:         "field-birth",
				SemanticID: "LAF.3",
				SystemName: "birth",
				Position:   3,
				PathElements: []domain.PathElement{
					pe("property", "crm", "P98i_was_born", "", 0),
					pe("class", "crm", "E67_Birth", "", 1),
				},
			},
		},
	})

	var buf bytes.Buffer
	if err := NewRenderer().Render(context.Background(), snap, &buf); err != nil {
		t.Fatalf("Render() error: %v", err)
	}

	assertMermaid(t, buf.String(), `
graph TD
classDef Literal fill:#ffffff,stroke:#000000;
classDef CRM_Entity fill:#ffffff,stroke:#000000;
classDef Temporal_Entity fill:#82c3ec,stroke:#000000;
classDef Type fill:#fab565,stroke:#000000;
classDef Time-Span fill:#86bcc8,stroke:#000000;
classDef Appellation fill:#fef3ba,stroke:#000000;
classDef Place fill:#94cc7d,stroke:#000000;
classDef Persistent_Item fill:#ffffff,stroke:#000000;
classDef Conceptual_Object fill:#fddc34,stroke:#000000;
classDef Physical_Thing fill:#e1ba9c,stroke:#000000;
classDef Actor fill:#ffbdca,stroke:#000000;
classDef PC_Classes fill:#cc80ff,stroke:#000000;
classDef Multi fill:#cccccc,stroke:#000000;
classDef Default fill:#ffffff,stroke:#000000;

0["crm:E21_Person"]:::Actor -->|crm:P2_has_type| 1["crm:E55_Type"]:::Type
0["crm:E21_Person"]:::Actor -->|crm:P98i_was_born| 2["crm:E67_Birth"]:::Temporal_Entity
`)
}

// TestRendererOntologyMergesIdenticalBranches: two fields on a
// structurally identical path collapse to one ontology node. Node
// identity is the generated path_node_id (path-derived) — the legacy
// hand-coded instance_id no longer forces a split.
func TestRendererOntologyMergesIdenticalBranches(t *testing.T) {
	primaryName := peWithInstance("class", "crm", "E41_Appellation", "E41", 1, "primary_name")
	alternateName := peWithInstance("class", "crm", "E41_Appellation", "E41", 1, "alternate_name")
	snap := generators.BuildCollectionSnapshot(generators.CollectionSnapshotInput{
		Project: domain.Project{
			Entity:    domain.Entity{ID: "LA"},
			Namespace: "https://data.example.org/ns/LA/",
		},
		Collection: domain.Collection{
			Entity:        domain.Entity{ID: "collection-1", SemanticID: "LAC.1", SystemName: "person"},
			OntologyScope: pe("class", "crm", "E21_Person", "", 0),
		},
		Fields: []domain.ResolvedField{
			{
				ID:         "field-primary-name",
				SemanticID: "LAF.1",
				SystemName: "primary_name",
				Position:   1,
				PathElements: []domain.PathElement{
					pe("property", "crm", "P1_is_identified_by", "", 0),
					primaryName,
				},
			},
			{
				ID:         "field-alternate-name",
				SemanticID: "LAF.2",
				SystemName: "alternate_name",
				Position:   2,
				PathElements: []domain.PathElement{
					pe("property", "crm", "P1_is_identified_by", "", 0),
					alternateName,
				},
			},
		},
	})

	var buf bytes.Buffer
	if err := NewRenderer().Render(context.Background(), snap, &buf); err != nil {
		t.Fatalf("Render() error: %v", err)
	}

	assertMermaid(t, buf.String(), `
graph TD
classDef Literal fill:#ffffff,stroke:#000000;
classDef CRM_Entity fill:#ffffff,stroke:#000000;
classDef Temporal_Entity fill:#82c3ec,stroke:#000000;
classDef Type fill:#fab565,stroke:#000000;
classDef Time-Span fill:#86bcc8,stroke:#000000;
classDef Appellation fill:#fef3ba,stroke:#000000;
classDef Place fill:#94cc7d,stroke:#000000;
classDef Persistent_Item fill:#ffffff,stroke:#000000;
classDef Conceptual_Object fill:#fddc34,stroke:#000000;
classDef Physical_Thing fill:#e1ba9c,stroke:#000000;
classDef Actor fill:#ffbdca,stroke:#000000;
classDef PC_Classes fill:#cc80ff,stroke:#000000;
classDef Multi fill:#cccccc,stroke:#000000;
classDef Default fill:#ffffff,stroke:#000000;

0["crm:E21_Person"]:::Actor -->|crm:P1_is_identified_by| 1["crm:E41_Appellation"]:::Appellation
`)
}

func TestRendererRejectsSnapshotErrors(t *testing.T) {
	snap := &generators.Snapshot{
		Report: generators.Report{
			Errors: []generators.Diagnostic{{Message: "missing ontology scope"}},
		},
	}

	var buf bytes.Buffer
	if err := NewRenderer().Render(context.Background(), snap, &buf); err == nil {
		t.Fatal("Render() error = nil, want error")
	}
}

func TestClassifierUsesEmbeddedConfigAndDefaultsUnknownClasses(t *testing.T) {
	classifier, err := DefaultClassifier()
	if err != nil {
		t.Fatalf("DefaultClassifier() error: %v", err)
	}

	if got := classifier.Group(domain.PathElement{
		Type:      "class",
		URI:       "http://www.cidoc-crm.org/cidoc-crm/E39_Actor",
		Prefix:    "crm",
		LocalName: "E39_Actor",
	}); got != "Actor" {
		t.Fatalf("E39 group = %q, want Actor", got)
	}

	if got := classifier.Group(domain.PathElement{
		Type:      "class",
		URI:       "http://example.org/ontology/Unknown",
		Prefix:    "ex",
		LocalName: "Unknown",
	}); got != "Default" {
		t.Fatalf("unknown group = %q, want Default", got)
	}
}

func pe(kind, prefix, localName, classCode string, position int) domain.PathElement {
	return peWithInstance(kind, prefix, localName, classCode, position, "")
}

func peWithInstance(kind, prefix, localName, classCode string, position int, instanceID string) domain.PathElement {
	return domain.PathElement{
		Type:       kind,
		URI:        prefix + ":" + localName,
		Prefix:     prefix,
		LocalName:  localName,
		ClassCode:  classCode,
		Position:   position,
		InstanceID: instanceID,
	}
}

func assertMermaid(t *testing.T, got string, want string) {
	t.Helper()
	if strings.TrimSpace(got) != strings.TrimSpace(want) {
		t.Fatalf("mermaid mismatch\n--- got ---\n%s\n--- want ---\n%s", got, strings.TrimSpace(want))
	}
}

func assertMermaidGolden(t *testing.T, got string, path string) {
	t.Helper()
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v", path, err)
	}
	assertMermaid(t, got, string(want))
}
