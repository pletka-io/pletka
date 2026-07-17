package i18n_test

import (
	"testing"

	"github.com/pletka-io/pletka/pkg/i18n"
)

func TestPluralization(t *testing.T) {
	manager, err := newMemoryManager(i18n.Config{})
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	// Test with explicit plural forms
	err = manager.Set("items_found", "en", &i18n.Translation{
		Value: "1 item found|{count} items found",
		PluralForms: map[i18n.PluralForm]string{
			i18n.PluralOne:   "1 item found",
			i18n.PluralOther: "{count} items found",
		},
		Status: i18n.StatusApproved,
	})
	if err != nil {
		t.Fatalf("Failed to set translation: %v", err)
	}

	// Test singular
	result := manager.TN("items_found", 1, "en")
	expected := "1 item found"
	if result != expected {
		t.Errorf("Expected '%s', got '%s'", expected, result)
	}

	// Test plural
	result = manager.TN("items_found", 5, "en")
	expected = "5 items found"
	if result != expected {
		t.Errorf("Expected '%s', got '%s'", expected, result)
	}

	// Test zero
	result = manager.TN("items_found", 0, "en")
	expected = "0 items found"
	if result != expected {
		t.Errorf("Expected '%s', got '%s'", expected, result)
	}
}
