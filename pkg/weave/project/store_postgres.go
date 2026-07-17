package project

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	weaveauth "github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/database/dbutil"
	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/domain"
)

// postgresStore is the pgx + sqlc implementation of Store, backed by
// the weave_projects table (id IS the IDPrefix).
type postgresStore struct {
	queries *sqlcgen.Queries
	pool    *pgxpool.Pool
}

var _ Store = (*postgresStore)(nil)

// NewPostgresStore returns a Store backed by the supplied pgx pool.
func NewPostgresStore(pool *pgxpool.Pool) Store {
	return &postgresStore{
		queries: sqlcgen.New(pool),
		pool:    pool,
	}
}

// ---------------------------------------------------------------------------
// CRUD
// ---------------------------------------------------------------------------

func (s *postgresStore) Create(ctx context.Context, p *domain.Project) error {
	if p.Status == "" {
		p.Status = domain.StatusDraft
	}
	row, err := s.queries.WeaveCreateProject(ctx, sqlcgen.WeaveCreateProjectParams{
		ID:              p.ID,
		SystemName:      dbutil.EmptyToNil(p.SystemName),
		UiName:          marshalTranslations(p.UIName),
		Description:     marshalTranslations(p.Description),
		Status:          string(p.Status),
		Namespace:       dbutil.EmptyToNil(p.Namespace),
		ParentProjectID: p.ParentProjectID,
		StagingID:       p.StagingID,
		OwnerID:         p.OwnerID,
		Visibility:      p.Visibility,
		CreatedByID:     p.CreatedByID,
		IsCoreWeave:   p.IsCoreWeave,
	})
	if err != nil {
		return fmt.Errorf("create weave project: %w", err)
	}
	p.CreatedAt = row.CreatedAt
	p.UpdatedAt = row.UpdatedAt
	return nil
}

func (s *postgresStore) GetByID(ctx context.Context, id string) (*domain.Project, error) {
	row, err := s.queries.WeaveGetProjectByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get weave project by id: %w", err)
	}
	return rowToProject(row), nil
}

