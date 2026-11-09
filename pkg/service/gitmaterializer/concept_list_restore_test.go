package gitmaterializer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
)

// conceptListFixture writes a minimal authored tree: manifest, pletka.mod,
// one field with a base override, and one concept list.
func conceptListFixture(t *testing.T, projectID, ownerID string) string {
	t.Helper()
	root := t.TempDir()
	manifest, err := encodeProjectManifest(projectManifest{
		SchemaVersion: 1,
		Project: projectManifestProject{
			ID: projectID, Title: map[string]string{"en": projectID}, Namespace: "https://example.org/" + projectID + "/", Visibility: "private",
			Owner: &manifestActor{ActorID: ownerID, Type: "organization", Slug: "cl-org", DisplayName: "CL Org"},
		},
		Namespaces: &projectManifestNamespaces{EffectiveBindings: []projectManifestNamespaceBinding{
			{Prefix: "crm", Namespace: "http://www.cidoc-crm.org/cidoc-crm/", Source: "ontology"},
		}},
		Manifests: &projectManifestManifests{},
	})
	if err != nil {
		t.Fatal(err)
	}
	mustWrite(t, root, "project.yaml", manifest)
	mod, err := encodePletkaModManifest(pletkaModManifest{SchemaVersion: 1, Module: pletkaModModule{Path: "pletka.io/orgs/cl-org/projects/" + projectID, ProjectID: projectID}, Require: pletkaModRequire{Projects: []pletkaProjectRequirement{}, Ontologies: []pletkaOntologyRequirement{}}, Replace: []map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	mustWrite(t, root, "pletka.mod", mod)
	fieldID := projectID + "F.1"
	mustWrite(t, root, domain.FilePath(domain.PathSpec{EntityType: "field", EntityID: fieldID}), []byte(
		"deprecated: false\ndescription: {}\nexpected_value_type: String\nontology_path: ->crm:P1_is_identified_by->crm:E41_Appellation\nontology_scope: crm:E21_Person\npath_elements:\n  - crm:P1_is_identified_by\n  - crm:E41_Appellation\nsemantic_id: "+fieldID+"\nstatus: draft\nsystem_name: name\nui_name:\n  en: Name\n"))
	mustWrite(t, root, domain.FilePath(domain.PathSpec{EntityType: "base_override", FieldID: fieldID}), []byte(
		"field_id: "+fieldID+"\nentity_type: \"\"\nentity_id: \"\"\nposition: 0\ncollection_order: 0\ndisplay_name:\n  en: Name\ndescription: {}\ncollection_name: {}\ncategory_id: \"\"\npart_of_collection_id: \"\"\nset_value: \"\"\nis_required: false\nmin_occurs: 0\nmax_occurs: null\nis_hidden: false\nvisibility: \"\"\nrefs: []\n"))
	listID := projectID + ".CL.1"
	mustWrite(t, root, domain.FilePath(domain.PathSpec{EntityType: "concept_list", EntityID: listID}), []byte(
		"deprecated: false\ndescription: {}\nis_closed: true\nsemantic_id: "+listID+"\nstatus: draft\nsystem_name: license\nui_name:\n  en: License\n"))
	return root
}

func mustWrite(t *testing.T, root, rel string, payload []byte) {
	t.Helper()
	abs := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, payload, 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestLoadProjectSnapshot_ConceptLists pins that concept-lists/<ID>.yaml is
// classified and bucketed by the loader.
func TestLoadProjectSnapshot_ConceptLists(t *testing.T) {
	root := conceptListFixture(t, "TPL", "org-1")
	snap, err := LoadProjectSnapshot(root)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(snap.Entities.ConceptLists); n != 1 {
		t.Fatalf("want 1 concept list, got %d", n)
	}
	if got := snap.Entities.ConceptLists[0].EntityID; got != "TPL.CL.1" {
		t.Errorf("entity id = %q", got)
	}
	if got := snap.Entities.ConceptLists[0].EntityType; got != "concept_list" {
		t.Errorf("entity type = %q", got)
	}
}
