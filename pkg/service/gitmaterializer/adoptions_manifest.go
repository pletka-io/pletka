package gitmaterializer

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/canonical"
)

type adoptionsIndexManifest struct {
	SchemaVersion int                          `json:"schema_version"`
	Receipts      []adoptionsIndexReceiptEntry `json:"receipts"`
}

type adoptionsIndexReceiptEntry struct {
	EntityType string `json:"entity_type"`
	EntityID   string `json:"entity_id"`
	File       string `json:"file"`
}

type adoptionReceiptManifest struct {
	SchemaVersion int                   `json:"schema_version"`
	Adoption      adoptionReceiptRecord `json:"adoption"`
}

type adoptionReceiptRecord struct {
	EntityType string                   `json:"entity_type"`
	EntityID   string                   `json:"entity_id"`
	Source     adoptionReceiptSource    `json:"source"`
	Contexts   []adoptionReceiptContext `json:"contexts"`
}

type adoptionReceiptSource struct {
	ProjectID string `json:"project_id"`
	EntityID  string `json:"entity_id"`
	Version   string `json:"version,omitempty"`
}

type adoptionReceiptContext struct {
	ContextEntityType string         `json:"context_entity_type"`
	ContextEntityID   string         `json:"context_entity_id"`
	AdoptedAt         string         `json:"adopted_at,omitempty"`
	AdoptedBy         *manifestActor `json:"adopted_by,omitempty"`
}

type groupedAdoptionReceipt struct {
	EntityType      string
	EntityID        string
	SourceProjectID string
	SourceVersion   string
	Contexts        []domain.Adoption
}

func encodeAdoptionsIndexManifest(manifest adoptionsIndexManifest) ([]byte, error) {
	raw, err := json.Marshal(manifest)
	if err != nil {
		return nil, fmt.Errorf("marshal adoptions index to json: %w", err)
	}
	var normalized any
	if err := json.Unmarshal(raw, &normalized); err != nil {
		return nil, fmt.Errorf("normalize adoptions index json: %w", err)
	}
	payload, err := canonical.Encode(normalized)
	if err != nil {
		return nil, fmt.Errorf("encode adoptions index yaml: %w", err)
	}
	return payload, nil
}

func encodeAdoptionReceiptManifest(manifest adoptionReceiptManifest) ([]byte, error) {
	raw, err := json.Marshal(manifest)
	if err != nil {
		return nil, fmt.Errorf("marshal adoption receipt to json: %w", err)
	}
	var normalized any
	if err := json.Unmarshal(raw, &normalized); err != nil {
		return nil, fmt.Errorf("normalize adoption receipt json: %w", err)
	}
	payload, err := canonical.Encode(normalized)
	if err != nil {
		return nil, fmt.Errorf("encode adoption receipt yaml: %w", err)
	}
	return payload, nil
}

func (m *Materializer) writeAdoptionManifests(ctx context.Context, workDir, projectID string) error {
	adoptions, err := m.loadProjectAdoptions(ctx, projectID)
	if err != nil {
		return err
	}
	grouped := groupProjectAdoptions(adoptions)

	index := adoptionsIndexManifest{
		SchemaVersion: 1,
		Receipts:      make([]adoptionsIndexReceiptEntry, 0, len(grouped)),
	}
	for _, receipt := range grouped {
		file := adoptionReceiptFileName(receipt.EntityType, receipt.EntityID)
		index.Receipts = append(index.Receipts, adoptionsIndexReceiptEntry{
			EntityType: receipt.EntityType,
			EntityID:   receipt.EntityID,
			File:       file,
		})
		payload, err := m.encodeAdoptionReceipt(ctx, receipt)
		if err != nil {
			return err
		}
		if err := writeEntityFile(workDir, "adoptions/"+file, payload); err != nil {
			return err
		}
	}
	indexPayload, err := encodeAdoptionsIndexManifest(index)
	if err != nil {
		return err
	}
	return writeEntityFile(workDir, "adoptions/index.yaml", indexPayload)
}

