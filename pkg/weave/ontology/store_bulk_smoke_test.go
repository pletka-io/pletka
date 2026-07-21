//go:build integration

package ontology_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave"
	weaveontology "github.com/pletka-io/pletka/pkg/weave/ontology"
)

// testPool opens a pgxpool against a seeded real-data DB, skipping the test
// when TEST_DATABASE_URL is not set. The smokes that use it (edge_provenance,
// store_bulk, service_describe) assert on real multi-version customer ontology
// data the synthetic fixtures do not reproduce, so this package cannot run
// testdb.Setup — in the fixture lane they skip rather than fall through to a
// dev DB. Mirrors the helper in
// pkg/weave/ontology/autocomplete/resolver_smoke_test.go.
func testPool(t *testing.T) *pgxpool.Pool {
	testdb.RequireRealDataDB(t)
	return testdb.Pool(t)
}

// domain0Opts returns a zero-value opts struct (include own + inherited).
func domain0Opts() domain.ResolvedOntologyVersionOpts {
	return domain.ResolvedOntologyVersionOpts{}
}

// TestStore_ListClassesByVersions_EqualsUnion_Smoke verifies that
// ListClassesByVersions returns the same total as summing ListClassesByVersion
// across each version individually, for a project with multiple linked
// ontology versions (AME).
func TestStore_ListClassesByVersions_EqualsUnion_Smoke(t *testing.T) {
	pool := testPool(t)
	store := weaveontology.NewPostgresStore(pool)
	ws := weave.NewPostgresStore(pool)

	resolved, err := ws.Projects().ResolvedOntologyVersions(context.Background(), "AME", domain0Opts())
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	var versionIDs []string
	for _, r := range resolved {
		versionIDs = append(versionIDs, r.Link.OntologyVersionID)
	}
	if len(versionIDs) < 2 {
		t.Skipf("AME expected multiple linked versions, got %d", len(versionIDs))
	}

	bulk, err := store.ListClassesByVersions(context.Background(), versionIDs)
	if err != nil {
		t.Fatalf("bulk: %v", err)
	}
	var sum int
	for _, v := range versionIDs {
		per, err := store.ListClassesByVersion(context.Background(), v)
		if err != nil {
			t.Fatalf("per-version: %v", err)
		}
		sum += len(per)
	}
	if len(bulk) != sum {
		t.Fatalf("bulk count %d != sum of per-version %d", len(bulk), sum)
	}
	t.Logf("bulk=%d sum=%d versionIDs=%v", len(bulk), sum, versionIDs)
}
