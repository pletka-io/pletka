//go:build integration

package pathaudit_test

import (
	"context"
	"testing"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/weave/pathaudit"
)

// TestAudit_HER_MatchesCLIBaseline is the equivalence oracle for the lift:
// it asserts Audit(ctx, "HER") reproduces the exact verdict-class counts
// captured from `pletka weave verify-paths --project HER` before this
// refactor (scanned=2071, errors=12: missing=11, type-mismatch=1,
// ambiguous=0, malformed=0). A drift here means the lifted SQL or verdict
// logic diverged from the original CLI behavior. NOTE: these baseline
// counts predate the defaults-ontology resolution-scope union and the
// --exclude-standard retirement; if the real-data fixture DB gains a
// defaults ontology import or HER has standard-prefix path elements, these
// counts may need re-capturing.
func TestAudit_HER_MatchesCLIBaseline(t *testing.T) {
	testdb.RequireRealDataDB(t)
	pool := testdb.Pool(t)
	svc := pathaudit.NewService(pool)

	errs, scanned, err := svc.Audit(context.Background(), "HER")
	if err != nil {
		t.Fatalf("Audit: %v", err)
	}
	if scanned <= 0 {
		t.Fatalf("scanned = %d, want > 0", scanned)
	}

	const wantScanned = 2071
	if scanned != wantScanned {
		t.Errorf("scanned = %d, want %d", scanned, wantScanned)
	}

	wantByClass := map[string]int{
		"missing":       11,
		"type-mismatch": 1,
	}
	gotByClass := map[string]int{}
	for _, e := range errs {
		gotByClass[pathaudit.VerdictClass(e.Verdict)]++
	}

	if len(errs) != 12 {
		t.Errorf("len(errs) = %d, want 12", len(errs))
	}
	for class, want := range wantByClass {
		if got := gotByClass[class]; got != want {
			t.Errorf("verdict class %q count = %d, want %d", class, got, want)
		}
	}
	for class, got := range gotByClass {
		if _, known := wantByClass[class]; !known {
			t.Errorf("unexpected verdict class %q with count %d", class, got)
		}
	}
}
