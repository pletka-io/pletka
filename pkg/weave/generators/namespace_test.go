package generators

import (
	"slices"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/pletka-io/pletka/pkg/domain"
)

func buildTestNamespaceSet(t *testing.T, project domain.Project, bindings []*domain.NamespaceBinding) NamespaceSet {
	t.Helper()
	set, err := BuildNamespaceSet(project, bindings)
	if err != nil {
		t.Fatal(err)
	}
	return set
}

func TestBuildNamespaceSetEqualWeights(t *testing.T) {
	for _, tc := range []struct {
		name      string
		bindings  []*domain.NamespaceBinding
		wantError bool
	}{
		{"conflicting winners", []*domain.NamespaceBinding{
			{Prefix: "crm", Namespace: "https://a.example/", Weight: 80},
			{Prefix: "crm", Namespace: "https://b.example/", Weight: 80},
		}, true},
		{"lower priority aliases", []*domain.NamespaceBinding{
			{Prefix: "crm", Namespace: "https://a.example/", Weight: 40},
			{Prefix: "crm", Namespace: "https://b.example/", Weight: 40},
			{Prefix: "crm", Namespace: "https://canonical.example/", Weight: 80},
		}, false},
		{"duplicate URI with different provenance", []*domain.NamespaceBinding{
			{Prefix: "crm", Namespace: "https://a.example/", Weight: 80, Source: "ontology"},
			{Prefix: " crm ", Namespace: " https://a.example/ ", Weight: 80, Source: "global"},
		}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var want NamespaceSet
			var wantErr string
			for i := range len(tc.bindings) {
				order := append(slices.Clone(tc.bindings[i:]), tc.bindings[:i]...)
				for range 2 {
					set, err := BuildNamespaceSet(domain.Project{}, order)
					if (err != nil) != tc.wantError {
						t.Fatalf("BuildNamespaceSet: %v, wantError %v", err, tc.wantError)
					}
					if err != nil {
						if wantErr != "" && err.Error() != wantErr {
							t.Fatalf("error changed with order: %q vs %q", err, wantErr)
						}
						wantErr = err.Error()
					} else {
						if want.ByPrefix != nil {
							if diff := cmp.Diff(want, set); diff != "" {
								t.Fatal(diff)
							}
						}
						want = set
					}
					slices.Reverse(order)
				}
			}
		})
	}
}

func TestBuildNamespaceSet(t *testing.T) {
	set := buildTestNamespaceSet(t, domain.Project{
		Entity:    domain.Entity{ID: "SRD"},
		Namespace: "https://data.example.org/srd",
	}, []*domain.NamespaceBinding{
		{Prefix: "crm", Namespace: "http://old.example/crm/", Weight: 1, Source: "system"},
		{Prefix: "crm", Namespace: "http://www.cidoc-crm.org/cidoc-crm/", Weight: 10, Source: "system"},
		{Prefix: "la", Namespace: "https://linked.art/ns/terms/", Weight: 100, Source: "system"},
		{Prefix: "", Namespace: "https://ignored.example/", Weight: 100},
	})

	if set.ProjectPrefix != "srd" {
		t.Fatalf("ProjectPrefix = %q, want srd", set.ProjectPrefix)
	}
	if set.ProjectURI != "https://data.example.org/srd/" {
		t.Fatalf("ProjectURI = %q", set.ProjectURI)
	}
	if got := set.ByPrefix["crm"]; got != "http://www.cidoc-crm.org/cidoc-crm/" {
		t.Fatalf("crm namespace = %q", got)
	}
	if got := set.ByPrefix["srd"]; got != "https://data.example.org/srd/" {
		t.Fatalf("project namespace = %q", got)
	}
	if got := set.ByPrefix["la"]; got != "https://linked.art/ns/terms/" {
		t.Fatalf("la namespace = %q", got)
	}
	if set.Project == nil || set.Project.Source != "project" {
		t.Fatalf("project binding not marked as project: %#v", set.Project)
	}
}

func TestBuildNamespaceSetDoesNotClobberExplicitProjectIDPrefix(t *testing.T) {
	set := buildTestNamespaceSet(t, domain.Project{
		Entity:    domain.Entity{ID: "LA"},
		Namespace: "https://data.example.org/la",
	}, []*domain.NamespaceBinding{
		{Prefix: "la", Namespace: "https://linked.art/ns/terms/", Weight: 100, Source: "system"},
	})

	if got := set.ByPrefix["la"]; got != "https://linked.art/ns/terms/" {
		t.Fatalf("la namespace = %q, want linked.art namespace", got)
	}
	if got, ok := set.ResolvePrefix("la"); !ok || got != "https://linked.art/ns/terms/" {
		t.Fatalf("ResolvePrefix(la) = %q, %v; want linked.art namespace", got, ok)
	}
	if set.ProjectURI != "https://data.example.org/la/" {
		t.Fatalf("ProjectURI = %q", set.ProjectURI)
	}
}

func TestBuildNamespaceSetDefaultsProjectNamespace(t *testing.T) {
	set := buildTestNamespaceSet(t, domain.Project{Entity: domain.Entity{ID: "SRD"}}, nil)

	if got := set.ByPrefix["srd"]; got != "/ns/SRD/" {
		t.Fatalf("default project namespace = %q", got)
	}
}

func TestNamespaceSetResolvePrefixUsesWeightedBindings(t *testing.T) {
	set := NamespaceSet{
		ByPrefix: map[string]string{
			"crm": "http://old.example/crm/",
		},
		Bindings: []NamespaceBinding{
			{Prefix: "crm", Namespace: "http://lower.example/crm/", Weight: 1},
			{Prefix: "crm", Namespace: "http://www.cidoc-crm.org/cidoc-crm/", Weight: 100},
		},
	}

	got, ok := set.ResolvePrefix("crm")
	if !ok {
		t.Fatal("ResolvePrefix(crm) returned false")
	}
	if got != "http://www.cidoc-crm.org/cidoc-crm/" {
		t.Fatalf("ResolvePrefix(crm) = %q", got)
	}
}
