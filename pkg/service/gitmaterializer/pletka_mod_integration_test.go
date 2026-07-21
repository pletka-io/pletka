//go:build integration

package gitmaterializer

import (
	"context"
	"testing"
)

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
