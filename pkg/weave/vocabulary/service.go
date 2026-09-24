package vocabulary

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/ids"
	"github.com/pletka-io/pletka/pkg/weave/vocabconnector"
	"github.com/pletka-io/pletka/pkg/weave/vocabconnector/registry"
)

const defaultSearchLimit = 50

type Service struct {
	pool     *pgxpool.Pool
	queries  *sqlcgen.Queries
	registry *registry.Registry
	numberer EntityNumberer
}

type EntityNumberer interface {
	AllocateEntityNumber(ctx context.Context, projectID, kind string) (int64, error)
}

func NewService(pool *pgxpool.Pool, reg *registry.Registry, numberer ...EntityNumberer) *Service {
	if reg == nil {
		reg = registry.New(nil)
	}
	var n EntityNumberer
	if len(numberer) > 0 {
		n = numberer[0]
	}
	return &Service{pool: pool, queries: sqlcgen.New(pool), registry: reg, numberer: n}
}

type VocabularyView struct {
	ID            string              `json:"id"`
	SemanticID    string              `json:"semantic_id,omitempty"`
	SystemName    string              `json:"system_name,omitempty"`
	UIName        domain.Translations `json:"ui_name,omitempty"`
	Description   domain.Translations `json:"description,omitempty"`
	Status        string              `json:"status"`
	ProjectID     string              `json:"project_id,omitempty"`
	ConnectorType string              `json:"connector_type"`
	BaseURI       string              `json:"base_uri,omitempty"`
	CreatedAt     time.Time           `json:"created_at,omitempty"`
	UpdatedAt     time.Time           `json:"updated_at,omitempty"`
}

type AdminVocabularyView struct {
	VocabularyView
	EntryCount   int `json:"entry_count"`
	ProjectCount int `json:"project_count"`
}

type VocabularyEntryView struct {
	ID               string                      `json:"id"`
	VocabularyID     string                      `json:"vocabulary_id"`
	URI              string                      `json:"uri"`
	Label            domain.Translations         `json:"label,omitempty"`
	ScopeNote        domain.Translations         `json:"scope_note,omitempty"`
	BroaderURI       string                      `json:"broader_uri,omitempty"`
	BroaderPath      []string                    `json:"broader_path,omitempty"`
	BroaderPathItems []domain.VocabularyEntryRef `json:"broader_path_items,omitempty"`
	ExternalID       string                      `json:"external_id,omitempty"`
	Hydrated         bool                        `json:"hydrated"`
	CreatedAt        time.Time                   `json:"created_at,omitempty"`
	UpdatedAt        time.Time                   `json:"updated_at,omitempty"`
}

type ConceptListView struct {
	ID               string                     `json:"id"`
	SemanticID       string                     `json:"semantic_id,omitempty"`
	SystemName       string                     `json:"system_name,omitempty"`
	UIName           domain.Translations        `json:"ui_name,omitempty"`
	Description      domain.Translations        `json:"description,omitempty"`
	Status           string                     `json:"status"`
	ProjectID        string                     `json:"project_id"`
	ListType         *string                    `json:"list_type,omitempty"`
	ListTypeURI      string                     `json:"list_type_uri,omitempty"`
	ParentTermURI    string                     `json:"parent_term_uri,omitempty"`
	ListTypeLabel    string                     `json:"list_type_label,omitempty"`
	ParentTerm       *domain.VocabularyEntryRef `json:"parent_term,omitempty"`
	VocabularyID     *string                    `json:"vocabulary_id,omitempty"`
	VocabularyLabel  string                     `json:"vocabulary_label,omitempty"`
	SourceVocabulary *domain.VocabularyRef      `json:"source_vocabulary,omitempty"`
	EntryCount       int                        `json:"entry_count"`
	BoundFieldCount  int                        `json:"bound_field_count"`
	Entries          []ConceptListEntryView     `json:"entries,omitempty"`
	CreatedAt        time.Time                  `json:"created_at,omitempty"`
	UpdatedAt        time.Time                  `json:"updated_at,omitempty"`
}

type ConceptListEntryView struct {
	ID                string              `json:"id"`
	ConceptListID     string              `json:"concept_list_id"`
	VocabularyEntryID string              `json:"vocabulary_entry_id"`
	Position          int                 `json:"position"`
	CustomLabel       domain.Translations `json:"custom_label,omitempty"`
	Entry             VocabularyEntryView `json:"entry"`
}

type ConceptListInput struct {
	SystemName   string
	UIName       domain.Translations
	Description  domain.Translations
	Status       string
	VocabularyID string
	ListTypeURI  string
}

type ErrConceptListValidation struct {
	Fields map[string][]string
}

func (e *ErrConceptListValidation) Error() string { return "concept list validation failed" }

func (e *ErrConceptListValidation) ValidationFields() map[string][]string {
	return e.Fields
}

type ErrConceptListNotFound struct {
	ID string
}

func (e *ErrConceptListNotFound) Error() string { return "concept list not found" }

func (e *ErrConceptListNotFound) IsNotFound() bool { return true }

type ErrConceptListInUse struct {
	Count int
}

func (e *ErrConceptListInUse) Error() string { return e.InUseMessage() }

func (e *ErrConceptListInUse) InUseMessage() string {
	if e.Count == 1 {
		return "Concept list is used by 1 field and cannot be deleted."
	}
	return fmt.Sprintf("Concept list is used by %d fields and cannot be deleted.", e.Count)
}

func (s *Service) ListGlobalVocabularies(ctx context.Context) ([]VocabularyView, error) {
	rows, err := s.queries.WeaveListGlobalVocabularies(ctx)
	if err != nil {
		return nil, fmt.Errorf("list global vocabularies: %w", err)
	}
	return vocabularyViews(rows), nil
}

func (s *Service) ListAdminVocabularies(ctx context.Context) ([]AdminVocabularyView, error) {
	rows, err := s.pool.Query(ctx, `
SELECT
    v.id,
    COALESCE(v.semantic_id, '') AS semantic_id,
    COALESCE(v.system_name, '') AS system_name,
    COALESCE(v.ui_name, '{}'::jsonb) AS ui_name,
    COALESCE(v.description, '{}'::jsonb) AS description,
    v.status,
    COALESCE(v.project_id, '') AS project_id,
    v.connector_type,
    COALESCE(v.base_uri, '') AS base_uri,
    v.created_at,
    v.updated_at,
    COUNT(DISTINCT ve.id) AS entry_count,
    COUNT(DISTINCT pv.project_id) FILTER (WHERE pv.status = 'active') AS project_count
FROM weave_vocabularies v
LEFT JOIN weave_vocabulary_entries ve ON ve.vocabulary_id = v.id
LEFT JOIN weave_project_vocabularies pv ON pv.vocabulary_id = v.id
WHERE v.project_id IS NULL
GROUP BY v.id
ORDER BY v.system_name ASC, v.id ASC
`)
	if err != nil {
		return nil, fmt.Errorf("list admin vocabularies: %w", err)
	}
	defer rows.Close()

	out := []AdminVocabularyView{}
	for rows.Next() {
		var (
			item                     AdminVocabularyView
			uiName, description      []byte
			entryCount, projectCount int64
		)
		if err := rows.Scan(
			&item.ID,
			&item.SemanticID,
			&item.SystemName,
			&uiName,
			&description,
			&item.Status,
			&item.ProjectID,
			&item.ConnectorType,
			&item.BaseURI,
			&item.CreatedAt,
			&item.UpdatedAt,
			&entryCount,
			&projectCount,
		); err != nil {
			return nil, fmt.Errorf("scan admin vocabulary: %w", err)
		}
		item.UIName = unmarshalTranslations(uiName)
		item.Description = unmarshalTranslations(description)
		item.EntryCount = int(entryCount)
		item.ProjectCount = int(projectCount)
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate admin vocabularies: %w", err)
	}
	return out, nil
}

