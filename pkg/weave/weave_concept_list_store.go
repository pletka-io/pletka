package weave

import (
	"context"
	"errors"
	"fmt"

	"github.com/pletka-io/pletka/pkg/database/dbutil"
	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/ids"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// conceptListStore implements domain.ConceptListStore backed by
// the weave_concept_lists and weave_concept_list_entries tables via sqlc queries.
type conceptListStore struct {
	queries *sqlcgen.Queries
	pool    *pgxpool.Pool
}

// Compile-time interface check.
var _ domain.ConceptListStore = (*conceptListStore)(nil)

// ---------------------------------------------------------------------------
// Row converters
// ---------------------------------------------------------------------------

// weaveRowToConceptList converts a sqlcgen.WeaveConceptList row to a *domain.ConceptList.
func weaveRowToConceptList(row sqlcgen.WeaveConceptList) *domain.ConceptList {
	return &domain.ConceptList{
		Entity: domain.Entity{
			ID:          row.ID,
			CreatedAt:   row.CreatedAt,
			UpdatedAt:   row.UpdatedAt,
			SemanticID:  dbutil.NilToEmpty(row.SemanticID),
			SystemName:  dbutil.NilToEmpty(row.SystemName),
			UIName:      unmarshalDomainTranslations(row.UiName),
			Description: unmarshalDomainTranslations(row.Description),
			Status:      domain.Status(row.Status),
			ProjectID:   row.ProjectID,
		},
		ListType:     row.ListType,
		VocabularyID: row.VocabularyID,
		IsClosed:     row.IsClosed,
	}
}

// weaveRowToConceptListEntry converts a sqlcgen.WeaveConceptListEntry row to a *domain.ConceptListEntry.
func weaveRowToConceptListEntry(row sqlcgen.WeaveConceptListEntry) *domain.ConceptListEntry {
	return &domain.ConceptListEntry{
		ID:                row.ID,
		ConceptListID:     row.ConceptListID,
		VocabularyEntryID: row.VocabularyEntryID,
		Position:          int(row.Position),
		CustomLabel:       unmarshalDomainTranslations(row.CustomLabel),
		CreatedAt:         row.CreatedAt,
		UpdatedAt:         row.UpdatedAt,
	}
}

// ---------------------------------------------------------------------------
// ConceptListStore implementation
// ---------------------------------------------------------------------------

// Create inserts a new concept list into the weave_concept_lists table.
func (s *conceptListStore) Create(ctx context.Context, list *domain.ConceptList) error {
	if list.ID == "" {
		list.ID = ids.GenerateULID()
	}

	if list.Status == "" {
		list.Status = "draft"
	}

	row, err := s.queries.WeaveCreateConceptList(ctx, sqlcgen.WeaveCreateConceptListParams{
		ID:           list.ID,
		SemanticID:   dbutil.EmptyToNil(list.SemanticID),
		SystemName:   dbutil.EmptyToNil(list.SystemName),
		UiName:       marshalDomainTranslations(list.UIName),
		Description:  marshalDomainTranslations(list.Description),
		Status:       string(list.Status),
		ProjectID:    list.ProjectID,
		ListType:     list.ListType,
		VocabularyID: list.VocabularyID,
	})
	if err != nil {
		return fmt.Errorf("create concept list: %w", err)
	}

	list.CreatedAt = row.CreatedAt
	list.UpdatedAt = row.UpdatedAt

	return nil
}

// GetByID retrieves a concept list by its ULID. Returns nil, nil if not found.
func (s *conceptListStore) GetByID(ctx context.Context, id string) (*domain.ConceptList, error) {
	row, err := s.queries.WeaveGetConceptList(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get concept list: %w", err)
	}

	return weaveRowToConceptList(row), nil
}

// List returns all concept lists for a project ordered by system_name.
func (s *conceptListStore) List(ctx context.Context, projectID string) ([]*domain.ConceptList, error) {
	rows, err := s.queries.WeaveListConceptLists(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("list concept lists: %w", err)
	}

	result := make([]*domain.ConceptList, 0, len(rows))
	for _, row := range rows {
		result = append(result, weaveRowToConceptList(row))
	}

	return result, nil
}

// FindByListType returns all concept lists with a given list_type.
func (s *conceptListStore) FindByListType(ctx context.Context, listTypeID string) ([]*domain.ConceptList, error) {
	rows, err := s.queries.WeaveFindConceptListsByListType(ctx, dbutil.EmptyToNil(listTypeID))
	if err != nil {
		return nil, fmt.Errorf("find concept lists by list type: %w", err)
	}

	result := make([]*domain.ConceptList, 0, len(rows))
	for _, row := range rows {
		result = append(result, weaveRowToConceptList(row))
	}

	return result, nil
}

// AddEntry inserts a new entry into a concept list.
func (s *conceptListStore) AddEntry(ctx context.Context, entry *domain.ConceptListEntry) error {
	if entry.ID == "" {
		entry.ID = ids.GenerateULID()
	}

	row, err := s.queries.WeaveCreateConceptListEntry(ctx, sqlcgen.WeaveCreateConceptListEntryParams{
		ID:                entry.ID,
		ConceptListID:     entry.ConceptListID,
		VocabularyEntryID: entry.VocabularyEntryID,
		Position:          int32(entry.Position),
		CustomLabel:       marshalDomainTranslations(entry.CustomLabel),
	})
	if err != nil {
		return fmt.Errorf("add concept list entry: %w", err)
	}

	entry.CreatedAt = row.CreatedAt
	entry.UpdatedAt = row.UpdatedAt

	return nil
}

// RemoveEntry deletes an entry from a concept list.
func (s *conceptListStore) RemoveEntry(ctx context.Context, id string) error {
	if err := s.queries.WeaveDeleteConceptListEntry(ctx, id); err != nil {
		return fmt.Errorf("remove concept list entry: %w", err)
	}
	return nil
}

// ListEntries returns all entries for a concept list ordered by position.
func (s *conceptListStore) ListEntries(ctx context.Context, conceptListID string) ([]*domain.ConceptListEntry, error) {
	rows, err := s.queries.WeaveListConceptListEntries(ctx, conceptListID)
	if err != nil {
		return nil, fmt.Errorf("list concept list entries: %w", err)
	}

	result := make([]*domain.ConceptListEntry, 0, len(rows))
	for _, row := range rows {
		result = append(result, weaveRowToConceptListEntry(row))
	}

	return result, nil
}

// ReorderEntries updates position for a list of entry IDs (1-indexed).
func (s *conceptListStore) ReorderEntries(ctx context.Context, conceptListID string, entryIDs []string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	qtx := s.queries.WithTx(tx)

	// Fetch all entries for validation.
	rows, err := qtx.WeaveListConceptListEntries(ctx, conceptListID)
	if err != nil {
		return fmt.Errorf("reorder: list concept list entries: %w", err)
	}

	owned := make(map[string]bool, len(rows))
	for _, row := range rows {
		owned[row.ID] = true
	}

	for i, entryID := range entryIDs {
		if !owned[entryID] {
			return fmt.Errorf("reorder: entry %s not found in concept list %s", entryID, conceptListID)
		}

		if err := qtx.WeaveUpdateConceptListEntryOrder(ctx, sqlcgen.WeaveUpdateConceptListEntryOrderParams{
			ID:       entryID,
			Position: int32(i + 1),
		}); err != nil {
			return fmt.Errorf("reorder: update entry %s: %w", entryID, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}
