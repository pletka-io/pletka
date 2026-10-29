package gitmaterializer

import (
	"strings"
	"testing"
)

func TestRepointRejectsUntrustedRefInput(t *testing.T) {
	m := NewMaterializer(nil, t.TempDir(), nil)
	operation := strings.Repeat("a", 64)
	for _, ref := range []ReleaseRefState{
		{ProjectID: "../escape", Version: "1.0.0"},
		{ProjectID: "valid", Version: "1.0.0\ncreate refs/heads/main"},
		{ProjectID: "valid", Version: "1.0.0", Branch: strings.Repeat("a", 40) + "\n"},
		{ProjectID: "valid", Version: "1.0.0", Tag: strings.Repeat("z", 40)},
	} {
		if err := m.CheckReleaseRepoint(t.Context(), operation, ref, false); err == nil {
			t.Fatalf("unsafe refs accepted: %+v", ref)
		}
	}
	if err := m.CheckReleaseRepoint(t.Context(), "bad", ReleaseRefState{ProjectID: "valid", Version: "1.0.0"}, false); err == nil {
		t.Fatal("invalid operation accepted")
	}
}