func (s *Service) ListProjectVocabularies(ctx context.Context, projectID string) ([]VocabularyView, error) {
	rows, err := s.queries.WeaveListProjectScopedVocabularies(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("list project vocabularies: %w", err)
	}
	return vocabularyViews(rows), nil
}

func (s *Service) ListProjectConceptLists(ctx context.Context, projectID string) ([]ConceptListView, error) {
	rows, err := s.queries.WeaveListConceptLists(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("list concept lists: %w", err)
	}
	out := make([]ConceptListView, 0, len(rows))
	for _, row := range rows {
		out = append(out, conceptListView(row))
	}
	if err := s.decorateConceptLists(ctx, projectID, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Service) decorateConceptLists(ctx context.Context, projectID string, lists []ConceptListView) error {
	if len(lists) == 0 {
		return nil
	}
	rows, err := s.pool.Query(ctx, `
SELECT
    cl.id,
    COALESCE(v.id, '') AS vocabulary_id,
    COALESCE(v.semantic_id, '') AS vocabulary_semantic_id,
    COALESCE(v.system_name, '') AS vocabulary_system_name,
    COALESCE(v.ui_name, '{}'::jsonb) AS vocabulary_ui_name,
    COALESCE(v.base_uri, '') AS vocabulary_base_uri,
    COALESCE(lte.id, '') AS list_type_id,
    COALESCE(lte.vocabulary_id, '') AS list_type_vocabulary_id,
    COALESCE(lte.uri, '') AS list_type_uri,
    COALESCE(lte.label, '{}'::jsonb) AS list_type_label,
    COALESCE(lte.scope_note, '{}'::jsonb) AS list_type_scope_note,
    COALESCE(lte.broader_uri, '') AS list_type_broader_uri,
    COALESCE(lte.external_id, '') AS list_type_external_id,
    COUNT(DISTINCT cle.id) AS entry_count,
    COUNT(DISTINCT fo.field_id) AS bound_field_count
FROM weave_concept_lists cl
LEFT JOIN weave_vocabularies v ON v.id = cl.vocabulary_id
LEFT JOIN weave_vocabulary_entries lte ON lte.id = cl.list_type
LEFT JOIN weave_concept_list_entries cle ON cle.concept_list_id = cl.id
LEFT JOIN weave_override_refs r
    ON r.ref_type = 'concept_list'
    AND (r.target_id = cl.id OR r.semantic_id = cl.semantic_id)
LEFT JOIN weave_field_overrides fo
    ON fo.id = r.override_id
    AND fo.project_id = cl.project_id
WHERE cl.project_id = $1
GROUP BY cl.id, v.id, v.semantic_id, v.system_name, v.ui_name, v.base_uri, lte.id, lte.vocabulary_id, lte.uri, lte.label, lte.scope_note, lte.broader_uri, lte.external_id
`, projectID)
	if err != nil {
		return fmt.Errorf("load concept list decorations: %w", err)
	}
	defer rows.Close()

	type decoration struct {
		vocabularyID         string
		vocabularySemanticID string
		vocabularySystemName string
		vocabularyUIName     domain.Translations
		vocabularyBaseURI    string
		listTypeID           string
		listTypeVocabularyID string
		listTypeURI          string
		listTypeLabel        domain.Translations
		listTypeScopeNote    domain.Translations
		listTypeBroaderURI   string
		listTypeExternalID   string
		entryCount           int
		boundFieldCount      int
	}
	byID := make(map[string]decoration, len(lists))
	for rows.Next() {
		var (
			id, vocabID, vocabSemanticID, vocabSystemName, vocabBaseURI string
			vocabUIName                                                 []byte
			listTypeID, listTypeVocabularyID, listTypeURI               string
			listTypeLabel, listTypeScopeNote                            []byte
			listTypeBroaderURI, listTypeExternalID                      string
			entryCount                                                  int
			boundFieldCount                                             int
		)
		if err := rows.Scan(
			&id,
			&vocabID,
			&vocabSemanticID,
			&vocabSystemName,
			&vocabUIName,
			&vocabBaseURI,
			&listTypeID,
			&listTypeVocabularyID,
			&listTypeURI,
			&listTypeLabel,
			&listTypeScopeNote,
			&listTypeBroaderURI,
			&listTypeExternalID,
			&entryCount,
			&boundFieldCount,
		); err != nil {
			return fmt.Errorf("scan concept list decorations: %w", err)
		}
		byID[id] = decoration{
			vocabularyID:         vocabID,
			vocabularySemanticID: vocabSemanticID,
			vocabularySystemName: vocabSystemName,
			vocabularyUIName:     unmarshalTranslations(vocabUIName),
			vocabularyBaseURI:    vocabBaseURI,
			listTypeID:           listTypeID,
			listTypeVocabularyID: listTypeVocabularyID,
			listTypeURI:          listTypeURI,
			listTypeLabel:        unmarshalTranslations(listTypeLabel),
			listTypeScopeNote:    unmarshalTranslations(listTypeScopeNote),
			listTypeBroaderURI:   listTypeBroaderURI,
			listTypeExternalID:   listTypeExternalID,
			entryCount:           entryCount,
			boundFieldCount:      boundFieldCount,
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate concept list decorations: %w", err)
	}

	for i := range lists {
		dec, ok := byID[lists[i].ID]
		if !ok {
			continue
		}
		lists[i].EntryCount = dec.entryCount
		lists[i].BoundFieldCount = dec.boundFieldCount
		lists[i].VocabularyLabel = translationLabel(dec.vocabularyUIName, dec.vocabularySystemName)
		lists[i].ListTypeURI = dec.listTypeURI
		lists[i].ParentTermURI = dec.listTypeURI
		lists[i].ListTypeLabel = translationLabel(dec.listTypeLabel, dec.listTypeURI)
		if dec.vocabularyID != "" {
			lists[i].SourceVocabulary = &domain.VocabularyRef{
				ID:         dec.vocabularyID,
				SemanticID: dec.vocabularySemanticID,
				SystemName: dec.vocabularySystemName,
				Name:       dec.vocabularyUIName,
				BaseURI:    dec.vocabularyBaseURI,
			}
		}
		if dec.listTypeURI != "" {
			lists[i].ParentTerm = &domain.VocabularyEntryRef{
				ID:           dec.listTypeID,
				VocabularyID: dec.listTypeVocabularyID,
				URI:          dec.listTypeURI,
				Label:        dec.listTypeLabel,
				ScopeNote:    dec.listTypeScopeNote,
				BroaderURI:   dec.listTypeBroaderURI,
				ExternalID:   dec.listTypeExternalID,
			}
		}
	}
	return nil
}

func (s *Service) GetProjectConceptList(ctx context.Context, projectID, id string) (*ConceptListView, error) {
	row, err := s.queries.WeaveGetProjectConceptList(ctx, sqlcgen.WeaveGetProjectConceptListParams{
		ProjectID: projectID,
		ID:        id,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get concept list: %w", err)
	}
	view := conceptListView(row)
	decorated := []ConceptListView{view}
	if err := s.decorateConceptLists(ctx, projectID, decorated); err != nil {
		return nil, err
	}
	view = decorated[0]
	entries, err := s.queries.WeaveListConceptListEntriesWithVocabulary(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("list concept list entries: %w", err)
	}
	view.Entries = make([]ConceptListEntryView, 0, len(entries))
	for _, entry := range entries {
		view.Entries = append(view.Entries, conceptListEntryViewFromListRow(entry))
	}
	return &view, nil
}

func (s *Service) CreateConceptList(ctx context.Context, projectID string, input ConceptListInput) (*ConceptListView, error) {
	input = normalizeConceptListInput(input)
	if err := validateConceptListInput(input); err != nil {
		return nil, err
	}
	id, err := s.nextConceptListID(ctx, projectID)
	if err != nil {
		return nil, err
	}
	if input.SystemName == "" {
		input.SystemName = toSystemName(firstTranslation(input.UIName, id))
	}
	if input.SystemName == "" {
		input.SystemName = strings.ToLower(strings.ReplaceAll(id, ".", "_"))
	}
	if err := s.validateVocabularyInProject(ctx, projectID, input.VocabularyID); err != nil {
		return nil, err
	}
	listTypeID, err := s.resolveConceptListTypeID(ctx, input.VocabularyID, input.ListTypeURI)
	if err != nil {
		return nil, err
	}

	var vocabularyArg any
	if input.VocabularyID != "" {
		vocabularyArg = input.VocabularyID
	}
	var listTypeArg any
	if listTypeID != "" {
		listTypeArg = listTypeID
	}
	_, err = s.pool.Exec(ctx, `
INSERT INTO weave_concept_lists (
    id, semantic_id, system_name, ui_name, description, status,
    project_id, list_type, vocabulary_id, created_at, updated_at
) VALUES (
    $1, $1, $2, $3::jsonb, $4::jsonb, $5,
    $6, $7, $8, NOW(), NOW()
)`, id, input.SystemName, marshalJSON(input.UIName), marshalJSON(input.Description), input.Status, projectID, listTypeArg, vocabularyArg)
	if err != nil {
		return nil, fmt.Errorf("create concept list: %w", err)
	}
	return s.GetProjectConceptList(ctx, projectID, id)
}

func (s *Service) UpdateConceptList(ctx context.Context, projectID, id string, input ConceptListInput) (*ConceptListView, error) {
	existing, err := s.GetProjectConceptList(ctx, projectID, id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, &ErrConceptListNotFound{ID: id}
	}
	input = normalizeConceptListInput(input)
	if input.SystemName == "" {
		input.SystemName = existing.SystemName
	}
	if len(input.UIName) == 0 {
		input.UIName = existing.UIName
	}
	if input.Description == nil {
		input.Description = existing.Description
	}
	if input.Status == "" {
		input.Status = existing.Status
	}
	if input.VocabularyID == "" && existing.VocabularyID != nil {
		input.VocabularyID = *existing.VocabularyID
	}
	if err := validateConceptListInput(input); err != nil {
		return nil, err
	}
	if err := s.validateVocabularyInProject(ctx, projectID, input.VocabularyID); err != nil {
		return nil, err
	}
	listTypeID, err := s.resolveConceptListTypeID(ctx, input.VocabularyID, input.ListTypeURI)
	if err != nil {
		return nil, err
	}
	var vocabularyArg any
	if input.VocabularyID != "" {
		vocabularyArg = input.VocabularyID
	}
	var listTypeArg any
	if listTypeID != "" {
		listTypeArg = listTypeID
	}

	_, err = s.pool.Exec(ctx, `
UPDATE weave_concept_lists
SET system_name = $3,
    ui_name = $4::jsonb,
    description = $5::jsonb,
    status = $6,
    vocabulary_id = $7,
    list_type = $8,
    updated_at = NOW()
WHERE project_id = $1
  AND (id = $2 OR semantic_id = $2)
`, projectID, id, input.SystemName, marshalJSON(input.UIName), marshalJSON(input.Description), input.Status, vocabularyArg, listTypeArg)
	if err != nil {
		return nil, fmt.Errorf("update concept list: %w", err)
	}
	return s.GetProjectConceptList(ctx, projectID, existing.ID)
}

func (s *Service) DeleteConceptList(ctx context.Context, projectID, id string) error {
	existing, err := s.GetProjectConceptList(ctx, projectID, id)
	if err != nil {
		return err
	}
	if existing == nil {
		return &ErrConceptListNotFound{ID: id}
	}
	var bound int
	if err := s.pool.QueryRow(ctx, `
SELECT COUNT(DISTINCT fo.field_id)
FROM weave_override_refs r
JOIN weave_field_overrides fo ON fo.id = r.override_id
WHERE fo.project_id = $1
  AND r.ref_type = 'concept_list'
  AND (r.target_id = $2 OR r.semantic_id = $3)
`, projectID, existing.ID, existing.SemanticID).Scan(&bound); err != nil {
		return fmt.Errorf("count concept list bindings: %w", err)
	}
	if bound > 0 {
		return &ErrConceptListInUse{Count: bound}
	}
	_, err = s.pool.Exec(ctx, `
DELETE FROM weave_concept_lists
WHERE project_id = $1
  AND (id = $2 OR semantic_id = $2)
`, projectID, id)
	if err != nil {
		return fmt.Errorf("delete concept list: %w", err)
	}
	return nil
}

func (s *Service) AddConceptListEntry(ctx context.Context, projectID, listID, vocabularyEntryID, vocabularyEntryURI string) (*ConceptListEntryView, error) {
	list, err := s.GetProjectConceptList(ctx, projectID, listID)
	if err != nil {
		return nil, err
	}
	if list == nil {
		return nil, &ErrConceptListNotFound{ID: listID}
	}
	vocabularyEntryID = strings.TrimSpace(vocabularyEntryID)
	vocabularyEntryURI = strings.TrimSpace(vocabularyEntryURI)
	if vocabularyEntryID == "" && vocabularyEntryURI == "" {
		return nil, &ErrConceptListValidation{Fields: map[string][]string{"vocabulary_entry_id": {"Choose a vocabulary entry."}}}
	}
	if vocabularyEntryID == "" {
		resolved, err := s.persistSelectedVocabularyEntry(ctx, list, vocabularyEntryURI)
		if err != nil {
			return nil, err
		}
		vocabularyEntryID = resolved.ID
	}
	entry, err := s.queries.WeaveGetVocabularyEntry(ctx, vocabularyEntryID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, &ErrConceptListValidation{Fields: map[string][]string{"vocabulary_entry_id": {"Vocabulary entry not found."}}}
		}
		return nil, fmt.Errorf("get vocabulary entry: %w", err)
	}
	if list.VocabularyID != nil && *list.VocabularyID != "" && entry.VocabularyID != *list.VocabularyID {
		return nil, &ErrConceptListValidation{Fields: map[string][]string{"vocabulary_entry_id": {"Entry is not from this list's source vocabulary."}}}
	}

	var existingID string
	err = s.pool.QueryRow(ctx, `
SELECT id
FROM weave_concept_list_entries
WHERE concept_list_id = $1 AND vocabulary_entry_id = $2
LIMIT 1
`, list.ID, vocabularyEntryID).Scan(&existingID)
	if err == nil && existingID != "" {
		return s.getConceptListEntryView(ctx, list.ID, existingID)
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("check concept list entry duplicate: %w", err)
	}

	var position int
	if err := s.pool.QueryRow(ctx, `
SELECT COALESCE(MAX(position), 0) + 1
FROM weave_concept_list_entries
WHERE concept_list_id = $1
`, list.ID).Scan(&position); err != nil {
		return nil, fmt.Errorf("next concept list entry position: %w", err)
	}
	entryID := ids.GenerateULID()
	if _, err := s.pool.Exec(ctx, `
INSERT INTO weave_concept_list_entries (
    id, concept_list_id, vocabulary_entry_id, position, custom_label, created_at, updated_at
) VALUES (
    $1, $2, $3, $4, '{}'::jsonb, NOW(), NOW()
)`, entryID, list.ID, vocabularyEntryID, position); err != nil {
		return nil, fmt.Errorf("add concept list entry: %w", err)
	}
	return s.getConceptListEntryView(ctx, list.ID, entryID)
}

// CreateTermInput is the payload for authoring a local (hand-typed) concept.
type CreateTermInput struct {
	Label     domain.Translations `json:"label"`
	ScopeNote domain.Translations `json:"scope_note,omitempty"`
}

// CreateLocalTerm mints a hand-authored concept in the project's local
// vocabulary (no remote authority) and links it to the given list. The term
// gets a stable curie URI "pletka:concept/<ULID>". Local terms are allowed in
// any list regardless of the list's source vocabulary.
func (s *Service) CreateLocalTerm(ctx context.Context, projectID, listID string, in CreateTermInput) (*ConceptListEntryView, error) {
	list, err := s.GetProjectConceptList(ctx, projectID, listID)
	if err != nil {
		return nil, err
	}
	if list == nil {
		return nil, &ErrConceptListNotFound{ID: listID}
	}
	if strings.TrimSpace(in.Label.Get("en", "")) == "" {
		return nil, &ErrConceptListValidation{Fields: map[string][]string{"label": {"Enter a term label."}}}
	}

	localVocabID, err := s.ensureLocalVocabulary(ctx, projectID)
	if err != nil {
		return nil, err
	}

	entryID := ids.GenerateULID()
	uri := "pletka:concept/" + entryID
	if _, err := s.queries.WeaveCreateVocabularyEntry(ctx, sqlcgen.WeaveCreateVocabularyEntryParams{
		ID:               entryID,
		VocabularyID:     localVocabID,
		Uri:              uri,
		Label:            marshalJSON(in.Label),
		ScopeNote:        marshalJSON(in.ScopeNote),
		BroaderUri:       nil,
		BroaderPath:      marshalJSONArray([]string{}),
		BroaderPathItems: marshalJSONArray([]domain.VocabularyEntryRef{}),
		ExternalID:       nil,
	}); err != nil {
		return nil, fmt.Errorf("create local term: %w", err)
	}

	var position int
	if err := s.pool.QueryRow(ctx, `
SELECT COALESCE(MAX(position), 0) + 1 FROM weave_concept_list_entries WHERE concept_list_id = $1
`, list.ID).Scan(&position); err != nil {
		return nil, fmt.Errorf("next concept list entry position: %w", err)
	}
	junctionID := ids.GenerateULID()
	if _, err := s.pool.Exec(ctx, `
INSERT INTO weave_concept_list_entries (
    id, concept_list_id, vocabulary_entry_id, position, custom_label, created_at, updated_at
) VALUES ($1, $2, $3, $4, '{}'::jsonb, NOW(), NOW())
`, junctionID, list.ID, entryID, position); err != nil {
		return nil, fmt.Errorf("link local term to list: %w", err)
	}
	return s.getConceptListEntryView(ctx, list.ID, junctionID)
}

// ensureLocalVocabulary returns the id of the project's local vocabulary,
// creating it (connector_type 'local') if it does not exist. Idempotent.
func (s *Service) ensureLocalVocabulary(ctx context.Context, projectID string) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx, `
SELECT id FROM weave_vocabularies
WHERE project_id = $1 AND connector_type = 'local'
ORDER BY created_at LIMIT 1
`, projectID).Scan(&id)
	if err == nil && id != "" {
		return id, nil
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("lookup local vocabulary: %w", err)
	}
	newID := ids.GenerateULID()
	sysName := "local_terms"
	semID := projectID + ".VOCAB.local"
	baseURI := "pletka:concept/"
	if _, err := s.queries.WeaveCreateVocabulary(ctx, sqlcgen.WeaveCreateVocabularyParams{
		ID:            newID,
		SemanticID:    &semID,
		SystemName:    &sysName,
		UiName:        marshalJSON(domain.Translations{"en": "Local terms"}),
		Description:   marshalJSON(domain.Translations{}),
		Status:        "published",
		ProjectID:     &projectID,
		ConnectorType: "local",
		BaseUri:       &baseURI,
		Config:        nil,
	}); err != nil {
		return "", fmt.Errorf("create local vocabulary: %w", err)
	}
	return newID, nil
}

