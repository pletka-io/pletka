package weave

import (
	"testing"

	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
)

// TestWeaveRowToConceptList_MapsIsClosed guards a regression: the detailview
// reads is_closed through this store, so the converter must carry it — else a
// sealed list never reads as sealed in the UI (#3599).
func TestWeaveRowToConceptList_MapsIsClosed(t *testing.T) {
	for _, closed := range []bool{true, false} {
		got := weaveRowToConceptList(sqlcgen.WeaveConceptList{ID: "CL", IsClosed: closed})
		if got.IsClosed != closed {
			t.Fatalf("IsClosed not mapped: row=%v got=%v", closed, got.IsClosed)
		}
	}
}
