package conformance

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// The gate: no Go file may carry SQL as a string literal unless the allowlist
// says how much it currently carries and why.
//
// It fails three ways, and each is a different mistake:
//
//   - a file not in the allowlist holds SQL      -> new inline SQL
//   - an allowlisted file holds MORE than listed -> growth in existing debt
//   - an allowlisted file holds FEWER            -> the number is stale after
//     a conversion, and leaving it high would let the debt grow back silently
//
// The third is the one that keeps a shrinking list honest. It is a failure
// with a fix in the message, not an error: lower the number.
func TestNoInlineSQLOutsideNamedQueries(t *testing.T) {
	root := repoRoot(t)

	type finding struct {
		path  string
		found int
		want  int
		why   string
	}
	var newSQL, grew, stale []finding
	seen := map[string]bool{}

	err := filepath.Walk(filepath.Join(root, "pkg"), func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(p, ".go") {
			return err
		}
		rel := mustRel(t, root, p)
		if generatedOrExempt(rel) {
			return nil
		}
		n, perr := countInlineSQL(p)
		if perr != nil {
			t.Fatalf("parse %s: %v", rel, perr)
		}
		ex, listed := InlineSQLAllowlist[rel]
		if n == 0 {
			if listed {
				stale = append(stale, finding{rel, 0, ex.Statements, ex.Reason})
			}
			return nil
		}
		seen[rel] = true
		switch {
		case !listed:
			newSQL = append(newSQL, finding{rel, n, 0, ""})
		case n > ex.Statements:
			grew = append(grew, finding{rel, n, ex.Statements, ex.Reason})
		case n < ex.Statements:
			stale = append(stale, finding{rel, n, ex.Statements, ex.Reason})
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}

	// An allowlist entry for a file that no longer exists is also stale.
	for rel, ex := range InlineSQLAllowlist {
		if _, err := os.Stat(filepath.Join(root, rel)); os.IsNotExist(err) {
			stale = append(stale, finding{rel, -1, ex.Statements, "file is gone"})
		}
	}

	report := func(label string, fs []finding, advice string) {
		if len(fs) == 0 {
			return
		}
		sort.Slice(fs, func(i, j int) bool { return fs[i].path < fs[j].path })
		var b strings.Builder
		fmt.Fprintf(&b, "%s:\n", label)
		for _, f := range fs {
			switch {
			case f.found == -1:
				fmt.Fprintf(&b, "  %s — listed but the file no longer exists\n", f.path)
			case f.want == 0 && f.why == "":
				fmt.Fprintf(&b, "  %s — %d SQL literal(s), not in the allowlist\n", f.path, f.found)
			default:
				fmt.Fprintf(&b, "  %s — found %d, allowlist says %d (%s)\n", f.path, f.found, f.want, f.why)
			}
		}
		b.WriteString("\n" + advice)
		t.Error(b.String())
	}

	report("New inline SQL", newSQL,
		"Write the query in pkg/database/queries/<area>.sql and run `go tool sqlc generate`,\n"+
			"then call the generated method from the slice's store_postgres.go. sqlc checks the\n"+
			"query against the migrations; a Go string is checked by nothing until it runs.\n"+
			"If the SQL genuinely cannot be a named query, add an allowlist entry saying why.")

	report("Inline SQL grew in a file that already had debt", grew,
		"An allowlist number is a ceiling, not a budget to spend. Add the new query as a\n"+
			"named query instead; the existing statements convert separately.")

	report("Allowlist numbers are stale", stale,
		"Lower (or remove) these entries in pkg/conformance/inline_sql_allowlist.go.\n"+
			"A number left high after a conversion lets the debt grow back unnoticed, which\n"+
			"is the one thing a shrinking list must not allow.")
}

// Every entry must name a reason, and a permanent one must be a deliberate
// choice rather than a number nobody revisited.
func TestInlineSQLAllowlistEntriesNameAReason(t *testing.T) {
	for path, ex := range InlineSQLAllowlist {
		if strings.TrimSpace(ex.Reason) == "" {
			t.Errorf("%s: allowlisted with no reason; an exemption nobody can explain is a bug waiting to be inherited", path)
		}
		if ex.Statements < 0 {
			t.Errorf("%s: negative statement ceiling %d", path, ex.Statements)
		}
	}
}