// scopeTerm enforces tenant isolation for hierarchy operations: the list must
// belong to projectID and conceptID must be an entry of it. Returns the
// resolved list (its real id) or a not-found error — the same response whether
// the list/term is missing or belongs to another project, so cross-tenant
// probing reveals nothing.
func (s *Service) scopeTerm(ctx context.Context, projectID, listID, conceptID string) (*ConceptListView, error) {
	list, err := s.GetProjectConceptList(ctx, projectID, listID)
	if err != nil {
		return nil, err
	}
	if list == nil {
		return nil, &ErrConceptListNotFound{ID: listID}
	}
	present, err := s.queries.WeaveConceptListEntryExists(ctx, sqlcgen.WeaveConceptListEntryExistsParams{
		ConceptListID:     list.ID,
		VocabularyEntryID: conceptID,
	})
	if err != nil {
		return nil, fmt.Errorf("scope term: %w", err)
	}
	if !present {
		return nil, &ErrConceptListNotFound{ID: conceptID}
	}
	return list, nil
}

// AddBroader records an editable skos:broader edge, scoped to the list. The
// narrower concept must be a term in (projectID, listID); the broader concept
// must exist (it may be a global/remote term). The edge is always scheme-scoped
// to the list — cross-scheme global edges are a privileged operation not
// exposed here. Idempotent: a duplicate edge is a no-op.
func (s *Service) AddBroader(ctx context.Context, projectID, listID string, edge domain.ConceptBroaderEdge) (domain.ConceptBroaderEdge, error) {
	if strings.TrimSpace(edge.ConceptID) == "" || strings.TrimSpace(edge.BroaderID) == "" {
		return edge, &ErrConceptListValidation{Fields: map[string][]string{"broader": {"Concept and broader are required."}}}
	}
	if edge.ConceptID == edge.BroaderID {
		return edge, &ErrConceptListValidation{Fields: map[string][]string{"broader": {"A concept cannot be broader than itself."}}}
	}
	list, err := s.scopeTerm(ctx, projectID, listID, edge.ConceptID)
	if err != nil {
		return edge, err
	}
	broaderExists, err := s.queries.WeaveVocabularyEntryExists(ctx, edge.BroaderID)
	if err != nil {
		return edge, fmt.Errorf("check broader concept: %w", err)
	}
	if !broaderExists {
		return edge, &ErrConceptListValidation{Fields: map[string][]string{"broader": {"Broader concept not found."}}}
	}
	scheme := list.ID
	edge.SchemeID = &scheme
	if edge.ID == "" {
		edge.ID = ids.GenerateULID()
	}
	if err := s.queries.WeaveAddConceptBroader(ctx, sqlcgen.WeaveAddConceptBroaderParams{
		ID:        edge.ID,
		ConceptID: edge.ConceptID,
		BroaderID: edge.BroaderID,
		SchemeID:  edge.SchemeID,
		Position:  int32(edge.Position),
	}); err != nil {
		return edge, fmt.Errorf("add broader edge: %w", err)
	}
	return edge, nil
}

