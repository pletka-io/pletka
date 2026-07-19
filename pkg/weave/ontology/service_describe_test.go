package ontology_test

import (
	"context"
	"sort"
	"testing"

	"github.com/pletka-io/pletka/pkg/weave"
	weaveontology "github.com/pletka-io/pletka/pkg/weave/ontology"
)

// TestService_DescribeTerm_Smoke is the live-DB gate for DescribeTerm: a
// known CRM class in the GLB (Globalise) project resolves to a populated
// TermDescription, an unknown qname resolves to (nil, nil), and edge
// slices come back sorted ascending.
func TestService_DescribeTerm_Smoke(t *testing.T) {
	pool := testPool(t) // defined in store_bulk_smoke_test.go (package ontology_test)
	defer pool.Close()

	store := weaveontology.NewPostgresStore(pool)
	ws := weave.NewPostgresStore(pool)
	svc := weaveontology.NewService(store, ws.Projects(), nil)

	ctx := context.Background()

	t.Run("known term", func(t *testing.T) {
		desc, err := svc.DescribeTerm(ctx, "GLB", "crm:E21_Person")
		if err != nil {
			t.Fatalf("DescribeTerm: %v", err)
		}
		if desc == nil {
			t.Fatal("DescribeTerm: got nil TermDescription for crm:E21_Person")
		}
		if desc.Qname != "crm:E21_Person" {
			t.Errorf("Qname = %q, want crm:E21_Person", desc.Qname)
		}
		if desc.Type != "class" {
			t.Errorf("Type = %q, want class", desc.Type)
		}
		if len(desc.Label) == 0 {
			t.Error("Label is empty, want at least one translation")
		}
		if len(desc.Superclasses) == 0 {
			t.Error("Superclasses is empty, want at least one entry")
		}
		if desc.Ontology != "crm" {
			t.Errorf("Ontology = %q, want crm", desc.Ontology)
		}
		if !sort.StringsAreSorted(desc.Superclasses) {
			t.Errorf("Superclasses not sorted: %v", desc.Superclasses)
		}
		if !sort.StringsAreSorted(desc.Subclasses) {
			t.Errorf("Subclasses not sorted: %v", desc.Subclasses)
		}
		if !sort.StringsAreSorted(desc.AsDomainOf) {
			t.Errorf("AsDomainOf not sorted: %v", desc.AsDomainOf)
		}
		if !sort.StringsAreSorted(desc.AsRangeOf) {
			t.Errorf("AsRangeOf not sorted: %v", desc.AsRangeOf)
		}
	})

	t.Run("unknown term", func(t *testing.T) {
		desc, err := svc.DescribeTerm(ctx, "GLB", "crm:NoSuchTerm")
		if err != nil {
			t.Fatalf("DescribeTerm: %v", err)
		}
		if desc != nil {
			t.Fatalf("DescribeTerm: got %+v, want nil", desc)
		}
	})
}
