package gitmaterializer

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
)

func TestNamespaceAliasesFromBindings(t *testing.T) {
	rows := []sqlcgen.WeaveNamespaceBinding{
		{Prefix: "crmgeo", Namespace: "https://dlnarratives.eu/crmgeo/"},
		{Prefix: "crmgeo", Namespace: "http://www.ics.forth.gr/isl/CRMgeo/"},
		{Prefix: "crmgeo", Namespace: "http://www.cidoc-crm.org/extensions/crmgeo/"}, // canonical: dropped
		{Prefix: "crmgeo", Namespace: "http://www.ics.forth.gr/isl/CRMgeo/"},         // duplicate: dropped
		{Prefix: "crmgeo", Namespace: "  "},                                          // blank: dropped
		{Prefix: "crm", Namespace: "http://www.cidoc-crm.org/cidoc-crm/"},            // other prefix: dropped
	}

	got := namespaceAliasesFromBindings(rows, "crmgeo", "http://www.cidoc-crm.org/extensions/crmgeo/")
	want := []string{
		"http://www.ics.forth.gr/isl/CRMgeo/",
		"https://dlnarratives.eu/crmgeo/",
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("aliases mismatch (-want +got):\n%s", diff)
	}

	if got := namespaceAliasesFromBindings(rows, "", "x"); got != nil {
		t.Errorf("empty prefix: want nil, got %v", got)
	}
}

func TestPoolVendoredOntologyBindings(t *testing.T) {
	effective := []VendoredOntologyExternalBinding{
		{Prefix: "crm", Namespace: "http://www.cidoc-crm.org/cidoc-crm/"},
	}
	ontologies := []VendoredOntologySnapshot{
		{Module: "nil-snapshot"}, // skipped
		{
			Module: "crmgeo",
			Snapshot: &OntologySnapshot{Manifest: OntologyVendorManifest{
				Ontology: OntologyVendorManifestRoot{
					Slug:      "crmgeo-module",
					Prefixes:  []string{"crmgeo"},
					Namespace: "http://www.cidoc-crm.org/extensions/crmgeo/",
					NamespaceAliases: []string{
						"http://www.ics.forth.gr/isl/CRMgeo/",
						"",
					},
				},
			}},
		},
		{
			Module: "slug-fallback",
			Snapshot: &OntologySnapshot{Manifest: OntologyVendorManifest{
				Ontology: OntologyVendorManifestRoot{
					Slug:      "aaao",
					Namespace: "https://ontology.swissartresearch.net/aaao/",
				},
			}},
		},
	}

	got := poolVendoredOntologyBindings(effective, ontologies)
	want := []VendoredOntologyExternalBinding{
		{Prefix: "crm", Namespace: "http://www.cidoc-crm.org/cidoc-crm/"},
		{Prefix: "crmgeo", Namespace: "http://www.cidoc-crm.org/extensions/crmgeo/"},
		{Prefix: "crmgeo", Namespace: "http://www.ics.forth.gr/isl/CRMgeo/"},
		{Prefix: "aaao", Namespace: "https://ontology.swissartresearch.net/aaao/"},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("pooled bindings mismatch (-want +got):\n%s", diff)
	}
}

func vendoredProjectDep(projectID string, parentIDs ...string) VendoredProjectSnapshot {
	manifest := ProjectManifestFile{Project: ProjectManifestProject{ID: projectID}}
	if len(parentIDs) > 0 {
		inh := &ProjectManifestInheritance{}
		for _, parentID := range parentIDs {
			inh.Parents = append(inh.Parents, ProjectManifestParent{ProjectID: parentID})
		}
		manifest.Inheritance = inh
	}
	return VendoredProjectSnapshot{
		Module:   "pletka.io/projects/" + projectID,
		Snapshot: &ProjectSnapshot{Manifest: manifest},
	}
}

func TestTopoOrderVendoredProjects(t *testing.T) {
	// Alphabetical module order would hydrate DHI before its parent LA — the
	// AME restore failure. SRD depends on LAF; ORPHan has a parent outside
	// the pending set (already in the DB), which imposes no ordering.
	deps := []VendoredProjectSnapshot{
		vendoredProjectDep("DHI", "LA"),
		vendoredProjectDep("LA"),
		vendoredProjectDep("LAF"),
		vendoredProjectDep("ORPH", "ALREADY_IN_DB"),
		vendoredProjectDep("SRD", "LAF"),
	}
	pendingIdx := map[string]int{"DHI": 0, "LA": 1, "LAF": 2, "ORPH": 3, "SRD": 4}
	pendingIDs := []string{"DHI", "LA", "LAF", "ORPH", "SRD"}

	got, err := topoOrderVendoredProjects(deps, pendingIdx, pendingIDs)
	if err != nil {
		t.Fatalf("topo order: %v", err)
	}
	want := []string{"LA", "DHI", "LAF", "ORPH", "SRD"}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("order mismatch (-want +got):\n%s", diff)
	}
}

func TestTopoOrderVendoredProjectsCycle(t *testing.T) {
	deps := []VendoredProjectSnapshot{
		vendoredProjectDep("A", "B"),
		vendoredProjectDep("B", "A"),
	}
	pendingIdx := map[string]int{"A": 0, "B": 1}

	if _, err := topoOrderVendoredProjects(deps, pendingIdx, []string{"A", "B"}); err == nil {
		t.Fatal("want inheritance cycle error, got nil")
	}
}