func (m *Materializer) loadProjectAdoptions(ctx context.Context, projectID string) ([]domain.Adoption, error) {
	rows, err := m.pool.Query(ctx, `
		SELECT
			id,
			project_id,
			context_entity_type,
			context_entity_id,
			entity_type,
			source_project_id,
			source_entity_id,
			COALESCE(source_version, '') AS source_version,
			adopted_at,
			created_by_id
		FROM weave_adoptions
		WHERE project_id = $1
		ORDER BY entity_type ASC, source_entity_id ASC, context_entity_type ASC, context_entity_id ASC, adopted_at ASC
	`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list project adoptions for manifest: %w", err)
	}
	defer rows.Close()

	out := make([]domain.Adoption, 0)
	for rows.Next() {
		var row domain.Adoption
		var sourceVersion string
		if err := rows.Scan(
			&row.ID,
			&row.ProjectID,
			&row.ContextEntityType,
			&row.ContextEntityID,
			&row.EntityType,
			&row.SourceProjectID,
			&row.SourceEntityID,
			&sourceVersion,
			&row.AdoptedAt,
			&row.CreatedByID,
		); err != nil {
			return nil, fmt.Errorf("scan project adoption for manifest: %w", err)
		}
		row.SourceVersion = strings.TrimSpace(sourceVersion)
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate project adoptions for manifest: %w", err)
	}
	return out, nil
}

func groupProjectAdoptions(adoptions []domain.Adoption) []groupedAdoptionReceipt {
	type key struct {
		entityType      string
		entityID        string
		sourceProjectID string
		sourceVersion   string
	}
	grouped := make(map[key][]domain.Adoption)
	for _, adoption := range adoptions {
		k := key{
			entityType:      adoption.EntityType,
			entityID:        adoption.SourceEntityID,
			sourceProjectID: adoption.SourceProjectID,
			sourceVersion:   strings.TrimSpace(adoption.SourceVersion),
		}
		grouped[k] = append(grouped[k], adoption)
	}

	out := make([]groupedAdoptionReceipt, 0, len(grouped))
	for k, contexts := range grouped {
		sort.Slice(contexts, func(i, j int) bool {
			if contexts[i].ContextEntityType != contexts[j].ContextEntityType {
				return contexts[i].ContextEntityType < contexts[j].ContextEntityType
			}
			if contexts[i].ContextEntityID != contexts[j].ContextEntityID {
				return contexts[i].ContextEntityID < contexts[j].ContextEntityID
			}
			return contexts[i].AdoptedAt.Before(contexts[j].AdoptedAt)
		})
		out = append(out, groupedAdoptionReceipt{
			EntityType:      k.entityType,
			EntityID:        k.entityID,
			SourceProjectID: k.sourceProjectID,
			SourceVersion:   k.sourceVersion,
			Contexts:        contexts,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].EntityType != out[j].EntityType {
			return out[i].EntityType < out[j].EntityType
		}
		if out[i].EntityID != out[j].EntityID {
			return out[i].EntityID < out[j].EntityID
		}
		if out[i].SourceProjectID != out[j].SourceProjectID {
			return out[i].SourceProjectID < out[j].SourceProjectID
		}
		return out[i].SourceVersion < out[j].SourceVersion
	})
	return out
}

func (m *Materializer) encodeAdoptionReceipt(ctx context.Context, grouped groupedAdoptionReceipt) ([]byte, error) {
	contexts := make([]adoptionReceiptContext, 0, len(grouped.Contexts))
	for _, adoption := range grouped.Contexts {
		contexts = append(contexts, adoptionReceiptContext{
			ContextEntityType: adoption.ContextEntityType,
			ContextEntityID:   adoption.ContextEntityID,
			AdoptedAt:         formatManifestTime(adoption.AdoptedAt),
			AdoptedBy:         m.loadManifestActor(ctx, adoption.CreatedByID),
		})
	}
	payload, err := encodeAdoptionReceiptManifest(adoptionReceiptManifest{
		SchemaVersion: 1,
		Adoption: adoptionReceiptRecord{
			EntityType: grouped.EntityType,
			EntityID:   grouped.EntityID,
			Source: adoptionReceiptSource{
				ProjectID: grouped.SourceProjectID,
				EntityID:  grouped.EntityID,
				Version:   grouped.SourceVersion,
			},
			Contexts: contexts,
		},
	})
	if err != nil {
		return nil, err
	}
	return payload, nil
}

func adoptionReceiptFileName(entityType, entityID string) string {
	return fmt.Sprintf("%s-%s.yaml", entityType, entityID)
}

func formatManifestTime(ts time.Time) string {
	if ts.IsZero() {
		return ""
	}
	return ts.UTC().Format(time.RFC3339)
}
