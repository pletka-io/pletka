package gitmaterializer

import (
	"context"
	"testing"
)

func TestEncodePletkaModManifest_Deterministic(t *testing.T) {
	manifest := pletkaModManifest{
		SchemaVersion: 1,
		Module: pletkaModModule{
			Path:      "pletka.io/orgs/to/projects/TPC",
			ProjectID: "TPC",
		},
		Require: pletkaModRequire{
			Projects: []pletkaProjectRequirement{
				{Module: "pletka.io/projects/linked-art/LA", ProjectID: "LA", Mode: "parent", Version: "1.2.0"},
			},
			Ontologies: []pletkaOntologyRequirement{
				{Module: "ontology.pletka.io/cidoc-crm", OntologyID: "cidoc-crm", OntologyVersionID: "OV1", Version: "v7.1.3"},
				{Module: "ontology.pletka.io/linked-art", OntologyID: "linked-art", OntologyVersionID: "OV2", Version: "v1.1.0"},
			},
		},
		Replace: []map[string]any{},
	}

	got, err := encodePletkaModManifest(manifest)
	if err != nil {
		t.Fatalf("encodePletkaModManifest: %v", err)
	}
	gotAgain, err := encodePletkaModManifest(manifest)
	if err != nil {
		t.Fatalf("encodePletkaModManifest second pass: %v", err)
	}
	if string(got) != string(gotAgain) {
		t.Fatal("encodePletkaModManifest is not deterministic")
	}

	want := `module:
  path: pletka.io/orgs/to/projects/TPC
  project_id: TPC
replace: []
require:
  ontologies:
    - module: ontology.pletka.io/cidoc-crm
      ontology_id: cidoc-crm
      ontology_version_id: OV1
      version: v7.1.3
    - module: ontology.pletka.io/linked-art
      ontology_id: linked-art
      ontology_version_id: OV2
      version: v1.1.0
  projects:
    - mode: parent
      module: pletka.io/projects/linked-art/LA
      project_id: LA
      version: 1.2.0
schema_version: 1
`

	if string(got) != want {
		t.Fatalf("unexpected yaml:\n%s", got)
	}
}

// TestOntologyModulePath_DefaultHost proves a Materializer built without an
// explicit host override falls back to the public-clean default
// ontology.pletka.io. No DB access needed — ontologyModulePath is a pure
// string function keyed off m.ontologyHost.
func TestOntologyModulePath_DefaultHost(t *testing.T) {
	m := NewMaterializer(nil, "", nil)
	if got := m.ontologyModulePath("cidoc-crm"); got != "ontology.pletka.io/cidoc-crm" {
		t.Fatalf("ontologyModulePath = %q, want ontology.pletka.io/cidoc-crm", got)
	}
}

// TestOntologyModulePath_OverrideHost proves WithModuleHosts overrides the
// default so a hosted deployment (e.g. git.example.org) keeps its own namespace.
func TestOntologyModulePath_OverrideHost(t *testing.T) {
	m := NewMaterializer(nil, "", nil).WithModuleHosts("git.example.org", "ontology.example.org")
	if got := m.ontologyModulePath("cidoc-crm"); got != "ontology.example.org/cidoc-crm" {
		t.Fatalf("ontologyModulePath = %q, want ontology.example.org/cidoc-crm", got)
	}
}

// TestWithModuleHosts_EmptyArgsKeepDefault proves passing empty strings to
// WithModuleHosts is a no-op — the constructor default survives, matching
// the WithOntologyImporter chaining style where an unset value falls back
// rather than clobbering.
func TestWithModuleHosts_EmptyArgsKeepDefault(t *testing.T) {
	m := NewMaterializer(nil, "", nil).WithModuleHosts("", "")
	if got := m.ontologyModulePath("cidoc-crm"); got != "ontology.pletka.io/cidoc-crm" {
		t.Fatalf("ontologyModulePath = %q, want default ontology.pletka.io/cidoc-crm", got)
	}
}

// TestProjectModulePath_DefaultHost exercises the real DB-backed fallback
// branch of projectModulePath (unknown actor -> WeaveGetActorByID errors ->
// fallback template), proving the fallback also honors the configured host.
func TestProjectModulePath_DefaultHost(t *testing.T) {
	ctx := context.Background()
	pool := hydrateTestPool(t)
	m := NewMaterializer(pool, t.TempDir(), nil)

	got, err := m.projectModulePath(ctx, "no-such-owner-id-xyz", "PROJ")
	if err != nil {
		t.Fatalf("projectModulePath: %v", err)
	}
	want := "pletka.io/actors/no-such-owner-id-xyz/projects/PROJ"
	if got != want {
		t.Fatalf("projectModulePath = %q, want %q", got, want)
	}
}

// TestProjectModulePath_OverrideHost proves the override host flows through
// the fallback branch too.
func TestProjectModulePath_OverrideHost(t *testing.T) {
	ctx := context.Background()
	pool := hydrateTestPool(t)
	m := NewMaterializer(pool, t.TempDir(), nil).WithModuleHosts("git.example.org", "ontology.example.org")

	got, err := m.projectModulePath(ctx, "no-such-owner-id-xyz", "PROJ")
	if err != nil {
		t.Fatalf("projectModulePath: %v", err)
	}
	want := "git.example.org/actors/no-such-owner-id-xyz/projects/PROJ"
	if got != want {
		t.Fatalf("projectModulePath = %q, want %q", got, want)
	}
}
