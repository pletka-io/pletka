package namespace_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/pletka-io/pletka/pkg/namespace"
)

func TestRegistryPreservesConflictsAndRequiresCheckedProjection(t *testing.T) {
	input := []namespace.Binding{
		{Prefix: "crm", Namespace: "https://one/", Weight: 20, Source: "ontology"},
		{Prefix: "crm", Namespace: "https://two/", Weight: 20, Source: "ontology"},
	}
	r := namespace.NewRegistry(input)
	want := append([]namespace.Binding{}, input...)
	input[0].Namespace = "mutated"
	got, err := r.Bindings()
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatal(diff)
	}
	got[0].Namespace = "mutated again"
	projection, err := r.Effective()
	if err == nil {
		t.Fatal("tied aliases became a checked projection")
	}
	if _, err := projection.Expand("crm", "E1"); err == nil {
		t.Fatal("failed projection retained a partial map")
	}
	got, err = r.Bindings()
	if err != nil || cmp.Diff(want, got) != "" {
		t.Fatalf("projection changed evidence: %v %+v", err, got)
	}
	input = append([]namespace.Binding{}, want...)
	input = append(input, namespace.Binding{Prefix: "crm", Namespace: "https://manifest/", Weight: 80, Source: "manifest"})
	effective, err := namespace.NewRegistry(input).Effective()
	if err != nil {
		t.Fatal(err)
	}
	bindings, err := effective.Bindings()
	if err != nil {
		t.Fatal(err)
	}
	bindings[0].Namespace = "mutated output"
	uri, err := effective.Expand("crm", "E1")
	if err != nil || uri != "https://manifest/E1" {
		t.Fatalf("checked expansion = %q, %v", uri, err)
	}
	if _, err := effective.Expand("rdf", "Property"); err == nil {
		t.Fatal("unknown prefix used a default")
	}
}

func TestNamespaceProjectionRejectsUnloadedAndMalformedEvidence(t *testing.T) {
	if _, err := (namespace.Registry{}).Bindings(); err == nil {
		t.Fatal("unloaded registry appeared empty")
	}
	for _, r := range []namespace.Registry{
		{}, namespace.NewRegistry([]namespace.Binding{{Prefix: "crm"}}),
		namespace.NewRegistry([]namespace.Binding{{Namespace: "https://example/"}}),
	} {
		if _, err := r.Effective(); err == nil {
			t.Fatal("unchecked inventory accepted")
		}
	}
	if _, err := (namespace.EffectiveBindings{}).Bindings(); err == nil {
		t.Fatal("zero projection usable")
	}
	if _, err := namespace.NewRegistry(nil).Effective(); err != nil {
		t.Fatalf("explicitly loaded empty inventory rejected: %v", err)
	}
}
