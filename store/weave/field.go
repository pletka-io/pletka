package weave

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/pletka-io/pletka/domain"
	domainweave "github.com/pletka-io/pletka/domain/weave"
	"github.com/pletka-io/pletka/store/dbutil"
	"github.com/pletka-io/pletka/store/sqlcgen"
)

type fieldStore struct {
	store *Store
}

var _ domainweave.FieldStore = (*fieldStore)(nil)

// Fields returns the store slice for project-scoped field metadata.
func (s *Store) Fields() domainweave.FieldStore {
	return &fieldStore{store: s}
}

func (s *fieldStore) GetByID(ctx context.Context, projectID, id string) (*domainweave.Field, error) {
	queries, err := s.queryFacade()
	if err != nil {
		return nil, err
	}
	row, err := queries.WeaveGetFieldByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get field by id: %w", err)
	}
	if row.ProjectID != projectID {
		return nil, nil
	}
	return fieldFromRow(row), nil
}

func (s *fieldStore) GetByIDVersion(ctx context.Context, projectID, id, version string) (*domainweave.Field, error) {
	pool, err := s.connectionPool()
	if err != nil {
		return nil, err
	}
	row := pool.QueryRow(ctx, archivedFieldSelect()+`
		WHERE id = $1 AND project_id = $2 AND version_number = $3
	`, id, projectID, version)
	field, err := scanArchivedField(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get archived field by id: %w", err)
	}
	return field, nil
}

func (s *fieldStore) List(ctx context.Context, projectID string) ([]*domainweave.Field, error) {
	pool, err := s.connectionPool()
	if err != nil {
		return nil, err
	}
	rows, err := pool.Query(ctx, `
		SELECT
			id, created_at, updated_at, semantic_id, system_name, ui_name, description,
			status, project_id, ontology_scope, ontology_path, path_elements,
			expected_value_type, examples, staging_id, deprecated, version_number
		FROM weave_fields
		WHERE project_id = $1
		ORDER BY system_name ASC
	`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list fields: %w", err)
	}
	defer rows.Close()

	fields := []*domainweave.Field{}
	for rows.Next() {
		field, err := scanField(rows)
		if err != nil {
			return nil, fmt.Errorf("scan field: %w", err)
		}
		fields = append(fields, field)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate fields: %w", err)
	}
	return fields, nil
}

func (s *fieldStore) ListVersion(ctx context.Context, projectID, version string) ([]*domainweave.Field, error) {
	pool, err := s.connectionPool()
	if err != nil {
		return nil, err
	}
	rows, err := pool.Query(ctx, archivedFieldSelect()+`
		WHERE project_id = $1 AND version_number = $2
		ORDER BY system_name ASC
	`, projectID, version)
	if err != nil {
		return nil, fmt.Errorf("list archived fields: %w", err)
	}
	defer rows.Close()

	fields := []*domainweave.Field{}
	for rows.Next() {
		field, err := scanArchivedField(rows)
		if err != nil {
			return nil, fmt.Errorf("scan archived field: %w", err)
		}
		fields = append(fields, field)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate archived fields: %w", err)
	}
	return fields, nil
}

func (s *fieldStore) queryFacade() (*sqlcgen.Queries, error) {
	if s == nil || s.store == nil || s.store.queries == nil {
		return nil, errors.New("field store is not initialized")
	}
	return s.store.queries, nil
}

func (s *fieldStore) connectionPool() (pgxPool, error) {
	if s == nil || s.store == nil || s.store.pool == nil {
		return nil, errors.New("field store is not initialized")
	}
	return s.store.pool, nil
}

func fieldFromRow(row sqlcgen.WeaveField) *domainweave.Field {
	return fieldFromValues(fieldValues{
		ID:                row.ID,
		CreatedAt:         row.CreatedAt,
		UpdatedAt:         row.UpdatedAt,
		SemanticID:        row.SemanticID,
		SystemName:        row.SystemName,
		UiName:            row.UiName,
		Description:       row.Description,
		Status:            row.Status,
		ProjectID:         row.ProjectID,
		OntologyScope:     row.OntologyScope,
		PathElements:      row.PathElements,
		ExpectedValueType: row.ExpectedValueType,
		Examples:          row.Examples,
		StagingID:         row.StagingID,
		Deprecated:        row.Deprecated,
		VersionNumber:     row.VersionNumber,
	})
}

type fieldValues struct {
	ID                string
	CreatedAt         time.Time
	UpdatedAt         time.Time
	SemanticID        *string
	SystemName        *string
	UiName            []byte
	Description       []byte
	Status            string
	ProjectID         string
	OntologyScope     []byte
	PathElements      []byte
	ExpectedValueType *string
	Examples          []byte
	StagingID         *int64
	Deprecated        bool
	VersionNumber     string
}

func scanField(row interface{ Scan(...any) error }) (*domainweave.Field, error) {
	var values fieldValues
	var ontologyPath *string
	if err := row.Scan(
		&values.ID,
		&values.CreatedAt,
		&values.UpdatedAt,
		&values.SemanticID,
		&values.SystemName,
		&values.UiName,
		&values.Description,
		&values.Status,
		&values.ProjectID,
		&values.OntologyScope,
		&ontologyPath,
		&values.PathElements,
		&values.ExpectedValueType,
		&values.Examples,
		&values.StagingID,
		&values.Deprecated,
		&values.VersionNumber,
	); err != nil {
		return nil, err
	}
	return fieldFromValues(values), nil
}

func scanArchivedField(row interface{ Scan(...any) error }) (*domainweave.Field, error) {
	return scanField(row)
}

func fieldFromValues(values fieldValues) *domainweave.Field {
	field := &domainweave.Field{
		Entity: domain.Entity{
			ID:            values.ID,
			CreatedAt:     values.CreatedAt,
			UpdatedAt:     values.UpdatedAt,
			SemanticID:    dbutil.NilToEmpty(values.SemanticID),
			SystemName:    dbutil.NilToEmpty(values.SystemName),
			UIName:        translationsFromJSON(values.UiName),
			Description:   translationsFromJSON(values.Description),
			Status:        domain.Status(values.Status),
			VersionNumber: values.VersionNumber,
			ProjectID:     values.ProjectID,
			Deprecated:    values.Deprecated,
		},
		ExpectedValueType: dbutil.NilToEmpty(values.ExpectedValueType),
		StagingID:         values.StagingID,
	}
	if len(values.OntologyScope) > 0 {
		_ = json.Unmarshal(values.OntologyScope, &field.OntologyScope)
	}
	if len(values.PathElements) > 0 {
		_ = json.Unmarshal(values.PathElements, &field.PathElements)
	}
	if len(values.Examples) > 0 {
		_ = json.Unmarshal(values.Examples, &field.Examples)
	}
	return field
}

func archivedFieldSelect() string {
	return `
		SELECT
			id, created_at, updated_at, semantic_id, system_name, ui_name, description,
			status, project_id, ontology_scope, ontology_path, path_elements,
			expected_value_type, examples, staging_id, deprecated, version_number
		FROM weave_fields_archive
	`
}
