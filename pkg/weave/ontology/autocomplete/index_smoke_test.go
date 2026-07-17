package autocomplete_test

import (
	"context"
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave"
	weaveontology "github.com/pletka-io/pletka/pkg/weave/ontology"
	"github.com/pletka-io/pletka/pkg/weave/ontology/autocomplete"
)

func TestBuildIndex_AME_Smoke(t *testing.T) {
	pool := testPool(t)
	defer pool.Close()
	store := weaveontology.NewPostgresStore(pool)
	ws := weave.NewPostgresStore(pool)

	resolved, err := ws.Projects().ResolvedOntologyVersions(context.Background(), "AME", domain.ResolvedOntologyVersionOpts{})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	var versionIDs []string
	var primary string
	for _, r := range resolved {
		versionIDs = append(versionIDs, r.Link.OntologyVersionID)
		if r.Link.IsPrimary {
			primary = r.Link.OntologyVersionID
		}
	}

	idx, err := autocomplete.BuildIndexForTest(context.Background(), store, versionIDs, primary)
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	// crm:E1_CRM_Entity must exist as a class node.
	e1 := idx.ByQname["crm:E1_CRM_Entity"]
	if e1 == nil || e1.Type != "class" {
		t.Fatalf("crm:E1_CRM_Entity missing or not class: %+v", e1)
	}
	// Multi-parent: crm:E33_E41_Linguistic_Appellation has 2 superclasses.
	if n := idx.ByQname["crm:E33_E41_Linguistic_Appellation"]; n == nil || len(n.Superclasses) < 2 {
		got := 0
		if n != nil {
			got = len(n.Superclasses)
		}
		t.Fatalf("crm:E33_E41 expected >=2 superclasses, got %d", got)
	}
	// Cross-version edge: aaao:ZE19_Naming has crm:E13 among its superclasses.
	ze19 := idx.ByQname["aaao:ZE19_Naming"]
	if ze19 == nil {
		t.Fatalf("aaao:ZE19_Naming missing")
	}
	foundE13 := false
	for _, p := range ze19.Superclasses {
		if p.Qname == "crm:E13_Attribute_Assignment" {
			foundE13 = true
		}
	}
	if !foundE13 {
		t.Fatalf("aaao:ZE19_Naming should have crm:E13 superclass across versions")
	}
}
