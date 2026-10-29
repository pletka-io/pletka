package release

import (
	"os"
	"strings"
	"testing"
)

func TestRebuildLockCoversCaptureLedger(t *testing.T) {
	ledger := loadCaptureLedger(t)
	data, err := os.ReadFile("../../database/queries/weave_release_rebuild.sql")
	if err != nil {
		t.Fatal(err)
	}
	_, locks, ok := strings.Cut(string(data), "LOCK TABLE ")
	if !ok {
		t.Fatal("rebuild must lock before its first snapshot-taking statement")
	}
	locks, _, _ = strings.Cut(locks, " IN SHARE ROW EXCLUSIVE MODE;")
	locked := map[string]bool{}
	for _, table := range strings.Split(locks, ",") {
		locked[strings.TrimSpace(table)] = true
	}
	want := map[string]bool{}
	for table := range ledger.OtherTables {
		if strings.HasPrefix(table, "weave_") {
			want[table] = true
		}
	}
	for _, s := range ledger.Sections {
		want[s.LiveTable] = true
		want[s.ArchiveTable] = true
	}
	for table := range want {
		if !locked[table] {
			t.Errorf("rebuild does not lock %s", table)
		}
	}
	for table := range locked {
		if !want[table] {
			t.Errorf("unexpected rebuild table %s", table)
		}
	}
}
