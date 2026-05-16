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

type collectionStore struct {
	store *Store
}

var _ domainweave.CollectionStore = (*collectionStore)(nil)

// Collections returns the store slice for project-scoped collection metadata.
func (s *Store) Collections() domainweave.CollectionStore {
	return &collectionStore{store: s}
}

func (s *collectionStore) GetByID(ctx context.Context, projectID, id string) (*domainweave.Collection, error) {
	queries, err := s.queryFacade()
	if err != nil {
		return nil, err
	}
	row, err := queries.WeaveGetCollectionByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get collection by id: %w", err)
	}
	if row.ProjectID != projectID {
		return nil, nil
	}
	return collectionFromRow(row), nil
}

func (s *collectionStore) GetByIDVersion(ctx context.Context, projectID, id, version string) (*domainweave.Collection, error) {
	pool, err := s.connectionPool()
	if err != nil {
		return nil, err
	}
	row := pool.QueryRow(ctx, archivedCollectionSelect()+`
		WHERE id = $1 AND project_id = $2 AND version_number = $3
	`, id, projectID, version)
	collection, err := scanArchivedCollection(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get archived collection by id: %w", err)
	}
	return collection, nil
}

func (s *collectionStore) List(ctx context.Context, projectID string) ([]*domainweave.Collection, error) {
	pool, err := s.connectionPool()
	if err != nil {
		return nil, err
	}
	rows, err := pool.Query(ctx, `
		SELECT
			id, created_at, updated_at, system_name, ui_name, description,
			status, project_id, ontology_scope, collection_number,
			canonical_collection_order, staging_id, deprecated, default_category_id, version_number
		FROM weave_collections
		WHERE project_id = $1
		ORDER BY canonical_collection_order ASC, system_name ASC
	`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list collections: %w", err)
	}
	defer rows.Close()

	collections := []*domainweave.Collection{}
	for rows.Next() {
		collection, err := scanCollection(rows)
		if err != nil {
			return nil, fmt.Errorf("scan collection: %w", err)
		}
		collections = append(collections, collection)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate collections: %w", err)
	}
	return collections, nil
}

func (s *collectionStore) ListVersion(ctx context.Context, projectID, version string) ([]*domainweave.Collection, error) {
	pool, err := s.connectionPool()
	if err != nil {
		return nil, err
	}
	rows, err := pool.Query(ctx, archivedCollectionSelect()+`
		WHERE project_id = $1 AND version_number = $2
		ORDER BY canonical_collection_order ASC, system_name ASC
	`, projectID, version)
	if err != nil {
		return nil, fmt.Errorf("list archived collections: %w", err)
	}
	defer rows.Close()

	collections := []*domainweave.Collection{}
	for rows.Next() {
		collection, err := scanArchivedCollection(rows)
		if err != nil {
			return nil, fmt.Errorf("scan archived collection: %w", err)
		}
		collections = append(collections, collection)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate archived collections: %w", err)
	}
	return collections, nil
}

func (s *collectionStore) queryFacade() (*sqlcgen.Queries, error) {
	if s == nil || s.store == nil || s.store.queries == nil {
		return nil, errors.New("collection store is not initialized")
	}
	return s.store.queries, nil
}

func (s *collectionStore) connectionPool() (pgxPool, error) {
	if s == nil || s.store == nil || s.store.pool == nil {
		return nil, errors.New("collection store is not initialized")
	}
	return s.store.pool, nil
}

func collectionFromRow(row sqlcgen.WeaveCollection) *domainweave.Collection {
	return collectionFromValues(collectionValues{
		ID:                       row.ID,
		CreatedAt:                row.CreatedAt,
		UpdatedAt:                row.UpdatedAt,
		SystemName:               row.SystemName,
		UiName:                   row.UiName,
		Description:              row.Description,
		Status:                   row.Status,
		ProjectID:                row.ProjectID,
		OntologyScope:            row.OntologyScope,
		CollectionNumber:         row.CollectionNumber,
		CanonicalCollectionOrder: row.CanonicalCollectionOrder,
		StagingID:                row.StagingID,
		Deprecated:               row.Deprecated,
		DefaultCategoryID:        row.DefaultCategoryID,
		VersionNumber:            row.VersionNumber,
	})
}

type collectionValues struct {
	ID                       string
	CreatedAt                time.Time
	UpdatedAt                time.Time
	SystemName               *string
	UiName                   []byte
	Description              []byte
	Status                   string
	ProjectID                string
	OntologyScope            []byte
	CollectionNumber         *int32
	CanonicalCollectionOrder *int32
	StagingID                *int64
	Deprecated               bool
	DefaultCategoryID        *string
	VersionNumber            string
}

func scanCollection(row interface{ Scan(...any) error }) (*domainweave.Collection, error) {
	var values collectionValues
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
		&values.CollectionNumber,
		&values.CanonicalCollectionOrder,
		&values.StagingID,
		&values.Deprecated,
		&values.DefaultCategoryID,
		&values.VersionNumber,
	); err != nil {
		return nil, err
	}
	return collectionFromValues(values), nil
}

func scanArchivedCollection(row interface{ Scan(...any) error }) (*domainweave.Collection, error) {
	return scanCollection(row)
}

func collectionFromValues(values collectionValues) *domainweave.Collection {
	collection := &domainweave.Collection{
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
		CollectionNumber:         intFromInt32Ptr(values.CollectionNumber),
		CanonicalCollectionOrder: intFromInt32Ptr(values.CanonicalCollectionOrder),
		DefaultCategoryID:        values.DefaultCategoryID,
		StagingID:                values.StagingID,
	}
	if len(values.OntologyScope) > 0 {
		_ = json.Unmarshal(values.OntologyScope, &collection.OntologyScope)
	}
	return collection
}

func intFromInt32Ptr(value *int32) int {
	if value == nil {
		return 0
	}
	return int(*value)
}

func archivedCollectionSelect() string {
	return `
		SELECT
			id, created_at, updated_at, system_name, ui_name, description,
			status, project_id, ontology_scope, collection_number,
			canonical_collection_order, staging_id, deprecated, default_category_id, version_number
		FROM weave_collections_archive
	`
}
