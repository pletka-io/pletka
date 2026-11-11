package generators

import (
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
)

func TestBuildNamespaceSet(t *testing.T) {
	set := BuildNamespaceSet(domain.Project{
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
	set := BuildNamespaceSet(domain.Project{
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
	set := BuildNamespaceSet(domain.Project{Entity: domain.Entity{ID: "SRD"}}, nil)

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

func TestBuildNamespaceSetProjectBindingBeatsHeavierGlobal(t *testing.T) {
	set := BuildNamespaceSet(domain.Project{Entity: domain.Entity{ID: "SI"}}, []*domain.NamespaceBinding{
		{Prefix: "crmgeo", Namespace: "https://dlnarratives.eu/crmgeo/", Weight: 80},
		{Prefix: "crmgeo", Namespace: "http://www.ics.forth.gr/isl/CRMgeo/", Weight: 5, ProjectID: "SI"},
		{Prefix: "crmgeo", Namespace: "https://other.example/", Weight: 900, ProjectID: "LA"},
		{Prefix: "crminf", Namespace: "https://dlnarratives.eu/crminf/", Weight: 80},
		{Prefix: "crminf", Namespace: "http://www.ics.forth.gr/isl/CRMinf/", Weight: 250},
	})
	if got := set.ByPrefix["crmgeo"]; got != "http://www.ics.forth.gr/isl/CRMgeo/" {
		t.Fatalf("crmgeo = %q, want the project's own binding", got)
	}
	if got := set.ByPrefix["crminf"]; got != "http://www.ics.forth.gr/isl/CRMinf/" {
		t.Fatalf("crminf = %q, want the heavier global binding", got)
	}
}

func TestBuildNamespaceSetParentBindingsRankBelowOwnAboveGlobal(t *testing.T) {
	set := BuildNamespaceSet(domain.Project{Entity: domain.Entity{ID: "SI"}}, []*domain.NamespaceBinding{
		{Prefix: "a", Namespace: "https://global.example/a/", Weight: 900},
		{Prefix: "a", Namespace: "https://pir.example/a/", Weight: 1, ProjectID: "PIR", ParentRank: 2},
		{Prefix: "a", Namespace: "https://la.example/a/", Weight: 1, ProjectID: "LA", ParentRank: 1},
		{Prefix: "b", Namespace: "https://pir.example/b/", Weight: 1, ProjectID: "PIR", ParentRank: 2},
		{Prefix: "b", Namespace: "https://si.example/b/", Weight: 1, ProjectID: "SI"},
		{Prefix: "c", Namespace: "https://pir.example/c/", Weight: 1, ProjectID: "PIR", ParentRank: 2},
		{Prefix: "d", Namespace: "https://stranger.example/d/", Weight: 900, ProjectID: "XYZ"},
	})
	want := map[string]string{
		"a": "https://la.example/a/",  // primary parent beats second parent and global
		"b": "https://si.example/b/",  // own beats parent
		"c": "https://pir.example/c/", // a parent-only prefix is inherited
	}
	for prefix, ns := range want {
		if got := set.ByPrefix[prefix]; got != ns {
			t.Errorf("%s = %q, want %q", prefix, got, ns)
		}
	}
	if got, ok := set.ByPrefix["d"]; ok {
		t.Errorf("d = %q, want an unrelated project's binding to stay invisible", got)
	}
}
