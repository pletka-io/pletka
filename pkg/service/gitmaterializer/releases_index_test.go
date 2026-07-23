package gitmaterializer

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestReleasesIndexRoundTrip(t *testing.T) {
	idx := releasesIndex{
		SchemaVersion: 1,
		Releases: []releasesIndexEntry{
			{
				Version:   "1.0.0",
				Title:     "First release",
				CreatedAt: "2026-01-01T00:00:00Z",
				CreatedBy: "ACT1",
				Tag:       "v1.0.0",
			},
			{
				Version:         "1.1.0",
				Title:           "Second release",
				Description:     "Adds foo",
				CreatedAt:       "2026-02-01T00:00:00Z",
				CreatedBy:       "ACT1",
				Tag:             "v1.1.0",
				ArchivedAt:      "2026-03-01T00:00:00Z",
				ArchivedMessage: "superseded",
			},
		},
	}

	data, err := encodeReleasesIndex(idx)
	if err != nil {
		t.Fatalf("encodeReleasesIndex: %v", err)
	}
	got, err := decodeReleasesIndex(data)
	if err != nil {
		t.Fatalf("decodeReleasesIndex: %v", err)
	}
	if diff := cmp.Diff(idx, got); diff != "" {
		t.Fatalf("round trip mismatch (-want +got):\n%s", diff)
	}

	// Empty archived fields must be absent from the encoded YAML for a
	// non-archived entry.
	oneEntry := releasesIndex{
		SchemaVersion: 1,
		Releases: []releasesIndexEntry{
			{
				Version:   "1.0.0",
				Title:     "First release",
				CreatedAt: "2026-01-01T00:00:00Z",
				CreatedBy: "ACT1",
				Tag:       "v1.0.0",
			},
		},
	}
	oneData, err := encodeReleasesIndex(oneEntry)
	if err != nil {
		t.Fatalf("encodeReleasesIndex (one entry): %v", err)
	}
	if strings.Contains(string(oneData), "archived_at") {
		t.Fatalf("expected archived_at absent from non-archived entry, got:\n%s", oneData)
	}
	if strings.Contains(string(oneData), "archived_message") {
		t.Fatalf("expected archived_message absent from non-archived entry, got:\n%s", oneData)
	}
}

func TestReleasesIndexEncodeDeterministic(t *testing.T) {
	idx := releasesIndex{
		SchemaVersion: 1,
		Releases: []releasesIndexEntry{
			{
				Version:   "1.0.0",
				Title:     "First release",
				CreatedAt: "2026-01-01T00:00:00Z",
				CreatedBy: "ACT1",
				Tag:       "v1.0.0",
			},
			{
				Version:         "2.0.0",
				CreatedAt:       "2026-04-01T00:00:00Z",
				CreatedBy:       "ACT2",
				Tag:             "v2.0.0",
				ArchivedAt:      "2026-05-01T00:00:00Z",
				ArchivedMessage: "retired",
			},
		},
	}

	got, err := encodeReleasesIndex(idx)
	if err != nil {
		t.Fatalf("encodeReleasesIndex: %v", err)
	}
	gotAgain, err := encodeReleasesIndex(idx)
	if err != nil {
		t.Fatalf("encodeReleasesIndex second pass: %v", err)
	}
	if string(got) != string(gotAgain) {
		t.Fatalf("encodeReleasesIndex is not deterministic across repeated calls:\n%s\n---\n%s", got, gotAgain)
	}
}

func TestReleasesIndexDecodeToleratesUnknownFields(t *testing.T) {
	data := []byte(`
schema_version: 1
future_top_level_field: something-new
releases:
  - version: "1.0.0"
    title: First release
    created_at: "2026-01-01T00:00:00Z"
    created_by: ACT1
    tag: v1.0.0
    future_entry_field: surprise
`)

	got, err := decodeReleasesIndex(data)
	if err != nil {
		t.Fatalf("decodeReleasesIndex with unknown fields: %v", err)
	}

	want := releasesIndex{
		SchemaVersion: 1,
		Releases: []releasesIndexEntry{
			{
				Version:   "1.0.0",
				Title:     "First release",
				CreatedAt: "2026-01-01T00:00:00Z",
				CreatedBy: "ACT1",
				Tag:       "v1.0.0",
			},
		},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("decode with unknown fields mismatch (-want +got):\n%s", diff)
	}
}
