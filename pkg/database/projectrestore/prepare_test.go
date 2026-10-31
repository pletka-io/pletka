package projectrestore

import (
	"context"
	"testing"

	"github.com/pletka-io/pletka/pkg/projectbundle"
)

func TestRestoreRejectsZeroInputBeforeDatabase(t *testing.T) {
	result, err := Restore(context.Background(), nil, projectbundle.ImportedProject{}, projectbundle.Catalog{})
	if err == nil || result.Committed {
		t.Fatalf("invalid import reached database: %+v %v", result, err)
	}
}
