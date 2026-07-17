package canonical

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestCanonicalEncode_SortedKeys(t *testing.T) {
	input := map[string]any{
		"zebra": "last",
		"alpha": "first",
		"nested": map[string]any{
			"z_key": 2,
			"a_key": 1,
		},
	}

	got, err := Encode(input)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	want := "alpha: first\nnested:\n  a_key: 1\n  z_key: 2\nzebra: last\n"

	if diff := cmp.Diff(want, string(got)); diff != "" {
		t.Errorf("sorted keys mismatch (-want +got):\n%s", diff)
	}
}

func TestCanonicalEncode_Idempotent(t *testing.T) {
	input := map[string]any{
		"name": map[string]string{
			"nl": "Dutch",
			"en": "English",
		},
		"count": 42,
		"items": []any{"b", "a"},
	}

	first, err := Encode(input)
	if err != nil {
		t.Fatalf("first Encode: %v", err)
	}

	second, err := Encode(input)
	if err != nil {
		t.Fatalf("second Encode: %v", err)
	}

	if diff := cmp.Diff(string(first), string(second)); diff != "" {
		t.Errorf("idempotent encode mismatch (-first +second):\n%s", diff)
	}
}

func TestHash_Stable(t *testing.T) {
	payload := []byte("hello canonical\n")
	got := Hash(payload)

	// SHA-256 of "hello canonical\n"
	want := "0e0649279641a4f732fe9c9c8d0edc2c1bc0c8f715baa65c0533cf4589583bf3"

	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("hash mismatch (-want +got):\n%s", diff)
	}
}
