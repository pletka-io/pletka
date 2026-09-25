package vocabconnector

import (
	"errors"
	"fmt"
	"testing"
)

// ErrDegraded must survive wrapping: a connector wraps it with context and
// the vocabulary service still has to recognize it, or a degraded lookup is
// reported to the caller as a hard failure.
func TestErrDegradedMatchesThroughWrapping(t *testing.T) {
	wrapped := fmt.Errorf("suggest %q: %w", "aat", ErrDegraded)
	if !errors.Is(wrapped, ErrDegraded) {
		t.Fatal("wrapped ErrDegraded no longer matches errors.Is")
	}
	if errors.Is(errors.New("some other failure"), ErrDegraded) {
		t.Fatal("an unrelated error matched ErrDegraded")
	}
}