func (s *postgresStore) GetByIDVersion(ctx context.Context, id, version string) (*domain.Project, error) {
	row := s.pool.QueryRow(ctx, `
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
		return nil, fmt.Errorf("get archived weave project by id: %w", err)
	}
	return project, nil
}

func (s *postgresStore) Update(ctx context.Context, p *domain.Project) error {
	if p.Status == "" {
		p.Status = domain.StatusDraft
	}
	row, err := s.queries.WeaveUpdateProject(ctx, sqlcgen.WeaveUpdateProjectParams{
		ID:              p.ID,
		UiName:          marshalTranslations(p.UIName),
		Description:     marshalTranslations(p.Description),
		SystemName:      dbutil.EmptyToNil(p.SystemName),
		Status:          string(p.Status),
		Namespace:       dbutil.EmptyToNil(p.Namespace),
		ParentProjectID: p.ParentProjectID,
		Visibility:      p.Visibility,
		IsCoreWeave:   p.IsCoreWeave,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("update weave project %s: not found", p.ID)
		}
		return fmt.Errorf("update weave project: %w", err)
	}
	p.UpdatedAt = row.UpdatedAt
	return nil
}

func (s *postgresStore) UpdateAbout(ctx context.Context, projectID string, in AboutUpdate) (*domain.Project, error) {
	topics := in.Topics
	if topics == nil {
		topics = []string{}
	}
	row, err := s.queries.WeaveUpdateProjectAbout(ctx, sqlcgen.WeaveUpdateProjectAboutParams{
		ID:      projectID,
		License: in.License,
		Readme:  marshalTranslations(in.README),
		Topics:  topics,
		BaseUrl: in.BaseURL,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("update about for project %s: not found", projectID)
		}
		return nil, fmt.Errorf("update project about: %w", err)
	}
	return rowToProject(row), nil
}

func (s *postgresStore) Delete(ctx context.Context, id string) error {
	if err := s.queries.WeaveDeleteProject(ctx, id); err != nil {
		return fmt.Errorf("delete weave project: %w", err)
	}
	return nil
}

func (s *postgresStore) List(ctx context.Context, opts ...domain.QueryOption) ([]*domain.Project, int64, error) {
	cfg := domain.ApplyOptions(opts)

	limit := int32(cfg.Limit)
	if limit <= 0 {
		limit = 100
	}
	offset := int32(cfg.Offset)
	if offset < 0 {
		offset = 0
	}

	institutionID := ""
	if v, ok := cfg.Filters["institution_id"]; ok {
		if str, ok := v.(string); ok {
			institutionID = str
		}
	}
	ownerID := ""
	if v, ok := cfg.Filters["owner_id"]; ok {
		if str, ok := v.(string); ok {
			ownerID = str
		}
	}
	createdByID := ""
	if v, ok := cfg.Filters["created_by_id"]; ok {
		if str, ok := v.(string); ok {
			createdByID = str
		}
	}

	rows, err := s.queries.WeaveListProjects(ctx, sqlcgen.WeaveListProjectsParams{
		Search:        cfg.Search,
		OwnerID:       ownerID,
		CreatedByID:   createdByID,
		InstitutionID: institutionID,
		SortBy:        cfg.OrderBy,
		SortDesc:      cfg.OrderDesc,
		ResultLimit:   limit,
		ResultOffset:  offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("list weave projects: %w", err)
	}

	total, err := s.queries.WeaveCountProjects(ctx, sqlcgen.WeaveCountProjectsParams{
		Search:        cfg.Search,
		OwnerID:       ownerID,
		CreatedByID:   createdByID,
		InstitutionID: institutionID,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("count weave projects: %w", err)
	}

	out := make([]*domain.Project, 0, len(rows))
	for _, row := range rows {
		out = append(out, rowToProject(row))
	}
	return out, total, nil
}

func (s *postgresStore) ListChildren(ctx context.Context, parentID string) ([]*domain.Project, error) {
	rows, err := s.queries.WeaveListChildProjects(ctx, &parentID)
	if err != nil {
		return nil, fmt.Errorf("list child weave projects: %w", err)
	}
	out := make([]*domain.Project, 0, len(rows))
	for _, row := range rows {
		out = append(out, rowToProject(row))
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Read composition
// ---------------------------------------------------------------------------

func (s *postgresStore) StatsForProjects(ctx context.Context, ids []string) (map[string]*domain.WeaveProjectStats, error) {
	if version := weaveauth.ProjectVersionFromContext(ctx); version != "" {
		return s.statsForProjectsVersion(ctx, ids, version)
	}
	out := make(map[string]*domain.WeaveProjectStats, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	const q = `
		SELECT
			wp.id,
			(SELECT COUNT(*) FROM weave_fields wf WHERE wf.project_id = wp.id) AS field_count,
			(SELECT COUNT(*) FROM weave_models wm WHERE wm.project_id = wp.id) AS model_count,
			(SELECT COUNT(*) FROM weave_collections wc WHERE wc.project_id = wp.id) AS collection_count,
			(SELECT COUNT(*) FROM weave_categories wcat WHERE wcat.project_id = wp.id) AS category_count
		FROM weave_projects wp
		WHERE wp.id = ANY($1)`
	rows, err := s.pool.Query(ctx, q, ids)
	if err != nil {
		return nil, fmt.Errorf("stats for projects: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var stats domain.WeaveProjectStats
		if err := rows.Scan(&id, &stats.FieldCount, &stats.ModelCount, &stats.CollectionCount, &stats.CategoryCount); err != nil {
			return nil, fmt.Errorf("scan project stats: %w", err)
		}
		out[id] = &stats
	}
	return out, rows.Err()
}

func (s *postgresStore) statsForProjectsVersion(ctx context.Context, ids []string, version string) (map[string]*domain.WeaveProjectStats, error) {
	out := make(map[string]*domain.WeaveProjectStats, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	const q = `
		SELECT
			wp.id,
			(SELECT COUNT(*) FROM weave_fields_archive wf WHERE wf.project_id = wp.id AND wf.version_number = $2) AS field_count,
			(SELECT COUNT(*) FROM weave_models_archive wm WHERE wm.project_id = wp.id AND wm.version_number = $2) AS model_count,
			(SELECT COUNT(*) FROM weave_collections_archive wc WHERE wc.project_id = wp.id AND wc.version_number = $2) AS collection_count,
			(SELECT COUNT(*) FROM weave_categories_archive wcat WHERE wcat.project_id = wp.id AND wcat.version_number = $2) AS category_count
		FROM weave_projects_archive wp
		WHERE wp.id = ANY($1) AND wp.version_number = $2`
	rows, err := s.pool.Query(ctx, q, ids, version)
	if err != nil {
		return nil, fmt.Errorf("stats for archived projects: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var stats domain.WeaveProjectStats
		if err := rows.Scan(&id, &stats.FieldCount, &stats.ModelCount, &stats.CollectionCount, &stats.CategoryCount); err != nil {
			return nil, fmt.Errorf("scan archived project stats: %w", err)
		}
		out[id] = &stats
	}
	return out, rows.Err()
}

func (s *postgresStore) ListOwnerInstitutions(ctx context.Context) ([]*domain.ProjectActor, error) {
	const q = `
		SELECT DISTINCT wa.id, wa.type, wa.display_name
		FROM weave_actors wa
		JOIN weave_projects wp ON wp.owner_id = wa.id
		WHERE wa.type = 'organization'
		ORDER BY wa.display_name`
	rows, err := s.pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list owner institutions: %w", err)
	}
	defer rows.Close()

	var out []*domain.ProjectActor
	for rows.Next() {
		var a domain.ProjectActor
		if err := rows.Scan(&a.ID, &a.Type, &a.DisplayName); err != nil {
			return nil, fmt.Errorf("scan institution: %w", err)
		}
		a.Role = "owner"
		out = append(out, &a)
	}
	return out, rows.Err()
}

func (s *postgresStore) OwnersForProjects(ctx context.Context, ids []string) (map[string]*domain.ProjectActor, error) {
	out := make(map[string]*domain.ProjectActor, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	const q = `
		SELECT wp.id, wa.id, wa.type, wa.display_name, 'owner'::text AS role
		FROM weave_projects wp
		JOIN weave_actors wa ON wa.id = wp.owner_id
		WHERE wp.id = ANY($1)`
	rows, err := s.pool.Query(ctx, q, ids)
	if err != nil {
		return nil, fmt.Errorf("owners for projects: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var pid string
		var a domain.ProjectActor
		if err := rows.Scan(&pid, &a.ID, &a.Type, &a.DisplayName, &a.Role); err != nil {
			return nil, fmt.Errorf("scan project owner: %w", err)
		}
		out[pid] = &a
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------------
// Hierarchy
// ---------------------------------------------------------------------------

func (s *postgresStore) GetParentID(ctx context.Context, projectID string) (*string, error) {
	parent, err := s.queries.WeaveGetProjectParentID(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("get project parent id: %w", err)
	}
	return parent, nil
}

// ---------------------------------------------------------------------------
// Row converter + helpers (slice-private)
// ---------------------------------------------------------------------------

func rowToProject(row sqlcgen.WeaveProject) *domain.Project {
	return &domain.Project{
		Entity: domain.Entity{
			ID:          row.ID,
			CreatedAt:   row.CreatedAt,
			UpdatedAt:   row.UpdatedAt,
			SemanticID:  row.ID, // weave_projects.id IS the semantic id
			SystemName:  derefStr(row.SystemName),
			UIName:      unmarshalTranslations(row.UiName),
			Description: unmarshalTranslations(row.Description),
			Status:      domain.Status(row.Status),
		},
		Namespace:       derefStr(row.Namespace),
		ParentProjectID: row.ParentProjectID,
		StagingID:       row.StagingID,
		OwnerID:         row.OwnerID,
		Visibility:      row.Visibility,
		CreatedByID:     row.CreatedByID,
		License:         row.License,
		README:          unmarshalTranslations(row.Readme),
		Topics:          row.Topics,
		BaseURL:         row.BaseUrl,
		IsCoreWeave:   row.IsCoreWeave,
	}
}

func scanArchivedProject(scanner interface{ Scan(...any) error }) (*domain.Project, error) {
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
	if err := scanner.Scan(
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
	return &domain.Project{
		Entity: domain.Entity{
			ID:            id,
			CreatedAt:     createdAt,
			UpdatedAt:     updatedAt,
			SemanticID:    id,
			SystemName:    derefStr(systemName),
			UIName:        unmarshalTranslations(uiName),
			Description:   unmarshalTranslations(description),
			Status:        domain.Status(status),
			VersionNumber: versionNumber,
		},
		Namespace:       derefStr(namespace),
		ParentProjectID: parentProjectID,
		StagingID:       stagingID,
		OwnerID:         ownerID,
		Visibility:      visibility,
		CreatedByID:     createdByID,
		License:         license,
		README:          unmarshalTranslations(readme),
		Topics:          topics,
		BaseURL:         baseURL,
	}, nil
}

func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func marshalTranslations(t domain.Translations) []byte {
	if t == nil {
		return nil
	}
	b, err := json.Marshal(t)
	if err != nil {
		return nil
	}
	return b
}

func unmarshalTranslations(b []byte) domain.Translations {
	if len(b) == 0 {
		return nil
	}
	var t domain.Translations
	if err := json.Unmarshal(b, &t); err != nil {
		return nil
	}
	return t
}
