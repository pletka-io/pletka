package ontology

import (
	"context"
	"testing"
)

func TestImportVersionWithOptionsAddsVersionDerivedNamespaceBinding(t *testing.T) {
	store := newRouteTestStore()
	svc := NewService(store, nil, nil)

	_, err := svc.ImportVersionWithOptions(context.Background(), ImportVersionInput{
		Version: CreateVersionInput{
			ID:            "ov-test",
			OntologyID:    "ont-test",
			VersionString: "1.0",
		},
		NamespaceBindings: []NamespaceBinding{
			{Prefix: "ex", Namespace: "https://example.org/", Weight: NamespaceBindingWeightRDFDeclared, Source: NamespaceBindingSourceRDFDeclared},
		},
	}, ImportVersionOptions{
		VersionNamespaceBinding: &NamespaceBinding{
			Prefix:    "frbroo",
			Namespace: "http://iflastandards.info/ns/fr/frbr/frbroo/",
		},
	})
	if err != nil {
		t.Fatalf("ImportVersionWithOptions: %v", err)
	}
	if store.lastImport == nil {
		t.Fatal("expected store import input")
	}

	var found bool
	for _, binding := range store.lastImport.NamespaceBindings {
		if binding.Prefix != "frbroo" {
			continue
		}
		found = true
		if binding.Namespace != "http://iflastandards.info/ns/fr/frbr/frbroo/" {
			t.Fatalf("namespace=%q", binding.Namespace)
		}
		if binding.Weight != NamespaceBindingWeightVersionDerived {
			t.Fatalf("weight=%d, want %d", binding.Weight, NamespaceBindingWeightVersionDerived)
		}
		if binding.Source != NamespaceBindingSourceVersionDerived {
			t.Fatalf("source=%q, want %q", binding.Source, NamespaceBindingSourceVersionDerived)
		}
	}
	if !found {
		t.Fatalf("missing version-derived binding in %#v", store.lastImport.NamespaceBindings)
	}
}

func TestImportVersionWithOptionsRejectsPartialVersionNamespaceBinding(t *testing.T) {
	store := newRouteTestStore()
	svc := NewService(store, nil, nil)

	_, err := svc.ImportVersionWithOptions(context.Background(), ImportVersionInput{
		Version: CreateVersionInput{
			ID:            "ov-test",
			OntologyID:    "ont-test",
			VersionString: "1.0",
		},
	}, ImportVersionOptions{
		VersionNamespaceBinding: &NamespaceBinding{Prefix: "frbroo"},
	})
	if err == nil {
		t.Fatal("expected partial version namespace binding to fail")
	}
	if store.lastImport != nil {
		t.Fatal("store import should not run for invalid version namespace binding")
	}
}
