package ontologysnapshot_test

import (
	"testing"

	"github.com/pletka-io/pletka/pkg/database/ontologysnapshot"
)

func TestRestoreValidatesDetachedInputsBeforeAcquiringDatabase(t *testing.T) {
	// A nil pool makes an accidental database acquisition fail this regression.
	if err := ontologysnapshot.Restore(t.Context(), nil, []ontologysnapshot.Artifact{{JSON: "{}", Digest: "invalid"}}); err == nil {
		t.Fatal("invalid input bypassed pure preflight")
	}
}
