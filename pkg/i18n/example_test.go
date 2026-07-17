package i18n_test

import (
	"fmt"
	"log/slog"
	"testing"

	"github.com/pletka-io/pletka/pkg/i18n"
)

func ExampleNew() {
	// Simple memory-based setup
	manager, err := newMemoryManager(i18n.Config{})

	if err != nil {
		panic(err)
	}

	// Add some test translations
	manager.Set("welcome", "en", &i18n.Translation{
		Value:  "Welcome, %s!",
		Status: i18n.StatusApproved,
	})

	manager.Set("welcome", "nl", &i18n.Translation{
		Value:  "Welkom, %s!",
		Status: i18n.StatusApproved,
	})

	// Add pluralized translation
	manager.Set("items_found", "en", &i18n.Translation{
		Value: "1 item found|{count} items found",
		PluralForms: map[i18n.PluralForm]string{
			i18n.PluralOne:   "1 item found",
			i18n.PluralOther: "{count} items found",
		},
		Status: i18n.StatusApproved,
	})

	// Use translations
	fmt.Println(manager.T("welcome", "en", "John"))
	fmt.Println(manager.T("welcome", "nl", "John"))
	fmt.Println(manager.TN("items_found", 1, "en"))
	fmt.Println(manager.TN("items_found", 5, "en"))

	// Output:
	// Welcome, John!
	// Welkom, John!
	// 1 item found
	// 5 items found
}

func TestBasicUsage(t *testing.T) {
	manager, err := newMemoryManager(i18n.Config{DebugMode: true})

	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	// Test missing key in debug mode
	result := manager.T("missing_key", "en")
	if result != "missing_key_missing_key" {
		t.Errorf("Expected debug missing key, got: %s", result)
	}

	// Add translation and test
	err = manager.Set("test_key", "en", &i18n.Translation{
		Value:  "Test value",
		Status: i18n.StatusApproved,
	})
	if err != nil {
		t.Fatalf("Failed to set translation: %v", err)
	}

	result = manager.T("test_key", "en")
	if result != "Test value" {
		t.Errorf("Expected 'Test value', got: %s", result)
	}
}

func TestCacheHitMiss(t *testing.T) {
	manager, err := newMemoryManager(i18n.Config{})

	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	// Add translation
	err = manager.Set("cached_key", "en", &i18n.Translation{
		Value:  "Cached value",
		Status: i18n.StatusApproved,
	})
	if err != nil {
		t.Fatalf("Failed to set translation: %v", err)
	}

	// Repeated calls should resolve from storage consistently.
	result1 := manager.T("cached_key", "en")
	result2 := manager.T("cached_key", "en")

	if result1 != "Cached value" || result2 != "Cached value" {
		t.Errorf("Translation results don't match: %s vs %s", result1, result2)
	}
}

func BenchmarkTranslation(b *testing.B) {
	manager, err := newMemoryManager(i18n.Config{
		Logger: slog.New(slog.NewTextHandler(&discardWriter{}, &slog.HandlerOptions{Level: slog.LevelError})),
	})

	if err != nil {
		b.Fatalf("Failed to create manager: %v", err)
	}

	// Setup test data
	manager.Set("bench_key", "en", &i18n.Translation{
		Value:  "Benchmark value",
		Status: i18n.StatusApproved,
	})

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = manager.T("bench_key", "en")
		}
	})
}

// discardWriter discards all writes (for benchmarking)
type discardWriter struct{}

func (d *discardWriter) Write(p []byte) (n int, err error) {
	return len(p), nil
}