// RemoveBroader deletes a broader edge, scoped to (projectID, listID, conceptID).
// Returns not-found when nothing matched so an edge id alone cannot delete
// across tenants, and probing does not confirm edge ids.
func (s *Service) RemoveBroader(ctx context.Context, projectID, listID, conceptID, edgeID string) error {
	list, err := s.scopeTerm(ctx, projectID, listID, conceptID)
	if err != nil {
		return err
	}
	scheme := list.ID
	n, err := s.queries.WeaveRemoveConceptBroaderScoped(ctx, sqlcgen.WeaveRemoveConceptBroaderScopedParams{
		ID:        edgeID,
		ConceptID: conceptID,
		SchemeID:  &scheme,
	})
	if err != nil {
		return fmt.Errorf("remove broader edge: %w", err)
	}
	if n == 0 {
		return &ErrConceptListNotFound{ID: edgeID}
	}
	return nil
}

// ListBroader returns a term's broader concepts within (projectID, listID).
func (s *Service) ListBroader(ctx context.Context, projectID, listID, conceptID string) ([]domain.ConceptBroaderEdge, error) {
	list, err := s.scopeTerm(ctx, projectID, listID, conceptID)
	if err != nil {
		return nil, err
	}
	scheme := list.ID
	rows, err := s.queries.WeaveListConceptBroaderInScheme(ctx, sqlcgen.WeaveListConceptBroaderInSchemeParams{
		ConceptID: conceptID,
		SchemeID:  &scheme,
	})
	if err != nil {
		return nil, fmt.Errorf("list broader edges: %w", err)
	}
	out := make([]domain.ConceptBroaderEdge, 0, len(rows))
	for _, r := range rows {
		out = append(out, domain.ConceptBroaderEdge{ID: r.ID, ConceptID: r.ConceptID, BroaderID: r.BroaderID, SchemeID: r.SchemeID, Position: int(r.Position)})
	}
	return out, nil
}

