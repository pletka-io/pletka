package weave

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	domainweave "github.com/pletka-io/pletka/domain/weave"
	"github.com/pletka-io/pletka/store/dbutil"
	"github.com/pletka-io/pletka/store/sqlcgen"
)

type projectOntologyVersionStore struct {
	store *Store
}

var _ domainweave.ProjectOntologyVersionStore = (*projectOntologyVersionStore)(nil)

// ProjectOntologyVersions returns the store slice for project ontology links.
func (s *Store) ProjectOntologyVersions() domainweave.ProjectOntologyVersionStore {
	return &projectOntologyVersionStore{store: s}
}

func (s *projectOntologyVersionStore) Get(ctx context.Context, projectID, ontologyVersionID string) (*domainweave.ProjectOntologyVersion, error) {
	queries, err := s.queryFacade()
	if err != nil {
		return nil, err
	}
	row, err := queries.WeaveGetProjectOntologyVersion(ctx, sqlcgen.WeaveGetProjectOntologyVersionParams{
		ProjectID:         projectID,
		OntologyVersionID: ontologyVersionID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get project ontology version: %w", err)
	}
	return projectOntologyVersionFromRow(row), nil
}

func (s *projectOntologyVersionStore) GetVersion(ctx context.Context, projectID, ontologyVersionID, version string) (*domainweave.ProjectOntologyVersion, error) {
	pool, err := s.connectionPool()
	if err != nil {
		return nil, err
	}
	row := pool.QueryRow(ctx, projectOntologyVersionSelect("weave_project_ontology_versions_archive")+`
		WHERE project_id = $1 AND ontology_version_id = $2 AND version_number = $3
	`, projectID, ontologyVersionID, version)
	link, err := scanProjectOntologyVersion(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get archived project ontology version: %w", err)
	}
	return link, nil
}

func (s *projectOntologyVersionStore) List(ctx context.Context, projectID string) ([]*domainweave.ProjectOntologyVersion, error) {
	queries, err := s.queryFacade()
	if err != nil {
		return nil, err
	}
	rows, err := queries.WeaveListProjectOntologyVersions(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("list project ontology versions: %w", err)
	}
	links := make([]*domainweave.ProjectOntologyVersion, 0, len(rows))
	for _, row := range rows {
		links = append(links, projectOntologyVersionFromRow(row))
	}
	return links, nil
}

func (s *projectOntologyVersionStore) ListVersion(ctx context.Context, projectID, version string) ([]*domainweave.ProjectOntologyVersion, error) {
	pool, err := s.connectionPool()
	if err != nil {
		return nil, err
	}
	rows, err := pool.Query(ctx, projectOntologyVersionSelect("weave_project_ontology_versions_archive")+`
		WHERE project_id = $1 AND version_number = $2
		ORDER BY is_primary DESC, added_at ASC
	`, projectID, version)
	if err != nil {
		return nil, fmt.Errorf("list archived project ontology versions: %w", err)
	}
	defer rows.Close()

	links := []*domainweave.ProjectOntologyVersion{}
	for rows.Next() {
		link, err := scanProjectOntologyVersion(rows)
		if err != nil {
			return nil, fmt.Errorf("scan archived project ontology version: %w", err)
		}
		links = append(links, link)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate archived project ontology versions: %w", err)
	}
	return links, nil
}

func (s *projectOntologyVersionStore) queryFacade() (*sqlcgen.Queries, error) {
	if s == nil || s.store == nil || s.store.queries == nil {
		return nil, errors.New("project ontology version store is not initialized")
	}
	return s.store.queries, nil
}

func (s *projectOntologyVersionStore) connectionPool() (pgxPool, error) {
	if s == nil || s.store == nil || s.store.pool == nil {
		return nil, errors.New("project ontology version store is not initialized")
	}
	return s.store.pool, nil
}

func projectOntologyVersionFromRow(row sqlcgen.WeaveProjectOntologyVersion) *domainweave.ProjectOntologyVersion {
	return projectOntologyVersionFromValues(projectOntologyVersionValues{
		ProjectID:         row.ProjectID,
		OntologyVersionID: row.OntologyVersionID,
		AddedAt:           row.AddedAt,
		AddedByID:         row.AddedByID,
		IsPrimary:         row.IsPrimary,
		UsageNotes:        row.UsageNotes,
		VersionNumber:     row.VersionNumber,
	})
}

type projectOntologyVersionValues struct {
	ProjectID         string
	OntologyVersionID string
	AddedAt           time.Time
	AddedByID         *string
	IsPrimary         *bool
	UsageNotes        *string
	VersionNumber     string
}

func scanProjectOntologyVersion(row interface{ Scan(...any) error }) (*domainweave.ProjectOntologyVersion, error) {
	var values projectOntologyVersionValues
	if err := row.Scan(
		&values.ProjectID,
		&values.OntologyVersionID,
		&values.AddedAt,
		&values.AddedByID,
		&values.IsPrimary,
		&values.UsageNotes,
		&values.VersionNumber,
	); err != nil {
		return nil, err
	}
	return projectOntologyVersionFromValues(values), nil
}

func projectOntologyVersionFromValues(values projectOntologyVersionValues) *domainweave.ProjectOntologyVersion {
	return &domainweave.ProjectOntologyVersion{
		ProjectID:         values.ProjectID,
		OntologyVersionID: values.OntologyVersionID,
		AddedAt:           values.AddedAt,
		AddedByID:         values.AddedByID,
		IsPrimary:         dbutil.Deref(values.IsPrimary),
		UsageNotes:        values.UsageNotes,
		VersionNumber:     values.VersionNumber,
	}
}

func projectOntologyVersionSelect(table string) string {
	return fmt.Sprintf(`
		SELECT project_id, ontology_version_id, added_at, added_by_id, is_primary, usage_notes, version_number
		FROM %s
	`, table)
}
