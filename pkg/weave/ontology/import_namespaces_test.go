package ontology

import (
	"testing"

	parsedontology "github.com/pletka-io/pletka/pkg/weave/ontology/rdf"
)

func TestBuildImportNamespaceBindings(t *testing.T) {
	got := BuildImportNamespaceBindings(ImportNamespacePlan{
		Shared: []NamespaceBinding{
			{Prefix: "crm", Namespace: "http://www.cidoc-crm.org/cidoc-crm/"},
			{Prefix: "crm2", Namespace: "http://www.cidoc-crm.org/cidoc-crm/"},
		},
		Ontologies: []ImportNamespaceOntology{
			{
				Prefix:    "la",
				Namespace: "https://linked.art/ns/terms/",
				Versions: []NamespaceBinding{
					{Prefix: "lrmoo", Namespace: "http://iflastandards.info/ns/lrm/lrmoo/"},
				},
				Aliases: []NamespaceBinding{
					{Prefix: "frbroo", Namespace: "http://iflastandards.info/ns/fr/frbr/frbroo/"},
				},
			},
		},
	})

	want := map[string]string{
		"http://www.cidoc-crm.org/cidoc-crm/":          "crm",
		"https://linked.art/ns/terms/":                 "la",
		"http://iflastandards.info/ns/lrm/lrmoo/":      "lrmoo",
		"http://iflastandards.info/ns/fr/frbr/frbroo/": "frbroo",
	}
	if len(got) != len(want) {
		t.Fatalf("len(bindings)=%d, want %d: %#v", len(got), len(want), got)
	}
	for _, binding := range got {
		if want[binding.Namespace] != binding.Prefix {
			t.Fatalf("binding for %q=%q, want %q", binding.Namespace, binding.Prefix, want[binding.Namespace])
		}
	}
}

func TestMergeNamespaceBindingsOrdersForURIResolution(t *testing.T) {
	got := MergeNamespaceBindings(
		[]NamespaceBinding{
			{Prefix: "ex", Namespace: "https://example.org/", Weight: 10},
			{Prefix: "crm", Namespace: "http://www.cidoc-crm.org/cidoc-crm/", Weight: 10},
		},
		[]NamespaceBinding{
			{Prefix: "exa", Namespace: "https://example.org/a/", Weight: 5},
			{Prefix: "alt", Namespace: "https://example.org/", Weight: 99},
		},
	)

	if got[0].Prefix != "crm" {
		t.Fatalf("first prefix=%q, want crm for longest namespace first: %#v", got[0].Prefix, got)
	}
	if got[1].Prefix != "exa" {
		t.Fatalf("second prefix=%q, want exa for nested namespace before ex: %#v", got[1].Prefix, got)
	}
	if got[2].Prefix != "ex" {
		t.Fatalf("duplicate namespace prefix=%q, want first declaration ex: %#v", got[2].Prefix, got)
	}
}

func TestNamespaceBindingsFromDeclarations(t *testing.T) {
	got := NamespaceBindingsFromDeclarations([]parsedontology.NamespaceDeclaration{
		{Prefix: " ex ", Namespace: " https://example.org/ "},
		{Prefix: "", Namespace: "https://skip.example/"},
		{Prefix: "ex2", Namespace: "https://example.org/"},
	})

	if len(got) != 1 {
		t.Fatalf("len(bindings)=%d, want 1: %#v", len(got), got)
	}
	if got[0].Prefix != "ex" || got[0].Namespace != "https://example.org/" {
		t.Fatalf("binding=%#v, want ex -> https://example.org/", got[0])
	}
}

func TestStandardNamespaceBindingsIncludesRDFCore(t *testing.T) {
	got := StandardNamespaceBindings()
	seen := map[string]NamespaceBinding{}
	for _, binding := range got {
		seen[binding.Prefix] = binding
	}
	for _, prefix := range []string{"rdf", "rdfs", "owl", "xsd"} {
		binding, ok := seen[prefix]
		if !ok {
			t.Fatalf("missing standard namespace prefix %q in %#v", prefix, got)
		}
		if binding.Weight != 100 || binding.Source != "system" {
			t.Fatalf("standard binding %q has weight/source %d/%q", prefix, binding.Weight, binding.Source)
		}
	}
}

// TestBuildImportNamespaceManagerHonorsWeight proves the import manager keeps a
// binding's stored weight instead of flattening everything to one value, so a
// higher-weight binding wins prefix resolution for a shared namespace (#3674).
func TestBuildImportNamespaceManagerHonorsWeight(t *testing.T) {
	const base = "https://schema.org/"
	mgr := BuildImportNamespaceManager([]NamespaceBinding{
		{Prefix: "x74cccdd8", Namespace: base, Weight: 80},
		{Prefix: "schemaorg", Namespace: base, Weight: 80},
		{Prefix: "sdo", Namespace: base, Weight: 200},
	})
	got, err := mgr.GetWithBase(base)
	if err != nil {
		t.Fatalf("GetWithBase: %v", err)
	}
	if got.Prefix != "sdo" {
		t.Errorf("GetWithBase: prefix = %q, want %q", got.Prefix, "sdo")
	}

	// A zero weight (legacy rows predating weighted sources) defaults to 100,
	// so it outranks an 80-weight competitor rather than tying at an
	// arbitrary flattened value.
	mgr2 := BuildImportNamespaceManager([]NamespaceBinding{
		{Prefix: "zzz", Namespace: base, Weight: 80},
		{Prefix: "legacy", Namespace: base, Weight: 0},
	})
	got2, err := mgr2.GetWithBase(base)
	if err != nil {
		t.Fatalf("GetWithBase legacy: %v", err)
	}
	if got2.Prefix != "legacy" {
		t.Errorf("GetWithBase legacy: prefix = %q, want %q", got2.Prefix, "legacy")
	}
}
