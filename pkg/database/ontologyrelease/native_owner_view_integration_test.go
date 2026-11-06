//go:build integration

package ontologyrelease_test

import (
	"testing"

	"github.com/pletka-io/pletka/pkg/weave/ontology"
)

// The capture guard reads weave_project_semantic_candidates and requires one
// candidate per location. An extension that redeclares a parent class under
// the parent's URI must therefore collapse to the native owner in storage
// too, as it does in the Go resolvers.
func TestCandidatesViewCollapsesRedeclaredTermToNativeOwner(t *testing.T) {
	defaults(t)
	base := candidate(t, "viewnative_base")
	base.publish(t)
	ext := candidate(t, "viewnative_ext")
	ext.extend(t, base)
	baseURI := "https://example.org/viewnative_base/A"
	ext.input.Classes = append(ext.input.Classes, ontology.ImportClass{ID: ontology.GenerateClassID(ext.input.Version.ID, baseURI), Prefix: "viewnative_base", LocalName: "A", URI: baseURI})
	ext.input.NamespaceBindings = append(ext.input.NamespaceBindings, ontology.NamespaceBinding{Prefix: "viewnative_base", Namespace: "https://example.org/viewnative_base/"})
	ext.input.Publication.Pins = map[string]string{base.input.Version.OntologyID: base.input.Version.ID}
	ext.publish(t)
	baseArtifact, extArtifact := base.artifact(t), ext.artifact(t)
	for _, s := range []string{
		`INSERT INTO weave_actors(id,type,slug,display_name) VALUES ('viewnative-owner','organization','viewnative-owner','Owner') ON CONFLICT DO NOTHING`,
		`INSERT INTO weave_projects(id,owner_id) VALUES ('viewnative','viewnative-owner')`,
		`INSERT INTO weave_releases(project_id,version,created_by_id) VALUES ('viewnative','1','viewnative-owner')`,
		`INSERT INTO weave_fields_archive(id,project_id,version_number,path_elements) VALUES ('viewnative-ref','viewnative','1','[{"type":"class","prefix":"viewnative_base","local_name":"A"}]')`,
	} {
		if _, err := base.pool.Exec(t.Context(), s); err != nil {
			t.Fatal(err)
		}
	}
	for _, p := range []struct {
		ontology, version, digest string
	}{{base.input.Version.OntologyID, base.input.Version.ID, baseArtifact.Digest}, {ext.input.Version.OntologyID, ext.input.Version.ID, extArtifact.Digest}} {
		if _, err := base.pool.Exec(t.Context(), `INSERT INTO weave_release_ontology_dependencies VALUES ('viewnative','1',$1,$2,$3)`, p.ontology, p.version, p.digest); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := base.pool.Query(t.Context(), `SELECT ontology_id FROM weave_project_semantic_candidates WHERE project_id='viewnative' AND version_number='1' AND entity_id='viewnative-ref' ORDER BY ontology_id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var owners []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		owners = append(owners, id)
	}
	if len(owners) != 1 || owners[0] != base.input.Version.OntologyID {
		t.Fatalf("candidates = %v, want only the native owner %s", owners, base.input.Version.OntologyID)
	}
}
