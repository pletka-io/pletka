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
		if _, ok := claimed[tbl]; ok {
			continue
		}
		if reason, ok := ArchiveTablesWithoutASlice[tbl]; ok {
			if len(strings.TrimSpace(reason)) < 30 {
				t.Errorf("%s is recorded as belonging to no slice, but the reason is too short to be one", tbl)
			}
			continue
		}
		unclaimed = append(unclaimed, tbl)
	}
	sort.Strings(unclaimed)
	if len(unclaimed) > 0 {
		t.Fatalf("archive tables claimed by no slice in VersionedSlices: %v\n"+
			"A table with an _archive counterpart belongs to a versioned slice. "+
			"Add it to the owning slice in slices.go, add the slice, or record it in "+
			"ArchiveTablesWithoutASlice with the reason it has no slice.", unclaimed)
	}
}

// The migrations use both "CREATE TABLE public.x_archive" and the
// unqualified form, 16 and 8 times respectively. A regex that misses the
// qualified majority makes this whole file decorative: it would claim every
// table is accounted for while never seeing most of them.
func TestArchiveTableRegexMatchesBothDeclarationForms(t *testing.T) {
	const fixture = `
CREATE TABLE public.weave_things_archive (id text);
CREATE TABLE weave_other_archive (id text);
CREATE TABLE IF NOT EXISTS public.weave_third_archive (id text);
create table if not exists weave_fourth_archive (id text);
CREATE TABLE weave_not_an_arch (id text);
CREATE INDEX idx_x ON public.weave_things_archive (id);
`
	want := map[string]bool{
		"weave_things_archive": true,
		"weave_other_archive":  true,
		"weave_third_archive":  true,
		"weave_fourth_archive": true,
	}
	got := map[string]bool{}
	for _, m := range createArchiveTable.FindAllStringSubmatch(fixture, -1) {
		got[strings.ToLower(m[1])] = true
	}
	for tbl := range want {
		if !got[tbl] {
			t.Errorf("regex missed %s", tbl)
		}
	}
	for tbl := range got {
		if !want[tbl] {
			t.Errorf("regex matched %s, which is not an archive table declaration", tbl)
		}
	}
}

// The repo has 24 archive tables today. A regex finding far fewer is the
// failure this file exists to prevent, and it is silent: the claim check
// passes because it never sees them.
func TestArchiveTableScanFindsThemAll(t *testing.T) {
	const atLeast = 24
	if got := len(archiveTablesInMigrations(t)); got < atLeast {
		t.Errorf("found %d archive tables in the migrations, expected at least %d -- "+
			"the scan is missing declaration forms, so the registry is unchecked against most of the schema", got, atLeast)
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

// The schema qualifier is optional and usually present: 16 of the repo's 24
// archive tables are declared as "public.x_archive". A class without the
// dot silently skipped all of them.
var createArchiveTable = regexp.MustCompile(`(?i)create\s+table\s+(?:if\s+not\s+exists\s+)?(?:[a-z0-9_]+\.)?([a-z0-9_]*_archive)\b`)

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
