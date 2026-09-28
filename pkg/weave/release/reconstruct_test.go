package release

import (
	"strings"
	"testing"
)

// TestReconstructInsertsMatchSnapshotStatements pins the one coupling this
// file cannot express in the type system: the reconstruction inserts are
// hand-written copies of the seven snapshot statements in service.go, with a
// timestamp predicate added. If someone adds a column to a snapshot
// statement, or narrows an ON CONFLICT target, or changes which expression
// feeds a column, the reconstruction must move with it — a reconstruction
// that inserts fewer columns silently writes an incomplete release, a
// narrower conflict target silently drops rows, and a SELECT list that
// drifted writes the wrong CONTENT into the right columns.
//
// The SELECT list matters as much as the column list and is easier to get
// wrong invisibly: swapping two same-typed columns (title/description, both
// jsonb) or dropping the weave_vocabularies status normalisation type-checks
// in Postgres and passes a column-list comparison, while corrupting every
// archived row.
//
// No database: this compares the SQL text of the two statement sets.
func TestReconstructInsertsMatchSnapshotStatements(t *testing.T) {
	snapshots := map[string]string{}
	for _, stmt := range snapshotStatements {
		if table, ok := insertTarget(stmt); ok {
			snapshots[table] = stmt
		}
	}

	for _, tbl := range reconstructTables {
		t.Run(tbl.archive, func(t *testing.T) {
			snap, ok := snapshots[tbl.archive]
			if !ok {
				t.Fatalf("no snapshot statement inserts into %s — the reconstruction would fill an archive the release snapshot does not", tbl.archive)
			}

			wantCols, ok := insertColumns(snap)
			if !ok {
				t.Fatalf("could not parse the snapshot statement's column list for %s", tbl.archive)
			}
			gotCols, ok := insertColumns(tbl.insert)
			if !ok {
				t.Fatalf("could not parse the reconstruction insert's column list for %s", tbl.archive)
			}
			if gotCols != wantCols {
				t.Errorf("column list drifted from snapshotStatements:\n  snapshot:      %s\n  reconstruct:   %s", wantCols, gotCols)
			}

			wantTarget, ok := conflictTarget(snap)
			if !ok {
				t.Fatalf("snapshot statement for %s has no ON CONFLICT target", tbl.archive)
			}
			gotTarget, ok := conflictTarget(tbl.insert)
			if !ok {
				t.Fatalf("reconstruction insert for %s has no ON CONFLICT target — re-running --apply would duplicate rows", tbl.archive)
			}
			if gotTarget != wantTarget {
				t.Errorf("ON CONFLICT target drifted from snapshotStatements:\n  snapshot:      %s\n  reconstruct:   %s", wantTarget, gotTarget)
			}

			wantSelect, ok := selectList(snap)
			if !ok {
				t.Fatalf("could not parse the snapshot statement's SELECT list for %s", tbl.archive)
			}
			gotSelect, ok := selectList(tbl.insert)
			if !ok {
				t.Fatalf("could not parse the reconstruction insert's SELECT list for %s", tbl.archive)
			}
			if gotSelect != wantSelect {
				t.Errorf("SELECT list drifted from snapshotStatements — the reconstruction would write different CONTENT into the same columns:\n  snapshot:      %s\n  reconstruct:   %s", wantSelect, gotSelect)
			}
		})
	}
}

// insertTarget returns the archive table an INSERT statement writes to.
func insertTarget(stmt string) (string, bool) {
	_, rest, ok := strings.Cut(stmt, "INSERT INTO ")
	if !ok {
		return "", false
	}
	name, _, ok := strings.Cut(rest, " (")
	if !ok {
		return "", false
	}
	return strings.TrimSpace(name), true
}

// insertColumns returns an INSERT's column list, whitespace-normalized.
func insertColumns(stmt string) (string, bool) {
	_, rest, ok := strings.Cut(stmt, "INSERT INTO ")
	if !ok {
		return "", false
	}
	_, rest, ok = strings.Cut(rest, "(")
	if !ok {
		return "", false
	}
	cols, _, ok := strings.Cut(rest, ")")
	if !ok {
		return "", false
	}
	return strings.Join(strings.Fields(cols), " "), true
}

// conflictTarget returns an INSERT's ON CONFLICT target, whitespace-normalized.
func conflictTarget(stmt string) (string, bool) {
	_, rest, ok := strings.Cut(stmt, "ON CONFLICT (")
	if !ok {
		return "", false
	}
	target, _, ok := strings.Cut(rest, ")")
	if !ok {
		return "", false
	}
	return strings.Join(strings.Fields(target), " "), true
}

// selectList returns an INSERT's SELECT list — everything between SELECT and
// the first FROM — with whitespace collapsed, and nothing else normalized.
//
// Alias qualifiers are deliberately NOT stripped. The snapshot and
// reconstruction SELECTs already carry the same join aliases, so stripping
// bought nothing and cost the guard its point: in the two joined statements
// both tables have same-named created_at/updated_at columns, so "e.created_at"
// and "v.created_at" normalize to the same text once the qualifier is gone.
// Reading the joined parent's timestamps instead of the row's own is a real
// content bug that a stripping comparison passes silently — exactly the class
// this guard exists to catch.
//
// The version placeholder is likewise left alone: $2 is the version in both
// statement sets, and $3 (the release timestamp) appears only in the
// reconstruction's WHERE clause, so a $3 reaching a SELECT list would be
// genuine drift worth failing on.
//
// None of the seven SELECT lists contains a nested FROM, so cutting at the
// first one is exact for this statement set.
func selectList(stmt string) (string, bool) {
	_, rest, ok := strings.Cut(stmt, "INSERT INTO ")
	if !ok {
		return "", false
	}
	_, rest, ok = strings.Cut(rest, "(")
	if !ok {
		return "", false
	}
	_, rest, ok = strings.Cut(rest, ")")
	if !ok {
		return "", false
	}
	_, rest, ok = strings.Cut(rest, "SELECT")
	if !ok {
		return "", false
	}
	list, _, ok := strings.Cut(rest, "FROM")
	if !ok {
		return "", false
	}

	return strings.Join(strings.Fields(list), " "), true
}
