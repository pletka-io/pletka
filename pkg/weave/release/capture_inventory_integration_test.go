//go:build integration

package release

import (
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

// Check migrations themselves, so stale generated models cannot hide a new
// column, table, ownership decision or broken archive identity.
func TestCaptureInventoryMatchesMigratedSchema(t *testing.T) {
	pool := releaseTestPool(t)
	ledger := loadCaptureLedger(t)
	tables := map[string]string{}
	classify := func(table, reason string) {
		t.Helper()
		if previous, exists := tables[table]; exists {
			t.Fatalf("duplicate table classification for %s: %s / %s", table, previous, reason)
		}
		tables[table] = reason
	}
	for _, section := range ledger.Sections {
		classify(section.LiveTable, "owned content")
		classify(section.ArchiveTable, "owned archive")
		t.Run(section.LiveTable, func(t *testing.T) {
			for table, want := range map[string][]string{
				section.LiveTable:    strings.Fields(section.LiveColumns),
				section.ArchiveTable: sortedKeys(section.Projection),
			} {
				rows, err := pool.Query(t.Context(), `SELECT column_name FROM information_schema.columns WHERE table_schema='public' AND table_name=$1`, table)
				if err != nil {
					t.Fatal(err)
				}
				actual, err := pgx.CollectRows(rows, pgx.RowTo[string])
				if err != nil {
					t.Fatal(err)
				}
				checkColumns(t, table, actual, want)
			}
			// Some archives use unique indexes rather than primary constraints.
			// Partial indexes and INCLUDE columns cannot establish full identity.
			rows, err := pool.Query(t.Context(), `SELECT array_agg(a.attname::text ORDER BY k.ordinality)
				FROM pg_index i JOIN pg_class c ON c.oid=i.indrelid
				JOIN pg_namespace n ON n.oid=c.relnamespace
				CROSS JOIN LATERAL unnest(i.indkey) WITH ORDINALITY k(attnum,ordinality)
				JOIN pg_attribute a ON a.attrelid=c.oid AND a.attnum=k.attnum
				WHERE n.nspname='public' AND c.relname=$1 AND i.indisunique AND i.indisvalid
				  AND i.indpred IS NULL AND i.indexprs IS NULL AND k.ordinality<=i.indnkeyatts
				GROUP BY i.indexrelid`, section.ArchiveTable)
			if err != nil {
				t.Fatal(err)
			}
			keys, err := pgx.CollectRows(rows, pgx.RowTo[[]string])
			if err != nil {
				t.Fatal(err)
			}
			want := strings.Fields(section.ArchiveKey)
			slices.Sort(want)
			found := false
			for _, key := range keys {
				slices.Sort(key)
				found = found || slices.Equal(key, want)
			}
			if !found {
				t.Fatalf("%s lacks full unique identity %v: %v", section.ArchiveTable, want, keys)
			}
		})
	}
	for table, reason := range ledger.OtherTables {
		classify(table, reason)
	}
	rows, err := pool.Query(t.Context(), `SELECT tablename FROM pg_tables WHERE schemaname='public'`)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatal(err)
	}
	checkColumns(t, "application tables", actual, sortedKeys(tables))
}
