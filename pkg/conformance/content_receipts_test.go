package conformance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/pletka-io/pletka/pkg/releasecontent"
)

func TestPortableReceiptInventoryMatchesReviewedCaptureLedger(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(repoRoot(t), "pkg/weave/release/testdata/capture_inventory.json"))
	if err != nil {
		t.Fatal(err)
	}
	var ledger struct {
		Sections []struct {
			Archive string `json:"archive_table"`
		} `json:"sections"`
	}
	if err := json.Unmarshal(data, &ledger); err != nil {
		t.Fatal(err)
	}
	want := make([]string, 0, len(ledger.Sections))
	for _, s := range ledger.Sections {
		want = append(want, s.Archive)
	}
	slices.Sort(want)
	if diff := cmp.Diff(want, releasecontent.RequiredSections()); diff != "" {
		t.Fatalf("receipt inventory differs from reviewed capture coverage: %s", diff)
	}
}
