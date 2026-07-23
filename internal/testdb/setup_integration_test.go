//go:build integration

package testdb_test

import (
	"context"
	"os"
	"testing"

	"github.com/pletka-io/pletka/internal/testdb"
)

func TestMain(m *testing.M) { os.Exit(testdb.Setup(m)) }

func TestClonedTemplateHasSchema(t *testing.T) {
	pool := testdb.Pool(t)
	var n int
	// weave_projects exists because the clone was made from a migrated template.
	err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM information_schema.tables WHERE table_name='weave_projects'`).Scan(&n)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if n != 1 {
		t.Fatalf("weave_projects table missing in clone: got %d", n)
	}
}

func TestClonedTemplateHasFixtures(t *testing.T) {
	pool := testdb.Pool(t)
	var n int
	// The three curated real-project fixture snapshots (testdb.FixtureOntology
	// = AME, testdb.FixtureParent = LA, testdb.FixtureChild = ING) are
	// hydrated into the template, so every clone inherits them — along with
	// GLB, SRD, and DHI, the inheritance parents AME vendors as a
	// self-contained snapshot.
	err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM weave_projects WHERE id IN ($1, $2, $3, $4, $5, $6)`,
		testdb.FixtureOntology, testdb.FixtureParent, testdb.FixtureChild, "GLB", "SRD", "DHI").Scan(&n)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if n != 6 {
		t.Fatalf("expected 6 fixture projects (AME, LA, ING, GLB, SRD, DHI) in clone, got %d", n)
	}
}

// TestClonedTemplateHasCrossOntologyEdge asserts one materialized
// cross-ontology relation from the AME snapshot's vendored aaao (2.2)
// companion ontology: aaao:ZE19_Naming is rdfs:subClassOf
// crm:E13_Attribute_Assignment (verified against the vendored source RDF at
// test/fixtures/AME/vendor/ontologies/ontology.pletka.io/aaao/2.2/src/AAAo_v2.2.rdf,
// not assumed from the class's local name — ZE19 also subclasses
// aaao:ZE13_Speech_Act, so the query pins both the source class and the
// target qname). Its presence proves the vendored ontology was actually
// restored and its relation targets threaded to the crm: namespace binding,
// not just the class row copied verbatim.
func TestClonedTemplateHasCrossOntologyEdge(t *testing.T) {
	pool := testdb.Pool(t)
	var found bool
	err := pool.QueryRow(context.Background(), `
		SELECT EXISTS (
			SELECT 1 FROM weave_ontology_relations r
			JOIN weave_ontology_classes c ON c.id = r.source_id
			WHERE r.source_kind = 'class' AND r.rel_type = 'subclass_of'
			  AND c.prefix = 'aaao' AND c.local_name = 'ZE19_Naming'
			  AND r.target_qname = 'crm:E13_Attribute_Assignment'
		)`).Scan(&found)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if !found {
		t.Fatal("expected aaao:ZE19_Naming subclass_of crm:E13_Attribute_Assignment edge in clone")
	}
}
