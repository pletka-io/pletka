//go:build integration

package ontologysnapshot_test

import (
	"testing"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/database/ontologysnapshot"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/ontology"
)

// An extension may redeclare a parent class under the parent's URI (Linked
// Art 0.9 carries six CIDOC CRM classes). The captured assessment must say
// what the Go resolvers say: the same URI is one term, owned by the ontology
// declaring it under its own prefix. Decode re-assesses in Go, so this also
// pins SQL/Go parity.
func TestRedeclaredParentTermResolvesToItsNativeOwner(t *testing.T) {
	p := testdb.EmptyPool(t)
	store := ontology.NewPostgresStore(p)
	const uri = "https://example.org/ncrm/E74_Group"
	for _, o := range []struct {
		id, prefix string
		kind       domain.OntologyType
	}{{"native-crm", "ncrm", domain.OntologyTypeBase}, {"native-la", "nla", domain.OntologyTypeExtension}} {
		if _, err := store.CreateOntology(t.Context(), ontology.CreateOntologyInput{ID: o.id, Prefix: o.prefix, Namespace: "https://example.org/" + o.prefix + "/", Name: o.id, OntologyType: o.kind}); err != nil {
			t.Fatal(err)
		}
		if _, err := store.StageLegacyVersion(t.Context(), ontology.ImportVersionInput{
			Version:           ontology.CreateVersionInput{ID: o.id + "-v1", OntologyID: o.id, VersionString: "1", RDFContent: "<rdf/>"},
			Classes:           []ontology.ImportClass{{ID: o.id + "-E74", Prefix: "ncrm", LocalName: "E74_Group", URI: uri, Label: domain.Translations{"en": "Group"}}},
			NamespaceBindings: []ontology.NamespaceBinding{{Prefix: "ncrm", Namespace: "https://example.org/ncrm/"}},
		}); err != nil {
			t.Fatal(err)
		}
	}
	for _, s := range []string{
		`INSERT INTO weave_actors(id,type,slug,display_name) VALUES ('native-owner','organization','native-owner','Owner')`,
		`INSERT INTO weave_projects(id,owner_id) VALUES ('native-project','native-owner')`,
		`INSERT INTO weave_releases(project_id,version,created_by_id) VALUES ('native-project','1','native-owner')`,
		`INSERT INTO weave_project_ontology_versions_archive(project_id,version_number,ontology_version_id,added_at,is_primary) VALUES ('native-project','1','native-crm-v1',now(),true),('native-project','1','native-la-v1',now(),false)`,
		`INSERT INTO weave_fields_archive(id,project_id,version_number,path_elements) VALUES ('group-ref','native-project','1','[{"type":"class","prefix":"ncrm","local_name":"E74_Group"}]')`,
	} {
		exec(t, p, s)
	}
	a := capture(t, p, "native-project")
	d, err := a.Decode()
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Assessments) != 1 || d.Assessments[0].Status != ontologysnapshot.Resolved || len(d.Assessments[0].Candidates) != 1 || d.Assessments[0].Candidates[0].OntologyID != "native-crm" {
		t.Fatalf("assessment: %+v", d.Assessments)
	}
	definition, err := a.Resolve("field", "group-ref", "/path_elements/0")
	if err != nil {
		t.Fatal(err)
	}
	if c, _, err := definition.Evidence(); err != nil || c.VersionID != "native-crm-v1" || c.URI != uri {
		t.Fatalf("definition: %+v %v", c, err)
	}
}