// ListNarrower returns the edges where conceptID is the broader concept
// (i.e. its narrower concepts).
func (s *Service) ListNarrower(ctx context.Context, conceptID string) ([]domain.ConceptBroaderEdge, error) {
	rows, err := s.queries.WeaveListConceptNarrower(ctx, conceptID)
	if err != nil {
		return nil, fmt.Errorf("list narrower edges: %w", err)
	}
	out := make([]domain.ConceptBroaderEdge, 0, len(rows))
	for _, r := range rows {
		out = append(out, domain.ConceptBroaderEdge{ID: r.ID, ConceptID: r.ConceptID, BroaderID: r.BroaderID, SchemeID: r.SchemeID, Position: int(r.Position)})
	}
	return out, nil
}

func (s *Service) persistSelectedVocabularyEntry(ctx context.Context, list *ConceptListView, uri string) (*VocabularyEntryView, error) {
	if strings.TrimSpace(uri) == "" {
		return nil, &ErrConceptListValidation{Fields: map[string][]string{"vocabulary_entry_uri": {"Choose a vocabulary entry."}}}
	}
	vocabularyID := ""
	if list.VocabularyID != nil {
		vocabularyID = strings.TrimSpace(*list.VocabularyID)
	}
	if vocabularyID == "" {
		resolved, err := s.ResolveEntry(ctx, uri, "en")
		if err != nil {
			return nil, err
		}
		if resolved == nil || resolved.ID == "" {
			return nil, &ErrConceptListValidation{Fields: map[string][]string{"vocabulary_entry_uri": {"Vocabulary entry not found."}}}
		}
		return resolved, nil
	}
	vocab, err := s.queries.WeaveGetVocabulary(ctx, vocabularyID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, &ErrConceptListValidation{Fields: map[string][]string{"vocabulary_id": {"Choose a valid source vocabulary."}}}
		}
		return nil, fmt.Errorf("get vocabulary for selected entry: %w", err)
	}
	uri = normalizeVocabularyTermURI(uri, stringPtrValue(vocab.BaseUri))
	row, err := s.queries.WeaveGetVocabularyEntryByVocabularyURI(ctx, sqlcgen.WeaveGetVocabularyEntryByVocabularyURIParams{
		VocabularyID: vocabularyID,
		Uri:          uri,
	})
	if err == nil {
		if hasVocabularyEntryContext(row) {
			view := vocabularyEntryView(row)
			return &view, nil
		}
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("get selected vocabulary entry: %w", err)
	}
	fetched, err := s.registry.ForVocabulary(vocabularyFromRow(vocab)).Fetch(ctx, uri, vocabconnector.SearchOpts{Lang: "en", Limit: 1})
	if err != nil {
		return nil, err
	}
	if fetched == nil {
		return nil, &ErrConceptListValidation{Fields: map[string][]string{"vocabulary_entry_uri": {"Vocabulary entry not found in the selected vocabulary."}}}
	}
	return s.persistConnectorEntry(ctx, vocabularyID, *fetched)
}

func hasVocabularyEntryContext(row sqlcgen.WeaveVocabularyEntry) bool {
	return len(row.BroaderPath) > 0 && string(row.BroaderPath) != "[]" ||
		len(row.BroaderPathItems) > 0 && string(row.BroaderPathItems) != "[]"
}

