package release

import (
	"strings"
	"testing"
)

// TestReconstructInsertsMatchSnapshotStatements pins the one coupling this
// file cannot express in the type system: the reconstruction inserts are
// hand-written copies of the seven snapshot statements in service.go, with a
// timestamp predicate added. If someone adds a column to a snapshot
// statement, or narrows an ON CONFLICT target, the reconstruction must move
// with it — a reconstruction that inserts fewer columns silently writes an
// incomplete release, and a narrower conflict target silently drops rows.
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
