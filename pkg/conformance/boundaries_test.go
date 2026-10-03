package conformance

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// The spec's one rule: ReadScopeFromContext is called at the request
// boundary -- an HTTP handler or an MCP tool -- and nowhere else. A service
// reaching into ctx for a scope is the ambient pattern wearing a new type,
// and it would drift exactly as ProjectVersionFromContext did.
func TestReadScopeFromContextOnlyAtRequestBoundaries(t *testing.T) {
	root := repoRoot(t)
	var offenders []string
	for _, f := range goFilesUnder(t, filepath.Join(root, "pkg")) {
		rel := mustRel(t, root, f)
		if isRequestBoundary(rel) || strings.HasSuffix(rel, "_test.go") || strings.HasPrefix(rel, "pkg/auth/") {
			continue
		}
		body, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		if strings.Contains(string(body), "ReadScopeFromContext") {
			offenders = append(offenders, rel)
		}
	}
	sort.Strings(offenders)
	if len(offenders) > 0 {
		t.Fatalf("ReadScopeFromContext outside a request boundary:\n  %s\n\n"+
			"Handlers and MCP tools resolve the scope and pass it as a parameter. "+
			"A service that reaches into ctx for it has reinvented the pattern this "+
			"replaced.", strings.Join(offenders, "\n  "))
	}
}

// ProjectVersionFromContext is the ambient accessor the read model replaces.
// It survives for unconverted callers and goes when the last one does, so
// the count may only fall.
func TestProjectVersionFromContextOnlyShrinks(t *testing.T) {
	// Measured 2026-10-03 over non-test files under pkg/, excluding pkg/auth
	// which defines the accessor. Lower this when a conversion removes call
	// sites. Never raise it.
	const baseline = 71

	root := repoRoot(t)
	var found []string
	for _, f := range goFilesUnder(t, filepath.Join(root, "pkg")) {
		rel := mustRel(t, root, f)
		if strings.HasSuffix(rel, "_test.go") || strings.HasPrefix(rel, "pkg/auth/") {
			continue
		}
		body, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		for i := 0; i < strings.Count(string(body), "ProjectVersionFromContext"); i++ {
			found = append(found, rel)
		}
	}
	if len(found) > baseline {
		sort.Strings(found)
		t.Fatalf("ProjectVersionFromContext call sites rose to %d (baseline %d):\n  %s\n\n"+
			"This accessor is being retired. A new call site means a new unscoped read.",
			len(found), baseline, strings.Join(found, "\n  "))
	}
	if len(found) < baseline {
		t.Logf("ProjectVersionFromContext is down to %d from a baseline of %d -- "+
			"lower the baseline in this test to lock the progress in.", len(found), baseline)
	}
}

// isRequestBoundary reports whether a file is a place the spec permits the
// accessor: an HTTP handler or an MCP tool.
func isRequestBoundary(rel string) bool {
	base := filepath.Base(rel)
	switch {
	case strings.HasPrefix(base, "handler"), strings.HasSuffix(base, "_handler.go"):
		return true
	case strings.HasPrefix(base, "routes"):
		return true
	case strings.Contains(rel, "/mcp/"):
		return true
	}
	return false
}

func goFilesUnder(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, ".go") {
			out = append(out, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", dir, err)
	}
	return out
}

func mustRel(t *testing.T, base, path string) string {
	t.Helper()
	rel, err := filepath.Rel(base, path)
	if err != nil {
		t.Fatalf("rel %s: %v", path, err)
	}
	return filepath.ToSlash(rel)
}
