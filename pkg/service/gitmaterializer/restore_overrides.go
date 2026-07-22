package gitmaterializer

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/jackc/pgx/v5"
)

type canonicalOverrideDoc struct {
	AdoptedFrom        string                    `json:"_adopted_from,omitempty"`
	FieldID            string                    `json:"field_id"`
	EntityType         string                    `json:"entity_type,omitempty"`
	EntityID           string                    `json:"entity_id,omitempty"`
	Position           int                       `json:"position,omitempty"`
	CollectionOrder    int                       `json:"collection_order,omitempty"`
	DisplayName        map[string]string         `json:"display_name,omitempty"`
	Description        map[string]string         `json:"description,omitempty"`
	CollectionName     map[string]string         `json:"collection_name,omitempty"`
	CategoryID         string                    `json:"category_id,omitempty"`
	PartOfCollectionID string                    `json:"part_of_collection_id,omitempty"`
	SetValue           string                    `json:"set_value,omitempty"`
	IsRequired         bool                      `json:"is_required,omitempty"`
	MinOccurs          int                       `json:"min_occurs,omitempty"`
	MaxOccurs          *int                      `json:"max_occurs,omitempty"`
	IsHidden           bool                      `json:"is_hidden,omitempty"`
	Visibility         string                    `json:"visibility,omitempty"`
	Refs               []canonicalOverrideRefDoc `json:"refs,omitempty"`
}

type canonicalOverrideRefDoc struct {
	RefType    string `json:"ref_type"`
	SemanticID string `json:"semantic_id"`
	Position   int    `json:"position,omitempty"`
}

func (m *Materializer) HydrateProjectOverrides(ctx context.Context, plan *RestorePlan) error {
	if plan == nil || plan.Snapshot == nil {
		return fmt.Errorf("hydrate project overrides: missing snapshot")
	}
	projectID := strings.TrimSpace(plan.ProjectID)
	if projectID == "" {
		return fmt.Errorf("hydrate project overrides: missing project id")
	}

	tx, err := m.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("hydrate project overrides: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	q := m.queries.WithTx(tx)
	if err := clearProjectOverrideState(ctx, tx, projectID); err != nil {
		return err
	}
	if err := m.hydrateBaseOverridesTx(ctx, q, projectID, plan.Snapshot.Entities.BaseOverrides); err != nil {
		return err
	}
	if err := m.hydrateScopedOverridesTx(ctx, q, projectID, "model", plan.Snapshot.Entities.ModelOverrides); err != nil {
		return err
	}
	if err := m.hydrateScopedOverridesTx(ctx, q, projectID, "collection", plan.Snapshot.Entities.CollectionOverrides); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("hydrate project overrides: commit tx: %w", err)
	}
	return nil
}

func (m *Materializer) HydrateProjectProvenance(ctx context.Context, plan *RestorePlan) error {
	if plan == nil || plan.Snapshot == nil {
		return fmt.Errorf("hydrate provenance: missing snapshot")
	}
	projectID := strings.TrimSpace(plan.ProjectID)
	if projectID == "" {
		return fmt.Errorf("hydrate provenance: missing project id")
	}

	tx, err := m.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("hydrate provenance: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := clearProjectAdoptionsAndForks(ctx, tx, projectID); err != nil {
		return err
	}
	if err := m.hydrateAdoptionsTx(ctx, tx, projectID, plan.Snapshot.Adoptions); err != nil {
		return err
	}
	if err := m.hydrateForksTx(ctx, tx, projectID, plan.Snapshot.Forks); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("hydrate provenance: commit tx: %w", err)
	}
	return nil
}

func (m *Materializer) HydrateProjectOverridesAndProvenance(ctx context.Context, plan *RestorePlan) error {
	if err := m.HydrateProjectOverrides(ctx, plan); err != nil {
		return err
	}
	if err := m.HydrateProjectProvenance(ctx, plan); err != nil {
		return err
	}
	return nil
}

