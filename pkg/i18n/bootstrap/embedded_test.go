package bootstrap

import (
	"io"
	"log/slog"
	"testing"
)

func TestNewEmbeddedManagerLoadsLanguagesAndTranslations(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	manager, err := NewEmbeddedManager(logger)
	if err != nil {
		t.Fatalf("NewEmbeddedManager: %v", err)
	}

	if got := len(manager.Languages()); got == 0 {
		t.Fatal("expected enabled languages")
	}
	if got := manager.T("common.country", "en"); got != "Country" {
		t.Fatalf("common.country en = %q, want Country", got)
	}
}
