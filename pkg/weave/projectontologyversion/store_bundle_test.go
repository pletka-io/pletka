//go:build integration

package projectontologyversion_test

import (
	"context"
	"testing"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave"
	"github.com/pletka-io/pletka/pkg/weave/projectontologyversion"
)

// TestBundleForVersions_SmokesAgainstDB verifies that BundleForVersions
// returns well-formed entries that include inherited ontology versions. It
// picks the first project that inherits from another via
// weave_project_inheritance, resolves the full own+inherited set, and
// asserts that the resolved bundle is strictly larger than the own-only
// bundle returned by BundleForProject — proving that inherited ontologies
// are included.
func TestBundleForVersions_SmokesAgainstDB(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()

	// Find a project that inherits from at least one other project.
	row := pool.QueryRow(ctx, `SELECT project_id FROM weave_project_inheritance LIMIT 1`)
	var projectID string
	if err := row.Scan(&projectID); err != nil {
		t.Skipf("no inheriting project in test DB (weave_project_inheritance is empty): %v", err)
	}

	store := projectontologyversion.NewPostgresStore(pool)
	weaveStore := weave.NewPostgresStore(pool)

	// Own-only bundle via BundleForProject.
	ownBundle, err := store.BundleForProject(ctx, projectID)
	if err != nil {
		t.Fatalf("BundleForProject: %v", err)
	}

	// Full own+inherited set via ResolvedOntologyVersions.
	resolved, err := weaveStore.Projects().ResolvedOntologyVersions(ctx, projectID, domain.ResolvedOntologyVersionOpts{})
	if err != nil {
		t.Fatalf("ResolvedOntologyVersions: %v", err)
	}
	if len(resolved) == 0 {
		t.Skipf("project %s has no resolved ontology versions (own or inherited); skipping", projectID)
	}

	versionIDs := make([]string, 0, len(resolved))
	for _, r := range resolved {
		if r.Link != nil {
			versionIDs = append(versionIDs, r.Link.OntologyVersionID)
		}
	}

	resolvedBundle, err := store.BundleForVersions(ctx, versionIDs)
	if err != nil {
		t.Fatalf("BundleForVersions: %v", err)
	}

	t.Logf("project=%s own=%d resolved=%d", projectID, len(ownBundle), len(resolvedBundle))

	// Core regression guard: inheritance must add at least one entry.
	if len(resolvedBundle) <= len(ownBundle) {
		t.Errorf("inheritance assertion failed: resolvedBundle len=%d must be > ownBundle len=%d for project %s",
			len(resolvedBundle), len(ownBundle), projectID)
	}

	// Every returned entry must have a non-empty Prefix.
	for i, e := range resolvedBundle {
		if e.Prefix == "" {
			t.Errorf("resolvedBundle[%d].Prefix is empty", i)
		}
	}
}

// TestBundleForVersions_EmptyInput verifies that an empty or nil versionIDs
// slice returns nil, nil without touching the database. This is a pure unit
// test: BundleForVersions returns before reaching s.queries when len==0.
func TestBundleForVersions_EmptyInput(t *testing.T) {
	// NewPostgresStore accepts a nil pool; the early-return in BundleForVersions
	// fires before any query is executed, so no real connection is needed.
	store := projectontologyversion.NewPostgresStore(nil)
	ctx := context.Background()

	bundle, err := store.BundleForVersions(ctx, nil)
	if err != nil {
		t.Fatalf("BundleForVersions(nil): %v", err)
	}
	if bundle != nil {
		t.Errorf("BundleForVersions(nil) = %v; want nil", bundle)
	}

	bundle, err = store.BundleForVersions(ctx, []string{})
	if err != nil {
		t.Fatalf("BundleForVersions([]): %v", err)
	}
	if bundle != nil {
		t.Errorf("BundleForVersions([]) = %v; want nil", bundle)
	}
}