func clearProjectOverrideState(ctx context.Context, tx pgx.Tx, projectID string) error {
	if _, err := tx.Exec(ctx, `
		DELETE FROM weave_override_refs
		WHERE override_id IN (
			SELECT id FROM weave_field_overrides WHERE project_id = $1
		)
	`, projectID); err != nil {
		return fmt.Errorf("hydrate project overrides: clear override refs: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM weave_field_overrides WHERE project_id = $1`, projectID); err != nil {
		return fmt.Errorf("hydrate project overrides: clear overrides: %w", err)
	}
	return nil
}

func clearProjectAdoptionsAndForks(ctx context.Context, tx pgx.Tx, projectID string) error {
	if _, err := tx.Exec(ctx, `DELETE FROM weave_adoptions WHERE project_id = $1`, projectID); err != nil {
		return fmt.Errorf("hydrate provenance: clear adoptions: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM weave_entity_forks WHERE project_id = $1`, projectID); err != nil {
		return fmt.Errorf("hydrate provenance: clear forks: %w", err)
	}
	return nil
}

func (m *Materializer) hydrateBaseOverridesTx(ctx context.Context, q *sqlcgen.Queries, projectID string, items []SnapshotEntityFile) error {
	for _, item := range items {
		doc, err := decodeCanonicalYAML[canonicalOverrideDoc](item.Payload)
		if err != nil {
			return fmt.Errorf("hydrate base override %s: decode payload: %w", item.Path, err)
		}
		if strings.TrimSpace(doc.AdoptedFrom) != "" {
			continue
		}
		fieldID, err := resolveFieldIdentifier(ctx, q, projectID, firstNonEmpty(doc.FieldID, item.FieldID))
		if err != nil {
			return fmt.Errorf("hydrate base override %s: %w", item.Path, err)
		}
		categoryID, err := resolveCategoryIdentifier(ctx, q, projectID, doc.CategoryID)
		if err != nil {
			return fmt.Errorf("hydrate base override %s: %w", item.Path, err)
		}
		row, err := q.WeaveUpsertBaseOverride(ctx, sqlcgen.WeaveUpsertBaseOverrideParams{
			FieldID:            fieldID,
			ProjectID:          projectID,
			Position:           int32(doc.Position),
			CollectionOrder:    int32(doc.CollectionOrder),
			DisplayName:        marshalStringTranslationsForRestore(doc.DisplayName),
			Description:        marshalStringTranslationsForRestore(doc.Description),
			CollectionName:     marshalStringTranslationsForRestore(doc.CollectionName),
			CategoryID:         nullableString(categoryID),
			PartOfCollectionID: nullableString(doc.PartOfCollectionID),
			ExpectedValueType:  nil,
			SetValue:           nullableString(doc.SetValue),
			IsRequired:         boolPtr(doc.IsRequired),
			MinOccurs:          int32Ptr(doc.MinOccurs),
			MaxOccurs:          int32FromIntPtr(doc.MaxOccurs),
			IsHidden:           boolPtr(doc.IsHidden),
			Visibility:         nullableString(doc.Visibility),
			ContentHash:        nil,
		})
		if err != nil {
			return fmt.Errorf("hydrate base override %s: upsert: %w", item.Path, err)
		}
		if err := setOverrideRefsTx(ctx, q, row.ID, doc.Refs); err != nil {
			return fmt.Errorf("hydrate base override %s: set refs: %w", item.Path, err)
		}
	}
	return nil
}

func (m *Materializer) hydrateScopedOverridesTx(ctx context.Context, q *sqlcgen.Queries, projectID, entityType string, items []SnapshotEntityFile) error {
	grouped := make(map[string][]SnapshotEntityFile)
	for _, item := range items {
		grouped[item.OwnerID] = append(grouped[item.OwnerID], item)
	}
	for ownerID, files := range grouped {
		overrides := make([]sqlcgen.WeaveCreateOverrideParams, 0, len(files))
		refSets := make([][]canonicalOverrideRefDoc, 0, len(files))
		for _, item := range files {
			doc, err := decodeCanonicalYAML[canonicalOverrideDoc](item.Payload)
			if err != nil {
				return fmt.Errorf("hydrate %s override %s: decode payload: %w", entityType, item.Path, err)
			}
			if strings.TrimSpace(doc.AdoptedFrom) != "" {
				continue
			}
			fieldID, err := resolveFieldIdentifier(ctx, q, projectID, firstNonEmpty(doc.FieldID, item.FieldID))
			if err != nil {
				return fmt.Errorf("hydrate %s override %s: %w", entityType, item.Path, err)
			}
			categoryID, err := resolveCategoryIdentifier(ctx, q, projectID, doc.CategoryID)
			if err != nil {
				return fmt.Errorf("hydrate %s override %s: %w", entityType, item.Path, err)
			}
			entityID := firstNonEmpty(doc.EntityID, item.OwnerID, ownerID)
			overrides = append(overrides, sqlcgen.WeaveCreateOverrideParams{
				FieldID:            fieldID,
				ProjectID:          projectID,
				EntityType:         entityType,
				EntityID:           entityID,
				Position:           int32(doc.Position),
				CollectionOrder:    int32(doc.CollectionOrder),
				DisplayName:        marshalStringTranslationsForRestore(doc.DisplayName),
				Description:        marshalStringTranslationsForRestore(doc.Description),
				CollectionName:     marshalStringTranslationsForRestore(doc.CollectionName),
				CategoryID:         nullableString(categoryID),
				PartOfCollectionID: nullableString(doc.PartOfCollectionID),
				ExpectedValueType:  nil,
				SetValue:           nullableString(doc.SetValue),
				IsRequired:         boolPtr(doc.IsRequired),
				MinOccurs:          int32Ptr(doc.MinOccurs),
				MaxOccurs:          int32FromIntPtr(doc.MaxOccurs),
				IsHidden:           boolPtr(doc.IsHidden),
				Visibility:         nullableString(doc.Visibility),
				ContentHash:        nil,
			})
			refSets = append(refSets, doc.Refs)
		}
		if len(overrides) == 0 {
			continue
		}
		if err := q.WeaveDeleteOverridesForEntity(ctx, sqlcgen.WeaveDeleteOverridesForEntityParams{
			EntityType: entityType,
			EntityID:   ownerID,
		}); err != nil {
			return fmt.Errorf("hydrate %s overrides for %s: clear entity overrides: %w", entityType, ownerID, err)
		}
		for i, params := range overrides {
			row, err := q.WeaveCreateOverride(ctx, params)
			if err != nil {
				return fmt.Errorf("hydrate %s overrides for %s: create override %d: %w", entityType, ownerID, i, err)
			}
			if err := setOverrideRefsTx(ctx, q, row.ID, refSets[i]); err != nil {
				return fmt.Errorf("hydrate %s overrides for %s: set refs %d: %w", entityType, ownerID, i, err)
			}
		}
	}
	return nil
}

func setOverrideRefsTx(ctx context.Context, q *sqlcgen.Queries, overrideID int64, refs []canonicalOverrideRefDoc) error {
	if err := q.WeaveDeleteOverrideRefs(ctx, overrideID); err != nil {
		return err
	}
	for _, ref := range refs {
		semanticID := strings.TrimSpace(ref.SemanticID)
		if semanticID == "" {
			continue
		}
		if err := q.WeaveCreateOverrideRef(ctx, sqlcgen.WeaveCreateOverrideRefParams{
			OverrideID: overrideID,
			RefType:    strings.TrimSpace(ref.RefType),
			TargetID:   semanticID,
			SemanticID: semanticID,
			Position:   int32(ref.Position),
		}); err != nil {
			return err
		}
	}
	return nil
}

func resolveFieldIdentifier(ctx context.Context, q *sqlcgen.Queries, projectID, identifier string) (string, error) {
	identifier = strings.TrimSpace(identifier)
	if identifier == "" {
		return "", fmt.Errorf("missing field identifier")
	}
	row, err := q.WeaveGetFieldByIdentifier(ctx, sqlcgen.WeaveGetFieldByIdentifierParams{
		SemanticID: nullableString(identifier),
		ProjectID:  projectID,
	})
	if err == nil {
		return row.ID, nil
	}
	if err != pgx.ErrNoRows {
		return "", fmt.Errorf("get field %s: %w", identifier, err)
	}
	// Not in the referencing project: a vendored parent's override can point
	// at another project's field (SRD -> LAF.10). Semantic ids and ULIDs are
	// globally unique, so fall back to a project-agnostic lookup; project-
	// scoped system names stay excluded to avoid ambiguous matches.
	global, err := q.WeaveGetFieldByGlobalIdentifier(ctx, nullableString(identifier))
	if err != nil {
		if err == pgx.ErrNoRows {
			return "", fmt.Errorf("field %s not found", identifier)
		}
		return "", fmt.Errorf("get field %s: %w", identifier, err)
	}
	return global.ID, nil
}

// resolveCategoryIdentifier re-resolves a category reference captured in a
// snapshot against the categories just restored in this project, mirroring
// resolveFieldIdentifier: categories are always re-created with a fresh ULID
// during restore (hydrateCategoryFile in restore_entities.go), so a
// category_id copied verbatim from the snapshot would dangle. Unlike field
// identifiers, category_id is optional (an override can be uncategorized),
// so an empty identifier is not an error — it stays empty.
func resolveCategoryIdentifier(ctx context.Context, q *sqlcgen.Queries, projectID, identifier string) (string, error) {
	identifier = strings.TrimSpace(identifier)
	if identifier == "" {
		return "", nil
	}
	row, err := q.WeaveGetCategoryByIdentifier(ctx, sqlcgen.WeaveGetCategoryByIdentifierParams{
		SemanticID: nullableString(identifier),
		ProjectID:  projectID,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return "", fmt.Errorf("category %s not found", identifier)
		}
		return "", fmt.Errorf("get category %s: %w", identifier, err)
	}
	return row.ID, nil
}

func (m *Materializer) hydrateAdoptionsTx(ctx context.Context, tx pgx.Tx, projectID string, set *AdoptionManifestSet) error {
	if set == nil {
		return nil
	}
	for _, entry := range set.Index.Receipts {
		receipt, ok := set.Receipts[filepathToSlash(entry.File)]
		if !ok {
			return fmt.Errorf("hydrate adoptions: receipt %s not loaded", entry.File)
		}
		for _, ctxRow := range receipt.Adoption.Contexts {
			adoptedAt, err := parseManifestTime(ctxRow.AdoptedAt)
			if err != nil {
				return fmt.Errorf("hydrate adoptions: parse adopted_at for %s: %w", entry.File, err)
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO weave_adoptions (
					project_id, context_entity_type, context_entity_id,
					entity_type, source_project_id, source_entity_id, source_version,
					adopted_at, created_by_id
				) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
			`, projectID, ctxRow.ContextEntityType, ctxRow.ContextEntityID, receipt.Adoption.EntityType, receipt.Adoption.Source.ProjectID, receipt.Adoption.Source.EntityID, receipt.Adoption.Source.Version, adoptedAt, actorIDPtrFromManifest(ctxRow.AdoptedBy)); err != nil {
				return fmt.Errorf("hydrate adoptions: insert %s: %w", entry.File, err)
			}
		}
	}
	return nil
}

func (m *Materializer) hydrateForksTx(ctx context.Context, tx pgx.Tx, projectID string, set *ForkManifestSet) error {
	if set == nil {
		return nil
	}
	for _, entry := range set.Index.Receipts {
		receipt, ok := set.Receipts[filepathToSlash(entry.File)]
		if !ok {
			return fmt.Errorf("hydrate forks: receipt %s not loaded", entry.File)
		}
		forkedAt, err := parseManifestTime(receipt.Fork.ForkedAt)
		if err != nil {
			return fmt.Errorf("hydrate forks: parse forked_at for %s: %w", entry.File, err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO weave_entity_forks (
				project_id, entity_type, fork_entity_id,
				source_project_id, source_entity_id, source_version,
				forked_at, created_by_id
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		`, projectID, receipt.Fork.EntityType, receipt.Fork.EntityID, receipt.Fork.Source.ProjectID, receipt.Fork.Source.EntityID, receipt.Fork.Source.Version, forkedAt, actorIDPtrFromManifest(receipt.Fork.CreatedBy)); err != nil {
			return fmt.Errorf("hydrate forks: insert %s: %w", entry.File, err)
		}
	}
	return nil
}

func parseManifestTime(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Now().UTC(), nil
	}
	return time.Parse(time.RFC3339, raw)
}

func marshalStringTranslationsForRestore(t map[string]string) []byte {
	if t == nil {
		return nil
	}
	payload, _ := json.Marshal(t)
	return payload
}

func boolPtr(v bool) *bool {
	return &v
}

func int32FromIntPtr(v *int) *int32 {
	if v == nil {
		return nil
	}
	out := int32(*v)
	return &out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func filepathToSlash(path string) string {
	return strings.ReplaceAll(path, "\\", "/")
}
