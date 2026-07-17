package gitmaterializer

import (
	"strings"
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
)

func TestEncodeProjectManifest_Deterministic(t *testing.T) {
	manifest := projectManifest{
		SchemaVersion: 1,
		Project: projectManifestProject{
			ID: "TPC",
			Title: domain.Translations{
				"nl": "Testproject",
				"en": "Test Project Core",
			},
			Description: domain.Translations{
				"nl": "Beschrijving",
				"en": "Example semantic modeling project",
			},
			Readme: domain.Translations{
				"en": "Long form project guide",
			},
			Visibility: "private",
			Owner: &manifestActor{
				ActorID:     "ORG1",
				Type:        "organization",
				Slug:        "to",
				DisplayName: "test org",
			},
			CreatedBy: &manifestActor{
				ActorID:     "ACT1",
				Type:        "person",
				Slug:        "sjoerd_siebinga",
				DisplayName: "Sjoerd Siebinga",
			},
			License: "CC-BY-4.0",
			BaseURL: "https://example.org/data/",
			Topics:  []string{"linked-art", "crm"},
		},
		Inheritance: &projectManifestInheritance{
			Parents: []projectManifestParent{
				{ProjectID: "LA", IsPrimary: true, CanonicalOrder: 0, SourceMode: "draft"},
				{ProjectID: "CRMPE", CanonicalOrder: 1, SourceMode: "release", SourceVersion: "1.2.0"},
			},
		},
		Ontologies: &projectManifestOntologies{
			LinkedVersions: []projectManifestLinkedOntology{
				{OntologyID: "linked-art", OntologyVersionID: "OV2", Version: "v1.1.0"},
				{OntologyID: "cidoc-crm", OntologyVersionID: "OV1", Version: "v7.1.3", IsPrimary: true},
			},
		},
		Namespaces: &projectManifestNamespaces{
			EffectiveBindings: []projectManifestNamespaceBinding{
				{Prefix: "crm", Namespace: "http://www.cidoc-crm.org/cidoc-crm/", Source: "system"},
				{Prefix: "ex", Namespace: "https://example.org/ns/", Source: "project"},
			},
		},
		Manifests: &projectManifestManifests{
			Adoptions: "adoptions/index.yaml",
			Forks:     "forks/index.yaml",
		},
	}

	got, err := encodeProjectManifest(manifest)
	if err != nil {
		t.Fatalf("encodeProjectManifest: %v", err)
	}
	gotAgain, err := encodeProjectManifest(manifest)
	if err != nil {
		t.Fatalf("encodeProjectManifest second pass: %v", err)
	}
	if string(got) != string(gotAgain) {
		t.Fatal("encodeProjectManifest is not deterministic across repeated calls")
	}

	want := strings.TrimLeft(`
inheritance:
  parents:
    - canonical_order: 0
      is_primary: true
      project_id: LA
      source_mode: draft
    - canonical_order: 1
      project_id: CRMPE
      source_mode: release
      source_version: 1.2.0
manifests:
  adoptions: adoptions/index.yaml
  forks: forks/index.yaml
namespaces:
  effective_bindings:
    - namespace: http://www.cidoc-crm.org/cidoc-crm/
      prefix: crm
      source: system
    - namespace: https://example.org/ns/
      prefix: ex
      source: project
ontologies:
  linked_versions:
    - ontology_id: linked-art
      ontology_version_id: OV2
      version: v1.1.0
    - is_primary: true
      ontology_id: cidoc-crm
      ontology_version_id: OV1
      version: v7.1.3
project:
  base_url: https://example.org/data/
  created_by:
    actor_id: ACT1
    display_name: Sjoerd Siebinga
    slug: sjoerd_siebinga
    type: person
  description:
    en: Example semantic modeling project
    nl: Beschrijving
  id: TPC
  license: CC-BY-4.0
  owner:
    actor_id: ORG1
    display_name: test org
    slug: to
    type: organization
  readme:
    en: Long form project guide
  title:
    en: Test Project Core
    nl: Testproject
  topics:
    - linked-art
    - crm
  visibility: private
schema_version: 1
`, "\n")

	if string(got) != want {
		t.Fatalf("unexpected yaml:\n%s", got)
	}
}