func (s *Service) RemoveConceptListEntry(ctx context.Context, projectID, listID, entryID string) error {
	list, err := s.GetProjectConceptList(ctx, projectID, listID)
	if err != nil {
		return err
	}
	if list == nil {
		return &ErrConceptListNotFound{ID: listID}
	}
	tag, err := s.pool.Exec(ctx, `
DELETE FROM weave_concept_list_entries
WHERE id = $1 AND concept_list_id = $2
`, entryID, list.ID)
	if err != nil {
		return fmt.Errorf("remove concept list entry: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return &ErrConceptListNotFound{ID: entryID}
	}
	return nil
}

func (s *Service) UpdateConceptListEntry(ctx context.Context, projectID, listID, entryID string, customLabel domain.Translations) (*ConceptListEntryView, error) {
	list, err := s.GetProjectConceptList(ctx, projectID, listID)
	if err != nil {
		return nil, err
	}
	if list == nil {
		return nil, &ErrConceptListNotFound{ID: listID}
	}
	if customLabel == nil {
		customLabel = domain.Translations{}
	}
	tag, err := s.pool.Exec(ctx, `
UPDATE weave_concept_list_entries
SET custom_label = $1::jsonb,
    updated_at = NOW()
WHERE id = $2
  AND concept_list_id = $3
`, marshalJSON(customLabel), entryID, list.ID)
	if err != nil {
		return nil, fmt.Errorf("update concept list entry: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil, &ErrConceptListNotFound{ID: entryID}
	}
	return s.getConceptListEntryView(ctx, list.ID, entryID)
}

func (s *Service) ReorderConceptListEntries(ctx context.Context, projectID, listID string, entryIDs []string) ([]ConceptListEntryView, error) {
	list, err := s.GetProjectConceptList(ctx, projectID, listID)
	if err != nil {
		return nil, err
	}
	if list == nil {
		return nil, &ErrConceptListNotFound{ID: listID}
	}
	rows, err := s.queries.WeaveListConceptListEntries(ctx, list.ID)
	if err != nil {
		return nil, fmt.Errorf("list concept list entries for reorder: %w", err)
	}
	if len(entryIDs) != len(rows) {
		return nil, &ErrConceptListValidation{Fields: map[string][]string{"entry_ids": {"Entry order must include every entry in the list exactly once."}}}
	}
	owned := make(map[string]bool, len(rows))
	for _, row := range rows {
		owned[row.ID] = true
	}
	seen := make(map[string]bool, len(entryIDs))
	for _, entryID := range entryIDs {
		entryID = strings.TrimSpace(entryID)
		if entryID == "" || !owned[entryID] || seen[entryID] {
			return nil, &ErrConceptListValidation{Fields: map[string][]string{"entry_ids": {"Entry order must include every entry in the list exactly once."}}}
		}
		seen[entryID] = true
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin concept list reorder: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	for i, entryID := range entryIDs {
		if _, err := tx.Exec(ctx, `
UPDATE weave_concept_list_entries
SET position = $1,
    updated_at = NOW()
WHERE id = $2
  AND concept_list_id = $3
`, i+1, entryID, list.ID); err != nil {
			return nil, fmt.Errorf("update concept list entry order: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit concept list reorder: %w", err)
	}

	updated, err := s.queries.WeaveListConceptListEntriesWithVocabulary(ctx, list.ID)
	if err != nil {
		return nil, fmt.Errorf("list reordered concept list entries: %w", err)
	}
	out := make([]ConceptListEntryView, 0, len(updated))
	for _, row := range updated {
		out = append(out, conceptListEntryViewFromListRow(row))
	}
	return out, nil
}

func (s *Service) SearchVocabularyEntries(ctx context.Context, vocabularyID, query, lang string, limit int) ([]VocabularyEntryView, error) {
	return s.searchVocabularyEntries(ctx, vocabularyID, query, lang, limit, "")
}

func (s *Service) SearchVocabularyEntriesWithParent(ctx context.Context, vocabularyID, query, lang string, limit int, parentURI string) ([]VocabularyEntryView, error) {
	return s.searchVocabularyEntries(ctx, vocabularyID, query, lang, limit, parentURI)
}

func (s *Service) searchVocabularyEntries(ctx context.Context, vocabularyID, query, lang string, limit int, parentURI string) ([]VocabularyEntryView, error) {
	if limit <= 0 || limit > 100 {
		limit = defaultSearchLimit
	}
	vocab, err := s.queries.WeaveGetVocabulary(ctx, vocabularyID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get vocabulary: %w", err)
	}
	rows, err := s.queries.WeaveSearchVocabularyEntries(ctx, sqlcgen.WeaveSearchVocabularyEntriesParams{
		VocabularyID: vocabularyID,
		Query:        query,
	})
	if err != nil {
		return nil, fmt.Errorf("search vocabulary entries: %w", err)
	}
	merged := entryRowsToMap(rows)
	if strings.TrimSpace(parentURI) != "" {
		merged = map[string]VocabularyEntryView{}
	}
	if len(merged) < limit || strings.TrimSpace(parentURI) != "" {
		connectorRows, err := s.registry.ForVocabulary(vocabularyFromRow(vocab)).Search(ctx, query, vocabconnector.SearchOpts{
			Lang:      lang,
			Limit:     limit,
			ParentURI: parentURI,
		})
		if err != nil && !errors.Is(err, vocabconnector.ErrNotImplemented) {
			return nil, err
		}
		for _, connectorRow := range connectorRows {
			view := connectorEntryView(vocabularyID, connectorRow)
			merged[view.URI] = view
		}
	}
	return limitedEntryViews(merged, limit), nil
}

func (s *Service) SearchConceptListEntries(ctx context.Context, conceptListID, query, lang string, limit int) ([]ConceptListEntryView, error) {
	if limit <= 0 || limit > 100 {
		limit = defaultSearchLimit
	}
	_, err := s.queries.WeaveGetConceptListByIDOrSemanticID(ctx, conceptListID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get concept list: %w", err)
	}
	rows, err := s.queries.WeaveSearchConceptListEntries(ctx, sqlcgen.WeaveSearchConceptListEntriesParams{
		ConceptListID: conceptListID,
		Query:         query,
		ResultLimit:   int32(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("search concept list entries: %w", err)
	}
	out := make([]ConceptListEntryView, 0, len(rows))
	for _, row := range rows {
		out = append(out, conceptListEntryViewFromSearchRow(row))
	}
	return out, nil
}

func (s *Service) SearchConceptListSourceEntries(ctx context.Context, projectID, conceptListID, query, lang string, limit int) ([]ConceptListEntryView, error) {
	if limit <= 0 || limit > 100 {
		limit = defaultSearchLimit
	}
	list, err := s.queries.WeaveGetProjectConceptList(ctx, sqlcgen.WeaveGetProjectConceptListParams{
		ProjectID: projectID,
		ID:        conceptListID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, &ErrConceptListNotFound{ID: conceptListID}
		}
		return nil, fmt.Errorf("get concept list: %w", err)
	}
	if list.VocabularyID == nil || *list.VocabularyID == "" {
		return s.appendProjectVocabularyHits(ctx, nil, list.ProjectID, query, lang, limit)
	}
	parentURI, err := s.conceptListTypeURI(ctx, list.ListType)
	if err != nil {
		return nil, err
	}
	vocabHits, err := s.SearchVocabularyEntriesWithParent(ctx, *list.VocabularyID, query, lang, limit, parentURI)
	if err != nil {
		return nil, err
	}
	return appendVocabularyHits(nil, vocabHits, limit), nil
}

func (s *Service) appendProjectVocabularyHits(ctx context.Context, out []ConceptListEntryView, projectID, query, lang string, limit int) ([]ConceptListEntryView, error) {
	vocabs, err := s.queries.WeaveListProjectScopedVocabularies(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("list project vocabularies: %w", err)
	}
	for _, vocab := range vocabs {
		if len(out) >= limit {
			break
		}
		vocabHits, err := s.SearchVocabularyEntries(ctx, vocab.ID, query, lang, limit-len(out))
		if err != nil {
			return nil, err
		}
		out = appendVocabularyHits(out, vocabHits, limit)
	}
	return out, nil
}

func appendVocabularyHits(out []ConceptListEntryView, vocabHits []VocabularyEntryView, limit int) []ConceptListEntryView {
	seen := map[string]struct{}{}
	for _, row := range out {
		seen[row.Entry.URI] = struct{}{}
	}
	for _, hit := range vocabHits {
		if _, ok := seen[hit.URI]; ok {
			continue
		}
		out = append(out, ConceptListEntryView{VocabularyEntryID: hit.ID, Entry: hit})
		if len(out) >= limit {
			break
		}
	}
	return out
}

func (s *Service) nextConceptListID(ctx context.Context, projectID string) (string, error) {
	var maxNumber int64
	if err := s.pool.QueryRow(ctx, `
SELECT COALESCE(MAX((regexp_match(id, '^' || $1 || '\.CL\.([0-9]+)$'))[1]::bigint), 0)
FROM weave_concept_lists
WHERE project_id = $1
  AND id ~ ('^' || $1 || '\.CL\.[0-9]+$')
`, projectID).Scan(&maxNumber); err != nil {
		return "", fmt.Errorf("read max concept list id: %w", err)
	}
	if s.numberer != nil {
		_, err := s.pool.Exec(ctx, `
INSERT INTO weave_entity_counters (project_id, kind, next_n)
VALUES ($1, 'concept_list', $2)
ON CONFLICT (project_id, kind)
DO UPDATE SET
    next_n = GREATEST(weave_entity_counters.next_n, EXCLUDED.next_n),
    updated_at = NOW()
`, projectID, maxNumber+1)
		if err != nil {
			return "", fmt.Errorf("seed concept list counter: %w", err)
		}
		next, err := s.numberer.AllocateEntityNumber(ctx, projectID, "concept_list")
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s.CL.%d", projectID, next), nil
	}
	return fmt.Sprintf("%s.CL.%d", projectID, maxNumber+1), nil
}

func (s *Service) validateVocabularyInProject(ctx context.Context, projectID, vocabularyID string) error {
	if strings.TrimSpace(vocabularyID) == "" {
		return nil
	}
	rows, err := s.queries.WeaveListProjectScopedVocabularies(ctx, projectID)
	if err != nil {
		return fmt.Errorf("list project vocabularies: %w", err)
	}
	for _, row := range rows {
		if row.ID == vocabularyID {
			return nil
		}
	}
	return &ErrConceptListValidation{Fields: map[string][]string{"vocabulary_id": {"Choose a vocabulary exposed to this project."}}}
}

func (s *Service) resolveConceptListTypeID(ctx context.Context, vocabularyID, rawURI string) (string, error) {
	rawURI = strings.TrimSpace(rawURI)
	if rawURI == "" {
		return "", nil
	}
	if vocabularyID == "" {
		return "", &ErrConceptListValidation{Fields: map[string][]string{"parent_term_uri": {"Choose a source vocabulary before setting a parent term."}}}
	}
	vocab, err := s.queries.WeaveGetVocabulary(ctx, vocabularyID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", &ErrConceptListValidation{Fields: map[string][]string{"vocabulary_id": {"Choose a valid source vocabulary."}}}
		}
		return "", fmt.Errorf("get vocabulary for parent term: %w", err)
	}
	uri := normalizeVocabularyTermURI(rawURI, stringPtrValue(vocab.BaseUri))
	row, err := s.queries.WeaveGetVocabularyEntryByVocabularyURI(ctx, sqlcgen.WeaveGetVocabularyEntryByVocabularyURIParams{
		VocabularyID: vocabularyID,
		Uri:          uri,
	})
	if err == nil {
		return row.ID, nil
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("get vocabulary parent term: %w", err)
	}
	fetched, err := s.registry.ForVocabulary(vocabularyFromRow(vocab)).Fetch(ctx, uri, vocabconnector.SearchOpts{Lang: "en", Limit: 1})
	if err != nil {
		return "", err
	}
	if fetched == nil {
		return "", &ErrConceptListValidation{Fields: map[string][]string{"parent_term_uri": {"Parent term was not found in the selected vocabulary."}}}
	}
	stored, err := s.persistConnectorEntry(ctx, vocabularyID, *fetched)
	if err != nil {
		return "", err
	}
	return stored.ID, nil
}

func (s *Service) conceptListTypeURI(ctx context.Context, listType *string) (string, error) {
	if listType == nil || strings.TrimSpace(*listType) == "" {
		return "", nil
	}
	entry, err := s.queries.WeaveGetVocabularyEntry(ctx, *listType)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}
		return "", fmt.Errorf("get concept list parent term: %w", err)
	}
	return entry.Uri, nil
}

var bareVocabularyID = regexp.MustCompile(`^[0-9]+$`)

func normalizeVocabularyTermURI(value, baseURI string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if !bareVocabularyID.MatchString(value) {
		return value
	}
	baseURI = strings.TrimSpace(baseURI)
	if baseURI == "" {
		baseURI = "https://vocab.getty.edu/aat/"
	}
	return strings.TrimRight(baseURI, "/") + "/" + value
}

func (s *Service) getConceptListEntryView(ctx context.Context, conceptListID, entryID string) (*ConceptListEntryView, error) {
	rows, err := s.queries.WeaveListConceptListEntriesWithVocabulary(ctx, conceptListID)
	if err != nil {
		return nil, fmt.Errorf("list concept list entries: %w", err)
	}
	for _, row := range rows {
		if row.ConceptListEntryID == entryID {
			view := conceptListEntryViewFromListRow(row)
			return &view, nil
		}
	}
	return nil, &ErrConceptListNotFound{ID: entryID}
}

func normalizeConceptListInput(input ConceptListInput) ConceptListInput {
	input.SystemName = toSystemName(input.SystemName)
	input.Status = normalizeStatus(input.Status)
	input.VocabularyID = strings.TrimSpace(input.VocabularyID)
	input.ListTypeURI = strings.TrimSpace(input.ListTypeURI)
	if input.UIName == nil {
		input.UIName = domain.Translations{}
	}
	if input.Description == nil {
		input.Description = domain.Translations{}
	}
	return input
}

func validateConceptListInput(input ConceptListInput) error {
	fields := map[string][]string{}
	if !translationsHaveValue(input.UIName) {
		fields["ui_name"] = []string{"Name is required."}
	}
	if input.Status == "" {
		fields["status"] = []string{"Status is required."}
	}
	if len(fields) > 0 {
		return &ErrConceptListValidation{Fields: fields}
	}
	return nil
}

func normalizeStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "published":
		return "published"
	case "deprecated":
		return "deprecated"
	default:
		return "draft"
	}
}

func translationsHaveValue(t domain.Translations) bool {
	for _, value := range t {
		if strings.TrimSpace(value) != "" {
			return true
		}
	}
	return false
}

func firstTranslation(t domain.Translations, fallback string) string {
	if value := strings.TrimSpace(t["en"]); value != "" {
		return value
	}
	keys := make([]string, 0, len(t))
	for key := range t {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if value := strings.TrimSpace(t[key]); value != "" {
			return value
		}
	}
	return fallback
}

var nonSystemNameChars = regexp.MustCompile(`[^a-z0-9_]+`)

func toSystemName(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "-", "_")
	value = strings.ReplaceAll(value, " ", "_")
	value = nonSystemNameChars.ReplaceAllString(value, "_")
	value = strings.Trim(value, "_")
	if value == "" {
		return ""
	}
	if value[0] < 'a' || value[0] > 'z' {
		value = "list_" + value
	}
	for strings.Contains(value, "__") {
		value = strings.ReplaceAll(value, "__", "_")
	}
	return value
}

