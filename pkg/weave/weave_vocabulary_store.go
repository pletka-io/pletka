package weave

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/pletka-io/pletka/pkg/database/dbutil"
	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/ids"
	"github.com/jackc/pgx/v5"
)

// vocabularyStore implements domain.VocabularyStore backed by
// the weave_vocabularies and weave_vocabulary_entries tables via sqlc queries.
type vocabularyStore struct {
	queries *sqlcgen.Queries
}

// Compile-time interface check.
var _ domain.VocabularyStore = (*vocabularyStore)(nil)

// ---------------------------------------------------------------------------
// Row converters
// ---------------------------------------------------------------------------

// weaveRowToVocabulary converts a sqlcgen.WeaveVocabulary row to a *domain.Vocabulary.
func weaveRowToVocabulary(row sqlcgen.WeaveVocabulary) *domain.Vocabulary {
	return &domain.Vocabulary{
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
		ConnectorType: row.ConnectorType,
		BaseURI:       dbutil.NilToEmpty(row.BaseUri),
		Config:        row.Config,
	}
}

// weaveRowToVocabularyEntry converts a sqlcgen.WeaveVocabularyEntry row to a *domain.VocabularyEntry.
func weaveRowToVocabularyEntry(row sqlcgen.WeaveVocabularyEntry) *domain.VocabularyEntry {
	entry := &domain.VocabularyEntry{
		ID:           row.ID,
		VocabularyID: row.VocabularyID,
		URI:          row.Uri,
		ScopeNote:    unmarshalDomainTranslations(row.ScopeNote),
		BroaderURI:   dbutil.NilToEmpty(row.BroaderUri),
		BroaderPath:  unmarshalVocabularyEntryRefs(row.BroaderPathItems),
		ExternalID:   dbutil.NilToEmpty(row.ExternalID),
		CreatedAt:    row.CreatedAt,
		UpdatedAt:    row.UpdatedAt,
	}

	// Label is json.RawMessage in sqlc, unmarshal to domain.Translations.
	if len(row.Label) > 0 {
		var t domain.Translations
		if err := json.Unmarshal(row.Label, &t); err == nil {
			entry.Label = t
		}
	}

	return entry
}

// ---------------------------------------------------------------------------
// VocabularyStore implementation
// ---------------------------------------------------------------------------

// CreateVocabulary inserts a new vocabulary into the weave_vocabularies table.
func (s *vocabularyStore) CreateVocabulary(ctx context.Context, vocab *domain.Vocabulary) error {
	if vocab.ID == "" {
		vocab.ID = ids.GenerateULID()
	}

	if vocab.Status == "" {
		vocab.Status = "draft"
	}

	row, err := s.queries.WeaveCreateVocabulary(ctx, sqlcgen.WeaveCreateVocabularyParams{
		ID:            vocab.ID,
		SemanticID:    dbutil.EmptyToNil(vocab.SemanticID),
		SystemName:    dbutil.EmptyToNil(vocab.SystemName),
		UiName:        marshalDomainTranslations(vocab.UIName),
		Description:   marshalDomainTranslations(vocab.Description),
		Status:        string(vocab.Status),
		ProjectID:     vocab.ProjectID,
		ConnectorType: vocab.ConnectorType,
		BaseUri:       dbutil.EmptyToNil(vocab.BaseURI),
		Config:        vocab.Config,
	})
	if err != nil {
		return fmt.Errorf("create vocabulary: %w", err)
	}

	vocab.CreatedAt = row.CreatedAt
	vocab.UpdatedAt = row.UpdatedAt

	return nil
}

// GetVocabulary retrieves a vocabulary by its ULID. Returns nil, nil if not found.
func (s *vocabularyStore) GetVocabulary(ctx context.Context, id string) (*domain.Vocabulary, error) {
	row, err := s.queries.WeaveGetVocabulary(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get vocabulary: %w", err)
	}

	return weaveRowToVocabulary(row), nil
}

// ListVocabularies returns all vocabularies ordered by system_name.
func (s *vocabularyStore) ListVocabularies(ctx context.Context) ([]*domain.Vocabulary, error) {
	rows, err := s.queries.WeaveListVocabularies(ctx)
	if err != nil {
		return nil, fmt.Errorf("list vocabularies: %w", err)
	}

	result := make([]*domain.Vocabulary, 0, len(rows))
	for _, row := range rows {
		result = append(result, weaveRowToVocabulary(row))
	}

	return result, nil
}

// CreateEntry inserts a new vocabulary entry into the weave_vocabulary_entries table.
func (s *vocabularyStore) CreateEntry(ctx context.Context, entry *domain.VocabularyEntry) error {
	if entry.ID == "" {
		entry.ID = ids.GenerateULID()
	}

	row, err := s.queries.WeaveCreateVocabularyEntry(ctx, sqlcgen.WeaveCreateVocabularyEntryParams{
		ID:               entry.ID,
		VocabularyID:     entry.VocabularyID,
		Uri:              entry.URI,
		Label:            marshalJSON(entry.Label),
		ScopeNote:        marshalDomainTranslations(entry.ScopeNote),
		BroaderUri:       dbutil.EmptyToNil(entry.BroaderURI),
		BroaderPath:      marshalJSONArray([]string{}),
		BroaderPathItems: marshalJSONArray(entry.BroaderPath),
		ExternalID:       dbutil.EmptyToNil(entry.ExternalID),
	})
	if err != nil {
		return fmt.Errorf("create vocabulary entry: %w", err)
	}

	entry.CreatedAt = row.CreatedAt
	entry.UpdatedAt = row.UpdatedAt

	return nil
}

// GetEntry retrieves a vocabulary entry by its ULID. Returns nil, nil if not found.
func (s *vocabularyStore) GetEntry(ctx context.Context, id string) (*domain.VocabularyEntry, error) {
	row, err := s.queries.WeaveGetVocabularyEntry(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get vocabulary entry: %w", err)
	}

	return weaveRowToVocabularyEntry(row), nil
}

// SearchEntries searches vocabulary entries by label, URI, or external ID.
func (s *vocabularyStore) SearchEntries(ctx context.Context, vocabularyID string, query string) ([]*domain.VocabularyEntry, error) {
	rows, err := s.queries.WeaveSearchVocabularyEntries(ctx, sqlcgen.WeaveSearchVocabularyEntriesParams{
		VocabularyID: vocabularyID,
		Query:        query,
	})
	if err != nil {
		return nil, fmt.Errorf("search vocabulary entries: %w", err)
	}

	result := make([]*domain.VocabularyEntry, 0, len(rows))
	for _, row := range rows {
		result = append(result, weaveRowToVocabularyEntry(row))
	}

	return result, nil
}
