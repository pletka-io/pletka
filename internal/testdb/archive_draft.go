package testdb

import (
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ArchiveDraft copies a project's current draft rows into every content
// registry section at version, the way pre-preserve releases hold their
// archives. Tests seed drafts, archive them, then edit drafts to prove a
// preserved release does not follow. The release row must already exist;
// rows already archived are kept.
func ArchiveDraft(t *testing.T, pool *pgxpool.Pool, project, version string) {
	t.Helper()
	ctx := t.Context()
	rows, err := pool.Query(ctx, `SELECT archive_table, expected_sql FROM weave_content_sections_v1() ORDER BY (parent_table<>''), archive_table`)
	if err != nil {
		t.Fatal(err)
	}
	type section struct{ table, expected string }
	var sections []section
	for rows.Next() {
		var s section
		if err := rows.Scan(&s.table, &s.expected); err != nil {
			t.Fatal(err)
		}
		sections = append(sections, s)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	for _, s := range sections {
		table := pgx.Identifier{s.table}.Sanitize()
		// Column order of the projection is not the table's; populate by name.
		stmt := fmt.Sprintf(`INSERT INTO %s SELECT (jsonb_populate_record(NULL::%s, to_jsonb(r))).* FROM (%s) r ON CONFLICT DO NOTHING`, table, table, s.expected)
		if _, err := pool.Exec(ctx, stmt, project, version); err != nil {
			t.Fatalf("archive %s: %v", s.table, err)
		}
	}
}