func (s *Service) ResolveEntry(ctx context.Context, uri, lang string) (*VocabularyEntryView, error) {
	uri = strings.TrimSpace(uri)
	if uri == "" {
		return nil, nil
	}
	row, err := s.queries.WeaveGetVocabularyEntryByURI(ctx, uri)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("get vocabulary entry by uri: %w", err)
	}
	if err == nil {
		view := vocabularyEntryView(row)
		if !isPlaceholderLabel(view) {
			return &view, nil
		}
		vocab, err := s.queries.WeaveGetVocabulary(ctx, row.VocabularyID)
		if err != nil {
			return nil, fmt.Errorf("get vocabulary for entry: %w", err)
		}
		fetched, err := s.registry.ForVocabulary(vocabularyFromRow(vocab)).Fetch(ctx, uri, vocabconnector.SearchOpts{Lang: lang, Limit: 1})
		if err != nil || fetched == nil {
			return &view, err
		}
		stored, err := s.persistConnectorEntry(ctx, row.VocabularyID, *fetched)
		if err != nil {
			return nil, err
		}
		stored.BroaderPath = fetched.BroaderPath
		stored.BroaderPathItems = fetched.BroaderPathItems
		return stored, nil
	}
	vocab, err := s.queries.WeaveFindVocabularyForURI(ctx, uri)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("find vocabulary for uri: %w", err)
	}
	fetched, err := s.registry.ForVocabulary(vocabularyFromRow(vocab)).Fetch(ctx, uri, vocabconnector.SearchOpts{Lang: lang, Limit: 1})
	if err != nil || fetched == nil {
		return nil, err
	}
	stored, err := s.persistConnectorEntry(ctx, vocab.ID, *fetched)
	if err != nil {
		return nil, err
	}
	stored.BroaderPath = fetched.BroaderPath
	stored.BroaderPathItems = fetched.BroaderPathItems
	return stored, nil
}

func (s *Service) persistConnectorEntry(ctx context.Context, vocabularyID string, entry vocabconnector.Entry) (*VocabularyEntryView, error) {
	entry.URI = strings.TrimSpace(entry.URI)
	if entry.URI == "" {
		return nil, fmt.Errorf("connector returned empty URI")
	}
	if entry.ExternalID == "" {
		entry.ExternalID = path.Base(entry.URI)
	}
	row, err := s.queries.WeaveUpsertVocabularyEntry(ctx, sqlcgen.WeaveUpsertVocabularyEntryParams{
		ID:               ids.GenerateULID(),
		VocabularyID:     vocabularyID,
		Uri:              entry.URI,
		Label:            marshalJSON(entry.Label),
		ScopeNote:        marshalJSON(entry.ScopeNote),
		BroaderUri:       entry.BroaderURI,
		BroaderPath:      marshalJSONArray(entry.BroaderPath),
		BroaderPathItems: marshalJSONArray(entry.BroaderPathItems),
		ExternalID:       entry.ExternalID,
	})
	if err != nil {
		return nil, fmt.Errorf("upsert vocabulary entry: %w", err)
	}
	view := vocabularyEntryView(row)
	return &view, nil
}

func vocabularyViews(rows []sqlcgen.WeaveVocabulary) []VocabularyView {
	out := make([]VocabularyView, 0, len(rows))
	for _, row := range rows {
		out = append(out, vocabularyView(row))
	}
	return out
}

