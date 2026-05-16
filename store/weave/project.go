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

type projectStore struct {
	store *Store
}

var _ domainweave.ProjectStore = (*projectStore)(nil)

// Projects returns the store slice for project metadata.
func (s *Store) Projects() domainweave.ProjectStore {
	return &projectStore{store: s}
}

func (s *projectStore) GetByID(ctx context.Context, id string) (*domainweave.Project, error) {
	queries, err := s.queryFacade()
	if err != nil {
		return nil, err
	}
	row, err := queries.WeaveGetProjectByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get project by id: %w", err)
	}
	return projectFromRow(row), nil
}

func (s *projectStore) GetByIDVersion(ctx context.Context, id, version string) (*domainweave.Project, error) {
	pool, err := s.connectionPool()
	if err != nil {
		return nil, err
	}
	row := pool.QueryRow(ctx, `
		SELECT
			id, created_at, updated_at, system_name, ui_name, description, status,
			namespace, parent_project_id, staging_id, owner_id, visibility,
			deprecated, license, readme, topics, base_url, created_by_id, version_number
		FROM weave_projects_archive
		WHERE id = $1 AND version_number = $2
	`, id, version)
	project, err := scanArchivedProject(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get archived project by id: %w", err)
	}
	return project, nil
}

func (s *projectStore) queryFacade() (*sqlcgen.Queries, error) {
	if s == nil || s.store == nil || s.store.queries == nil {
		return nil, errors.New("project store is not initialized")
	}
	return s.store.queries, nil
}

func (s *projectStore) connectionPool() (pgxPool, error) {
	if s == nil || s.store == nil || s.store.pool == nil {
		return nil, errors.New("project store is not initialized")
	}
	return s.store.pool, nil
}

type pgxPool interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func projectFromRow(row sqlcgen.WeaveProject) *domainweave.Project {
	return &domainweave.Project{
		Entity: domain.Entity{
			ID:            row.ID,
			CreatedAt:     row.CreatedAt,
			UpdatedAt:     row.UpdatedAt,
			SemanticID:    row.ID,
			SystemName:    dbutil.NilToEmpty(row.SystemName),
			UIName:        translationsFromJSON(row.UiName),
			Description:   translationsFromJSON(row.Description),
			Status:        domain.Status(row.Status),
			VersionNumber: row.VersionNumber,
			Deprecated:    row.Deprecated,
		},
		Namespace:       dbutil.NilToEmpty(row.Namespace),
		ParentProjectID: row.ParentProjectID,
		StagingID:       row.StagingID,
		OwnerID:         row.OwnerID,
		CreatedByID:     row.CreatedByID,
		Visibility:      row.Visibility,
		License:         row.License,
		README:          translationsFromJSON(row.Readme),
		Topics:          row.Topics,
		BaseURL:         row.BaseUrl,
		IsMasterWeave:   row.IsMasterWeave,
	}
}

func scanArchivedProject(row pgx.Row) (*domainweave.Project, error) {
	var (
		id              string
		createdAt       time.Time
		updatedAt       time.Time
		systemName      *string
		uiName          []byte
		description     []byte
		status          string
		namespace       *string
		parentProjectID *string
		stagingID       *int64
		ownerID         string
		visibility      string
		deprecated      bool
		license         string
		readme          []byte
		topics          []string
		baseURL         string
		createdByID     *string
		versionNumber   string
	)
	if err := row.Scan(
		&id,
		&createdAt,
		&updatedAt,
		&systemName,
		&uiName,
		&description,
		&status,
		&namespace,
		&parentProjectID,
		&stagingID,
		&ownerID,
		&visibility,
		&deprecated,
		&license,
		&readme,
		&topics,
		&baseURL,
		&createdByID,
		&versionNumber,
	); err != nil {
		return nil, err
	}

	return &domainweave.Project{
		Entity: domain.Entity{
			ID:            id,
			CreatedAt:     createdAt,
			UpdatedAt:     updatedAt,
			SemanticID:    id,
			SystemName:    dbutil.NilToEmpty(systemName),
			UIName:        translationsFromJSON(uiName),
			Description:   translationsFromJSON(description),
			Status:        domain.Status(status),
			VersionNumber: versionNumber,
			Deprecated:    deprecated,
		},
		Namespace:       dbutil.NilToEmpty(namespace),
		ParentProjectID: parentProjectID,
		StagingID:       stagingID,
		OwnerID:         ownerID,
		CreatedByID:     createdByID,
		Visibility:      visibility,
		License:         license,
		README:          translationsFromJSON(readme),
		Topics:          topics,
		BaseURL:         baseURL,
	}, nil
}

func translationsFromJSON(data []byte) domain.Translations {
	if len(data) == 0 {
		return nil
	}
	var translations domain.Translations
	if err := json.Unmarshal(data, &translations); err != nil {
		return nil
	}
	return translations
}
