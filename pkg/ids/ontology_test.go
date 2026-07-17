package ids

import (
	"testing"

	"github.com/google/uuid"
)

func TestOntologyNamespaceLiteral(t *testing.T) {
	want := uuid.NewSHA1(uuid.Nil, []byte("archessql/ontology/v1"))
	if OntologyNamespace != want {
		t.Fatalf("OntologyNamespace literal drifted: have %s want %s", OntologyNamespace, want)
	}
}

func TestOntologyVersionIDDeterministic(t *testing.T) {
	a := OntologyVersionID("crm", "7.1.3")
	b := OntologyVersionID("crm", "7.1.3")
	if a != b {
		t.Fatalf("OntologyVersionID not deterministic: %s != %s", a, b)
	}
	// Sanity: a different version yields a different UUID.
	c := OntologyVersionID("crm", "7.1.4")
	if a == c {
		t.Fatalf("OntologyVersionID collision across versions: %s", a)
	}
}

func TestOntologyClassIDStable(t *testing.T) {
	a := OntologyClassID("crm", "7.1.3", "crm:E1_CRM_Entity")
	b := OntologyClassID("crm", "7.1.3", "crm:E1_CRM_Entity")
	if a != b {
		t.Fatalf("OntologyClassID not deterministic: %s != %s", a, b)
	}
	if a == OntologyClassID("crm", "7.1.3", "crm:E2_Temporal_Entity") {
		t.Fatalf("OntologyClassID collides across qnames")
	}
}