func vocabularyView(row sqlcgen.WeaveVocabulary) VocabularyView {
	return VocabularyView{
		ID:            row.ID,
		SemanticID:    stringPtrValue(row.SemanticID),
		SystemName:    stringPtrValue(row.SystemName),
		UIName:        unmarshalTranslations(row.UiName),
		Description:   unmarshalTranslations(row.Description),
		Status:        row.Status,
		ProjectID:     stringPtrValue(row.ProjectID),
		ConnectorType: row.ConnectorType,
		BaseURI:       stringPtrValue(row.BaseUri),
		CreatedAt:     row.CreatedAt,
		UpdatedAt:     row.UpdatedAt,
	}
}

func vocabularyFromRow(row sqlcgen.WeaveVocabulary) *domain.Vocabulary {
	view := vocabularyView(row)
	return &domain.Vocabulary{
		Entity: domain.Entity{
			ID:          view.ID,
			SemanticID:  view.SemanticID,
			SystemName:  view.SystemName,
			UIName:      view.UIName,
			Description: view.Description,
			Status:      domain.Status(view.Status),
			ProjectID:   view.ProjectID,
			CreatedAt:   view.CreatedAt,
			UpdatedAt:   view.UpdatedAt,
		},
		ConnectorType: view.ConnectorType,
		BaseURI:       view.BaseURI,
		Config:        row.Config,
	}
}

func conceptListView(row sqlcgen.WeaveConceptList) ConceptListView {
	return ConceptListView{
		ID:            row.ID,
		SemanticID:    stringPtrValue(row.SemanticID),
		SystemName:    stringPtrValue(row.SystemName),
		UIName:        unmarshalTranslations(row.UiName),
		Description:   unmarshalTranslations(row.Description),
		Status:        row.Status,
		ProjectID:     row.ProjectID,
		ListType:      row.ListType,
		ListTypeLabel: stringPtrValue(row.ListType),
		VocabularyID:  row.VocabularyID,
		CreatedAt:     row.CreatedAt,
		UpdatedAt:     row.UpdatedAt,
	}
}

func conceptListEntryViewFromListRow(row sqlcgen.WeaveListConceptListEntriesWithVocabularyRow) ConceptListEntryView {
	return ConceptListEntryView{
		ID:                row.ConceptListEntryID,
		ConceptListID:     row.ConceptListID,
		VocabularyEntryID: row.VocabularyEntryID,
		Position:          int(row.Position),
		CustomLabel:       unmarshalTranslations(row.CustomLabel),
		Entry:             vocabularyEntryViewFromParts(row.EntryID, row.VocabularyID, row.Uri, row.Label, row.ScopeNote, row.BroaderUri, row.BroaderPath, row.BroaderPathItems, row.ExternalID, row.EntryCreatedAt, row.EntryUpdatedAt),
	}
}

func conceptListEntryViewFromSearchRow(row sqlcgen.WeaveSearchConceptListEntriesRow) ConceptListEntryView {
	return ConceptListEntryView{
		ID:                row.ConceptListEntryID,
		ConceptListID:     row.ConceptListID,
		VocabularyEntryID: row.VocabularyEntryID,
		Position:          int(row.Position),
		CustomLabel:       unmarshalTranslations(row.CustomLabel),
		Entry:             vocabularyEntryViewFromParts(row.EntryID, row.VocabularyID, row.Uri, row.Label, row.ScopeNote, row.BroaderUri, row.BroaderPath, row.BroaderPathItems, row.ExternalID, row.EntryCreatedAt, row.EntryUpdatedAt),
	}
}

func vocabularyEntryView(row sqlcgen.WeaveVocabularyEntry) VocabularyEntryView {
	return vocabularyEntryViewFromParts(row.ID, row.VocabularyID, row.Uri, row.Label, row.ScopeNote, row.BroaderUri, row.BroaderPath, row.BroaderPathItems, row.ExternalID, row.CreatedAt, row.UpdatedAt)
}

func connectorEntryView(vocabularyID string, entry vocabconnector.Entry) VocabularyEntryView {
	entry.URI = strings.TrimSpace(entry.URI)
	if entry.ExternalID == "" {
		entry.ExternalID = path.Base(entry.URI)
	}
	return VocabularyEntryView{
		VocabularyID:     vocabularyID,
		URI:              entry.URI,
		Label:            entry.Label,
		ScopeNote:        entry.ScopeNote,
		BroaderURI:       entry.BroaderURI,
		BroaderPath:      entry.BroaderPath,
		BroaderPathItems: entry.BroaderPathItems,
		ExternalID:       entry.ExternalID,
		Hydrated:         true,
	}
}

func vocabularyEntryViewFromParts(id, vocabularyID, uri string, label, scopeNote []byte, broaderURI *string, broaderPath, broaderPathItems []byte, externalID *string, createdAt, updatedAt time.Time) VocabularyEntryView {
	view := VocabularyEntryView{
		ID:               id,
		VocabularyID:     vocabularyID,
		URI:              uri,
		Label:            unmarshalTranslations(label),
		ScopeNote:        unmarshalTranslations(scopeNote),
		BroaderURI:       stringPtrValue(broaderURI),
		BroaderPath:      unmarshalStringSlice(broaderPath),
		BroaderPathItems: unmarshalVocabularyEntryRefs(broaderPathItems),
		ExternalID:       stringPtrValue(externalID),
		CreatedAt:        createdAt,
		UpdatedAt:        updatedAt,
	}
	view.Hydrated = !isPlaceholderLabel(view)
	return view
}

func entryRowsToMap(rows []sqlcgen.WeaveVocabularyEntry) map[string]VocabularyEntryView {
	out := make(map[string]VocabularyEntryView, len(rows))
	for _, row := range rows {
		view := vocabularyEntryView(row)
		out[view.URI] = view
	}
	return out
}

func limitedEntryViews(rows map[string]VocabularyEntryView, limit int) []VocabularyEntryView {
	out := make([]VocabularyEntryView, 0, len(rows))
	for _, row := range rows {
		out = append(out, row)
	}
	sort.SliceStable(out, func(i, j int) bool {
		left := out[i].Label["en"]
		right := out[j].Label["en"]
		if left == right {
			return out[i].URI < out[j].URI
		}
		return left < right
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func isPlaceholderLabel(view VocabularyEntryView) bool {
	if view.ExternalID == "" || len(view.Label) == 0 {
		return false
	}
	return view.Label["en"] == view.ExternalID
}

func unmarshalTranslations(raw []byte) domain.Translations {
	if len(raw) == 0 {
		return nil
	}
	var t domain.Translations
	if err := json.Unmarshal(raw, &t); err != nil {
		return nil
	}
	return t
}

func unmarshalStringSlice(raw []byte) []string {
	if len(raw) == 0 {
		return nil
	}
	var values []string
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil
	}
	return values
}

func unmarshalVocabularyEntryRefs(raw []byte) []domain.VocabularyEntryRef {
	if len(raw) == 0 {
		return nil
	}
	var values []domain.VocabularyEntryRef
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil
	}
	return values
}

func marshalJSON(value any) []byte {
	if value == nil {
		return []byte("{}")
	}
	b, err := json.Marshal(value)
	if err != nil || len(b) == 0 {
		return []byte("{}")
	}
	return b
}

func marshalJSONArray(value any) []byte {
	if value == nil {
		return []byte("[]")
	}
	b, err := json.Marshal(value)
	if err != nil || len(b) == 0 || string(b) == "null" {
		return []byte("[]")
	}
	return b
}

func stringPtrValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func translationLabel(value domain.Translations, fallback string) string {
	if value != nil {
		for _, lang := range []string{"en", "de", "nl"} {
			if strings.TrimSpace(value[lang]) != "" {
				return value[lang]
			}
		}
		for _, label := range value {
			if strings.TrimSpace(label) != "" {
				return label
			}
		}
	}
	return fallback
}
