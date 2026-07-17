package gitmaterializer

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/canonical"
)

type forksIndexManifest struct {
	SchemaVersion int                      `json:"schema_version"`
	Receipts      []forksIndexReceiptEntry `json:"receipts"`
}

type forksIndexReceiptEntry struct {
	EntityType string `json:"entity_type"`
	EntityID   string `json:"entity_id"`
	File       string `json:"file"`
}

type forkReceiptManifest struct {
	SchemaVersion int               `json:"schema_version"`
	Fork          forkReceiptRecord `json:"fork"`
}

type forkReceiptRecord struct {
	EntityType string            `json:"entity_type"`
	EntityID   string            `json:"entity_id"`
	Source     forkReceiptSource `json:"source"`
	ForkedAt   string            `json:"forked_at,omitempty"`
	CreatedBy  *manifestActor    `json:"created_by,omitempty"`
}

type forkReceiptSource struct {
	ProjectID string `json:"project_id"`
	EntityID  string `json:"entity_id"`
	Version   string `json:"version,omitempty"`
}

func encodeForksIndexManifest(manifest forksIndexManifest) ([]byte, error) {
	raw, err := json.Marshal(manifest)
	if err != nil {
		return nil, fmt.Errorf("marshal forks index to json: %w", err)
	}
	var normalized any
	if err := json.Unmarshal(raw, &normalized); err != nil {
		return nil, fmt.Errorf("normalize forks index json: %w", err)
	}
	payload, err := canonical.Encode(normalized)
	if err != nil {
		return nil, fmt.Errorf("encode forks index yaml: %w", err)
	}
	return payload, nil
}

func encodeForkReceiptManifest(manifest forkReceiptManifest) ([]byte, error) {
	raw, err := json.Marshal(manifest)
	if err != nil {
		return nil, fmt.Errorf("marshal fork receipt to json: %w", err)
	}
	var normalized any
	if err := json.Unmarshal(raw, &normalized); err != nil {
		return nil, fmt.Errorf("normalize fork receipt json: %w", err)
	}
	payload, err := canonical.Encode(normalized)
	if err != nil {
		return nil, fmt.Errorf("encode fork receipt yaml: %w", err)
	}
	return payload, nil
}

func (m *Materializer) writeForkManifests(ctx context.Context, workDir, projectID string) error {
	forks, err := m.loadProjectForks(ctx, projectID)
	if err != nil {
		return err
	}
	index := forksIndexManifest{
		SchemaVersion: 1,
		Receipts:      make([]forksIndexReceiptEntry, 0, len(forks)),
	}
	for _, fork := range forks {
		file := forkReceiptFileName(fork.EntityType, fork.ForkEntityID)
		index.Receipts = append(index.Receipts, forksIndexReceiptEntry{
			EntityType: fork.EntityType,
			EntityID:   fork.ForkEntityID,
			File:       file,
		})
		payload, err := m.encodeForkReceipt(ctx, fork)
		if err != nil {
			return err
		}
		if err := writeEntityFile(workDir, "forks/"+file, payload); err != nil {
			return err
		}
	}
	indexPayload, err := encodeForksIndexManifest(index)
	if err != nil {
		return err
	}
	return writeEntityFile(workDir, "forks/index.yaml", indexPayload)
}

func (m *Materializer) loadProjectForks(ctx context.Context, projectID string) ([]domain.EntityFork, error) {
	rows, err := m.pool.Query(ctx, `
		SELECT
			id,
			project_id,
			entity_type,
			fork_entity_id,
			source_project_id,
			source_entity_id,
			COALESCE(source_version, '') AS source_version,
			forked_at,
			created_by_id
		FROM weave_entity_forks
		WHERE project_id = $1
		ORDER BY entity_type ASC, fork_entity_id ASC
	`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list project forks for manifest: %w", err)
	}
	defer rows.Close()

	out := make([]domain.EntityFork, 0)
	for rows.Next() {
		var row domain.EntityFork
		var sourceVersion string
		if err := rows.Scan(
			&row.ID,
			&row.ProjectID,
			&row.EntityType,
			&row.ForkEntityID,
			&row.SourceProjectID,
			&row.SourceEntityID,
			&sourceVersion,
			&row.ForkedAt,
			&row.CreatedByID,
		); err != nil {
			return nil, fmt.Errorf("scan project fork for manifest: %w", err)
		}
		row.SourceVersion = strings.TrimSpace(sourceVersion)
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate project forks for manifest: %w", err)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].EntityType != out[j].EntityType {
			return out[i].EntityType < out[j].EntityType
		}
		return out[i].ForkEntityID < out[j].ForkEntityID
	})
	return out, nil
}

func (m *Materializer) encodeForkReceipt(ctx context.Context, fork domain.EntityFork) ([]byte, error) {
	return encodeForkReceiptManifest(forkReceiptManifest{
		SchemaVersion: 1,
		Fork: forkReceiptRecord{
			EntityType: fork.EntityType,
			EntityID:   fork.ForkEntityID,
			Source: forkReceiptSource{
				ProjectID: fork.SourceProjectID,
				EntityID:  fork.SourceEntityID,
				Version:   strings.TrimSpace(fork.SourceVersion),
			},
			ForkedAt:  formatManifestTime(fork.ForkedAt),
			CreatedBy: m.loadManifestActor(ctx, fork.CreatedByID),
		},
	})
}

func forkReceiptFileName(entityType, entityID string) string {
	return fmt.Sprintf("%s-%s.yaml", entityType, entityID)
}
