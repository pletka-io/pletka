package conformance

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The spec's scope rule is "a slice owning a table with an _archive
// counterpart is a versioned slice and must be converted" -- checkable
// rather than judged. It is only checkable if the registry cannot fall
// behind the schema, so every archive table in the migrations must be
// claimed by exactly one registered slice. A new archive table therefore
// fails this test until someone decides which slice owns it.
func TestEveryArchiveTableIsClaimedByASlice(t *testing.T) {
	claimed := map[string]string{} // table -> slice
	for _, s := range VersionedSlices {
		for _, tbl := range s.ArchiveTables {
			if prev, dup := claimed[tbl]; dup {
				t.Fatalf("archive table %s is claimed by both %s and %s", tbl, prev, s.Name)
			}
			claimed[tbl] = s.Name
		}
	}

	var unclaimed []string
	for _, tbl := range archiveTablesInMigrations(t) {
		if _, ok := claimed[tbl]; !ok {
			unclaimed = append(unclaimed, tbl)
		}
	}
	sort.Strings(unclaimed)
	if len(unclaimed) > 0 {
		t.Fatalf("archive tables claimed by no slice in VersionedSlices: %v\n"+
			"A table with an _archive counterpart belongs to a versioned slice. "+
			"Add it to the owning slice in slices.go, or add the slice.", unclaimed)
	}
}

func TestEveryRegisteredSliceExists(t *testing.T) {
	for _, s := range VersionedSlices {
		path := filepath.Join(repoRoot(t), filepath.FromSlash(s.Pkg), "service.go")
		if _, err := os.Stat(path); err != nil {
			t.Errorf("slice %s registers %s but %s is missing: %v", s.Name, s.Pkg, path, err)
		}
	}
}

var createArchiveTable = regexp.MustCompile(`(?i)create\s+table\s+(?:if\s+not\s+exists\s+)?([a-z0-9_]*_archive)\b`)

func archiveTablesInMigrations(t *testing.T) []string {
	t.Helper()
	dir := filepath.Join(repoRoot(t), "pkg", "database", "migrations")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read migrations: %v", err)
	}
	seen := map[string]bool{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		body, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		for _, m := range createArchiveTable.FindAllStringSubmatch(string(body), -1) {
			seen[strings.ToLower(m[1])] = true
		}
	}
	out := make([]string, 0, len(seen))
	for tbl := range seen {
		out = append(out, tbl)
	}
	sort.Strings(out)
	return out
}

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(wd, "go.mod")); err == nil {
			return wd
		}
		parent := filepath.Dir(wd)
		if parent == wd {
			t.Fatal("go.mod not found above the working directory")
		}
		wd = parent
	}
}
