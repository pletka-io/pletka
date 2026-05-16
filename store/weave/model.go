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

type modelStore struct {
	store *Store
}

var _ domainweave.ModelStore = (*modelStore)(nil)

// Models returns the store slice for project-scoped model metadata.
func (s *Store) Models() domainweave.ModelStore {
	return &modelStore{store: s}
}

func (s *modelStore) GetByID(ctx context.Context, projectID, id string) (*domainweave.Model, error) {
	queries, err := s.queryFacade()
	if err != nil {
		return nil, err
	}
	row, err := queries.WeaveGetModelByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get model by id: %w", err)
	}
	if row.ProjectID != projectID {
		return nil, nil
	}
	return modelFromRow(row), nil
}

func (s *modelStore) GetByIDVersion(ctx context.Context, projectID, id, version string) (*domainweave.Model, error) {
	pool, err := s.connectionPool()
	if err != nil {
		return nil, err
	}
	row := pool.QueryRow(ctx, archivedModelSelect()+`
		WHERE id = $1 AND project_id = $2 AND version_number = $3
	`, id, projectID, version)
	model, err := scanArchivedModel(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get archived model by id: %w", err)
	}
	return model, nil
}

func (s *modelStore) List(ctx context.Context, projectID string) ([]*domainweave.Model, error) {
	pool, err := s.connectionPool()
	if err != nil {
		return nil, err
	}
	rows, err := pool.Query(ctx, `
		SELECT
			id, created_at, updated_at, system_name, ui_name, description,
			status, project_id, ontology_scope, staging_id, deprecated, version_number, model_type
		FROM weave_models
		WHERE project_id = $1
		ORDER BY CASE WHEN model_type = 'core' THEN 0 ELSE 1 END, system_name ASC
	`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list models: %w", err)
	}
	defer rows.Close()

	models := []*domainweave.Model{}
	for rows.Next() {
		model, err := scanModel(rows)
		if err != nil {
			return nil, fmt.Errorf("scan model: %w", err)
		}
		models = append(models, model)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate models: %w", err)
	}
	return models, nil
}

func (s *modelStore) ListVersion(ctx context.Context, projectID, version string) ([]*domainweave.Model, error) {
	pool, err := s.connectionPool()
	if err != nil {
		return nil, err
	}
	rows, err := pool.Query(ctx, archivedModelSelect()+`
		WHERE project_id = $1 AND version_number = $2
		ORDER BY CASE WHEN model_type = 'core' THEN 0 ELSE 1 END, system_name ASC
	`, projectID, version)
	if err != nil {
		return nil, fmt.Errorf("list archived models: %w", err)
	}
	defer rows.Close()

	models := []*domainweave.Model{}
	for rows.Next() {
		model, err := scanArchivedModel(rows)
		if err != nil {
			return nil, fmt.Errorf("scan archived model: %w", err)
		}
		models = append(models, model)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate archived models: %w", err)
	}
	return models, nil
}

func (s *modelStore) queryFacade() (*sqlcgen.Queries, error) {
	if s == nil || s.store == nil || s.store.queries == nil {
		return nil, errors.New("model store is not initialized")
	}
	return s.store.queries, nil
}

func (s *modelStore) connectionPool() (pgxPool, error) {
	if s == nil || s.store == nil || s.store.pool == nil {
		return nil, errors.New("model store is not initialized")
	}
	return s.store.pool, nil
}

func modelFromRow(row sqlcgen.WeaveModel) *domainweave.Model {
	return modelFromValues(modelValues{
		ID:            row.ID,
		CreatedAt:     row.CreatedAt,
		UpdatedAt:     row.UpdatedAt,
		SystemName:    row.SystemName,
		UiName:        row.UiName,
		Description:   row.Description,
		Status:        row.Status,
		ProjectID:     row.ProjectID,
		OntologyScope: row.OntologyScope,
		StagingID:     row.StagingID,
		Deprecated:    row.Deprecated,
		VersionNumber: row.VersionNumber,
		ModelType:     row.ModelType,
	})
}

type modelValues struct {
	ID            string
	CreatedAt     time.Time
	UpdatedAt     time.Time
	SystemName    *string
	UiName        []byte
	Description   []byte
	Status        string
	ProjectID     string
	OntologyScope []byte
	StagingID     *int64
	Deprecated    bool
	VersionNumber string
	ModelType     string
}

func scanModel(row interface{ Scan(...any) error }) (*domainweave.Model, error) {
	var values modelValues
	if err := row.Scan(
		&values.ID,
		&values.CreatedAt,
		&values.UpdatedAt,
		&values.SystemName,
		&values.UiName,
		&values.Description,
		&values.Status,
		&values.ProjectID,
		&values.OntologyScope,
		&values.StagingID,
		&values.Deprecated,
		&values.VersionNumber,
		&values.ModelType,
	); err != nil {
		return nil, err
	}
	return modelFromValues(values), nil
}

func scanArchivedModel(row interface{ Scan(...any) error }) (*domainweave.Model, error) {
	return scanModel(row)
}

func modelFromValues(values modelValues) *domainweave.Model {
	model := &domainweave.Model{
		Entity: domain.Entity{
			ID:            values.ID,
			CreatedAt:     values.CreatedAt,
			UpdatedAt:     values.UpdatedAt,
			SemanticID:    values.ID,
			SystemName:    dbutil.NilToEmpty(values.SystemName),
			UIName:        translationsFromJSON(values.UiName),
			Description:   translationsFromJSON(values.Description),
			Status:        domain.Status(values.Status),
			VersionNumber: values.VersionNumber,
			ProjectID:     values.ProjectID,
			Deprecated:    values.Deprecated,
		},
		ModelType: values.ModelType,
		StagingID: values.StagingID,
	}
	if len(values.OntologyScope) > 0 {
		_ = json.Unmarshal(values.OntologyScope, &model.OntologyScope)
	}
	return model
}

func archivedModelSelect() string {
	return `
		SELECT
			id, created_at, updated_at, system_name, ui_name, description,
			status, project_id, ontology_scope, staging_id, deprecated, version_number, model_type
		FROM weave_models_archive
	`
}
