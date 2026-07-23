//go:build integration

package pathaudit_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/weave/pathaudit"
)

// TestAudit_LA_SelfConsistent replaces the former HER-baseline equivalence
// oracle (hardcoded scanned=2071/errors=12, captured from `pletka weave
// verify-paths --project HER`). HER does not exist in the real-project
// fixture set (test/fixtures), so hardcoded magnitudes captured against it
// cannot be reproduced here. testdb.FixtureParent (LA) is the closest
// analogue: a real, self-contained, clean project with no inheritance
// parents of its own.
//
// cmd/weave_verify_paths.go's runWeaveVerifyPaths calls
// pathaudit.NewService(pool).Audit — the exact same function this test
// calls — so there is no independent CLI code path to cross-check against;
// asserting the CLI "agrees with itself" would be a tautology. Instead this
// test checks agreement a different way: it independently recomputes, via a
// second SQL query that does not share Audit's CTE, the count of
// non-literal/non-complete path elements LA should have (across both
// f.path_elements and f.subfield_paths, the same universe Audit.scanned
// counts) and asserts Audit's own scanned return value agrees with that
// independent count. It also asserts LA is error-free: LA is a clean fixture
// project (`pletka weave verify-paths --project LA` against the hydrated
// fixture data reports zero errors locally), so any error here is a
// regression, not an expected baseline count to update.
func TestAudit_LA_SelfConsistent(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	projectID := testdb.FixtureParent // LA

	svc := pathaudit.NewService(pool)
	errs, scanned, err := svc.Audit(ctx, projectID)
	if err != nil {
		t.Fatalf("Audit: %v", err)
	}
	if len(errs) != 0 {
		t.Fatalf("expected %s to be a clean fixture project (zero path errors), got %d: %+v", projectID, len(errs), errs)
	}

	wantScanned := independentScannedCount(ctx, t, pool, projectID)
	if scanned <= 0 {
		t.Fatalf("scanned = %d, want > 0 (%s should have path elements to audit)", scanned, projectID)
	}
	if scanned != wantScanned {
		t.Errorf("scanned = %d, want %d (independently counted non-literal/complete path elements)", scanned, wantScanned)
	}
}

// independentScannedCount computes, via a query written independently of
// Audit's own CTE (pathaudit.go), the count of non-literal/non-complete path
// elements for projectID across both a field's primary path_elements and its
// subfield_paths — the same two sources Audit.scanned tallies. Kept as a
// separate SQL statement (not a call into pathaudit's internals) so this test
// is checking agreement between two independent counts, not restating the
// same code path twice.
func independentScannedCount(ctx context.Context, t *testing.T, pool *pgxpool.Pool, projectID string) int {
	t.Helper()
	const query = `
SELECT
  (
    SELECT count(*)
    FROM weave_fields f
    JOIN weave_projects pr ON pr.id = f.project_id
    CROSS JOIN LATERAL jsonb_array_elements(f.path_elements) AS e(elem)
    WHERE (pr.id = $1 OR pr.system_name = $1)
      AND f.path_elements IS NOT NULL
      AND jsonb_typeof(f.path_elements) = 'array'
      AND coalesce(e.elem->>'type', '') NOT IN ('literal', 'complete')
  ) + (
    SELECT count(*)
    FROM weave_fields f
    JOIN weave_projects pr ON pr.id = f.project_id
    CROSS JOIN LATERAL jsonb_array_elements(f.subfield_paths) AS sf(subfield)
    CROSS JOIN LATERAL jsonb_array_elements(sf.subfield->'path_elements') AS e(elem)
    WHERE (pr.id = $1 OR pr.system_name = $1)
      AND f.subfield_paths IS NOT NULL
      AND jsonb_typeof(f.subfield_paths) = 'array'
      AND coalesce(e.elem->>'type', '') NOT IN ('literal', 'complete')
  ) AS total`
	var total int
	if err := pool.QueryRow(ctx, query, projectID).Scan(&total); err != nil {
		t.Fatalf("independent scanned count query: %v", err)
	}
	return total
}
