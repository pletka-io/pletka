package gitmaterializer

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/pletka-io/pletka/pkg/weave/canonical"
	"gopkg.in/yaml.v3"
)

// releasesIndexPath is the well-known repo-relative path for the releases
// index file: a top-to-bottom, chronologically-ordered ledger of every
// release ever cut for a project, read start-to-finish by the restore path.
const releasesIndexPath = "releases/index.yaml"

// releasesIndex is the decoded form of releases/index.yaml.
type releasesIndex struct {
	SchemaVersion int                  `json:"schema_version" yaml:"schema_version"`
	Releases      []releasesIndexEntry `json:"releases" yaml:"releases"`
}

// releasesIndexEntry is one release record in the index.
type releasesIndexEntry struct {
	Version         string `json:"version" yaml:"version"`
	Title           string `json:"title,omitempty" yaml:"title,omitempty"`
	Description     string `json:"description,omitempty" yaml:"description,omitempty"`
	CreatedAt       string `json:"created_at" yaml:"created_at"` // RFC3339Nano UTC
	CreatedBy       string `json:"created_by" yaml:"created_by"`
	Tag             string `json:"tag" yaml:"tag"` // "v" + Version
	ArchivedAt      string `json:"archived_at,omitempty" yaml:"archived_at,omitempty"`
	ArchivedMessage string `json:"archived_message,omitempty" yaml:"archived_message,omitempty"`
}

// encodeReleasesIndex serializes idx to canonical YAML using the same
// json-marshal-then-normalize pattern as encodeProjectManifest: struct ->
// json.Marshal -> json.Unmarshal into `any` -> canonical.Encode. This keeps
// key ordering and formatting identical across every manifest type in this
// package, which is what makes canonical.Encode's determinism guarantee hold.
func encodeReleasesIndex(idx releasesIndex) ([]byte, error) {
	raw, err := json.Marshal(idx)
	if err != nil {
		return nil, fmt.Errorf("marshal releases index to json: %w", err)
	}
	var normalized any
	if err := json.Unmarshal(raw, &normalized); err != nil {
		return nil, fmt.Errorf("normalize releases index json: %w", err)
	}
	payload, err := canonical.Encode(normalized)
	if err != nil {
		return nil, fmt.Errorf("encode releases index yaml: %w", err)
	}
	return payload, nil
}

// decodeReleasesIndex parses releases/index.yaml. yaml.v3 tolerates unknown
// fields (both top-level and per-entry) by default, which is deliberate:
// this file is embedded in every release commit and a future restore path
// must be able to read an index written by a newer schema version without
// erroring on fields it doesn't yet know about.
func decodeReleasesIndex(data []byte) (releasesIndex, error) {
	var idx releasesIndex
	if err := yaml.Unmarshal(data, &idx); err != nil {
		return releasesIndex{}, fmt.Errorf("decode releases index yaml: %w", err)
	}
	return idx, nil
}

// loadReleasesIndex builds the releases index for projectID from
// weave_releases, ordered chronologically (oldest first) since the index is
// read top-to-bottom on restore.
func (m *Materializer) loadReleasesIndex(ctx context.Context, projectID string) (releasesIndex, error) {
	rows, err := m.pool.Query(ctx, `
		SELECT version, title, description, created_at, created_by_id, archived_at, archived_message
		FROM weave_releases
		WHERE project_id = $1
		ORDER BY created_at ASC, version ASC
	`, projectID)
	if err != nil {
		return releasesIndex{}, fmt.Errorf("list releases for %s: %w", projectID, err)
	}
	defer rows.Close()

	entries := make([]releasesIndexEntry, 0)
	for rows.Next() {
		var (
			version, title, description, createdBy, archivedMessage string
			createdAt                                               time.Time
			archivedAt                                              *time.Time
		)
		if err := rows.Scan(&version, &title, &description, &createdAt, &createdBy, &archivedAt, &archivedMessage); err != nil {
			return releasesIndex{}, fmt.Errorf("scan release for %s: %w", projectID, err)
		}
		entry := releasesIndexEntry{
			Version:         version,
			Title:           title,
			Description:     description,
			CreatedAt:       createdAt.UTC().Format(time.RFC3339Nano),
			CreatedBy:       createdBy,
			Tag:             "v" + version,
			ArchivedMessage: archivedMessage,
		}
		if archivedAt != nil {
			entry.ArchivedAt = archivedAt.UTC().Format(time.RFC3339Nano)
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return releasesIndex{}, fmt.Errorf("iterate releases for %s: %w", projectID, err)
	}

	return releasesIndex{
		SchemaVersion: 1,
		Releases:      entries,
	}, nil
}
