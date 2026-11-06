package gitmaterializer

import "testing"

// TestFindPletkaSumEntry_DraftPrefersDraftEntry pins that a draft vendored
// tree (version "") finds its own "draft" sum entry even when the same
// module is also vendored at a release version by another dependency.
func TestFindPletkaSumEntry_DraftPrefersDraftEntry(t *testing.T) {
	sum := []pletkaSumEntry{
		{Module: "pletka.io/orgs/x/projects/LA", Version: "0.1.0", TreeSHA: "rel"},
		{Module: "pletka.io/orgs/x/projects/LA", Version: "draft", TreeSHA: "drf"},
	}
	got, err := findPletkaSumEntry(sum, "pletka.io/orgs/x/projects/LA", "")
	if err != nil || got.TreeSHA != "drf" {
		t.Fatalf("want the draft entry, got %+v err=%v", got, err)
	}
	// a release lookup still matches exactly
	got, err = findPletkaSumEntry(sum, "pletka.io/orgs/x/projects/LA", "0.1.0")
	if err != nil || got.TreeSHA != "rel" {
		t.Fatalf("want the release entry, got %+v err=%v", got, err)
	}
	// legacy sums without a "draft" label: a single entry still matches by module
	one := sum[:1]
	if got, err := findPletkaSumEntry(one, "pletka.io/orgs/x/projects/LA", ""); err != nil || got.TreeSHA != "rel" {
		t.Fatalf("single entry fallback: %+v err=%v", got, err)
	}
	// two release entries and no draft stay ambiguous
	two := []pletkaSumEntry{sum[0], {Module: sum[0].Module, Version: "0.2.0", TreeSHA: "rel2"}}
	if _, err := findPletkaSumEntry(two, "pletka.io/orgs/x/projects/LA", ""); err == nil {
		t.Fatal("two releases and no draft must stay ambiguous")
	}
}
