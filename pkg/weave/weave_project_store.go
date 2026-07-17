package weave

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	weaveauth "github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/database/dbutil"
	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// weaveProjectStore implements domain.WeaveProjectStore backed by
// the weave_projects table via sqlc queries.
type weaveProjectStore struct {
	queries *sqlcgen.Queries
	pool    *pgxpool.Pool
}

type inheritanceTarget struct {
	ProjectID string
	Version   string
}

// Compile-time interface check.
var _ domain.WeaveProjectStore = (*weaveProjectStore)(nil)

// ---------------------------------------------------------------------------
// Row converters
// ---------------------------------------------------------------------------

// weaveRowToProject converts a sqlcgen.WeaveProject row to a *domain.Project.
// The weave_projects table uses id as the semantic ID (no separate semantic_id column).
func weaveRowToProject(row sqlcgen.WeaveProject) *domain.Project {
	return &domain.Project{
		Entity: domain.Entity{
			ID:          row.ID,
			CreatedAt:   row.CreatedAt,
			UpdatedAt:   row.UpdatedAt,
			SemanticID:  row.ID,
			SystemName:  dbutil.NilToEmpty(row.SystemName),
			UIName:      unmarshalDomainTranslations(row.UiName),
			Description: unmarshalDomainTranslations(row.Description),
			Status:      domain.Status(row.Status),
		},
		Namespace:       dbutil.NilToEmpty(row.Namespace),
		ParentProjectID: row.ParentProjectID,
		StagingID:       row.StagingID,
		OwnerID:         row.OwnerID,
		Visibility:      row.Visibility,
		License:         row.License,
		README:          unmarshalDomainTranslations(row.Readme),
		Topics:          row.Topics,
		BaseURL:         row.BaseUrl,
		IsCoreWeave:   row.IsCoreWeave,
	}
}

func scanArchivedWeaveProject(scanner interface{ Scan(...any) error }) (*domain.Project, error) {
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
			SystemName:    dbutil.NilToEmpty(systemName),
			UIName:        unmarshalDomainTranslations(uiName),
			Description:   unmarshalDomainTranslations(description),
			Status:        domain.Status(status),
			VersionNumber: versionNumber,
		},
		Namespace:       dbutil.NilToEmpty(namespace),
		ParentProjectID: parentProjectID,
		StagingID:       stagingID,
		OwnerID:         ownerID,
		Visibility:      visibility,
		License:         license,
		README:          unmarshalDomainTranslations(readme),
		Topics:          topics,
		BaseURL:         baseURL,
		CreatedByID:     createdByID,
	}, nil
}

// ---------------------------------------------------------------------------
// WeaveProjectStore implementation
// ---------------------------------------------------------------------------

// Create inserts a new project into the weave_projects table.
func (s *weaveProjectStore) Create(ctx context.Context, project *domain.Project) error {
	if project.Status == "" {
		project.Status = "draft"
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin create weave project tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	row, err := s.queries.WithTx(tx).WeaveCreateProject(ctx, sqlcgen.WeaveCreateProjectParams{
		ID:              project.ID,
		SystemName:      dbutil.EmptyToNil(project.SystemName),
		UiName:          marshalDomainTranslations(project.UIName),
		Description:     marshalDomainTranslations(project.Description),
		Status:          string(project.Status),
		Namespace:       dbutil.EmptyToNil(project.Namespace),
		ParentProjectID: project.ParentProjectID,
		StagingID:       project.StagingID,
		OwnerID:         project.OwnerID,
		Visibility:      project.Visibility,
		IsCoreWeave:   project.IsCoreWeave,
	})
	if err != nil {
		return fmt.Errorf("create weave project: %w", err)
	}
	if err := syncProjectPrimaryInheritance(ctx, tx, project.ID, project.ParentProjectID); err != nil {
		return fmt.Errorf("sync project inheritance on create: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit create weave project tx: %w", err)
	}

	project.CreatedAt = row.CreatedAt
	project.UpdatedAt = row.UpdatedAt

	return nil
}

// GetByID retrieves a project by its ID. Returns nil, nil if not found.
func (s *weaveProjectStore) GetByID(ctx context.Context, id string) (*domain.Project, error) {
	row, err := s.queries.WeaveGetProjectByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get weave project by id: %w", err)
	}

	return weaveRowToProject(row), nil
}

func (s *weaveProjectStore) GetByIDVersion(ctx context.Context, id, version string) (*domain.Project, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT
			id, created_at, updated_at, system_name, ui_name, description, status,
			namespace, parent_project_id, staging_id, owner_id, visibility,
			deprecated, license, readme, topics, base_url, created_by_id, version_number
		FROM weave_projects_archive
		WHERE id = $1 AND version_number = $2
	`, id, version)
	project, err := scanArchivedWeaveProject(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get archived weave project by id: %w", err)
	}
	return project, nil
}

// Update modifies an existing project in the weave_projects table.
func (s *weaveProjectStore) Update(ctx context.Context, project *domain.Project) error {
	if project.Status == "" {
		project.Status = "draft"
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin update weave project tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	row, err := s.queries.WithTx(tx).WeaveUpdateProject(ctx, sqlcgen.WeaveUpdateProjectParams{
		ID:              project.ID,
		UiName:          marshalDomainTranslations(project.UIName),
		Description:     marshalDomainTranslations(project.Description),
		SystemName:      dbutil.EmptyToNil(project.SystemName),
		Status:          string(project.Status),
		Namespace:       dbutil.EmptyToNil(project.Namespace),
		ParentProjectID: project.ParentProjectID,
		Visibility:      project.Visibility,
		IsCoreWeave:   project.IsCoreWeave,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("update weave project %s: not found", project.ID)
		}
		return fmt.Errorf("update weave project: %w", err)
	}
	if err := syncProjectPrimaryInheritance(ctx, tx, project.ID, project.ParentProjectID); err != nil {
		return fmt.Errorf("sync project inheritance on update: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit update weave project tx: %w", err)
	}

	project.UpdatedAt = row.UpdatedAt

	return nil
}

// UpdateAbout writes only the About columns. See domain.WeaveProjectStore.
func (s *weaveProjectStore) UpdateAbout(ctx context.Context, projectID string, in domain.ProjectAboutUpdate) (*domain.Project, error) {
	topics := in.Topics
	if topics == nil {
		topics = []string{}
	}
	row, err := s.queries.WeaveUpdateProjectAbout(ctx, sqlcgen.WeaveUpdateProjectAboutParams{
		ID:      projectID,
		License: in.License,
		Readme:  marshalDomainTranslations(in.README),
		Topics:  topics,
		BaseUrl: in.BaseURL,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("update about for project %s: not found", projectID)
		}
		return nil, fmt.Errorf("update project about: %w", err)
	}
	return weaveRowToProject(row), nil
}

// Delete removes a project from the weave_projects table.
func (s *weaveProjectStore) Delete(ctx context.Context, id string) error {
	if err := s.queries.WeaveDeleteProject(ctx, id); err != nil {
		return fmt.Errorf("delete weave project: %w", err)
	}
	return nil
}

// List returns projects matching the given query options.
// Returns projects, total count for pagination, and any error.
func (s *weaveProjectStore) List(ctx context.Context, opts ...domain.QueryOption) ([]*domain.Project, int64, error) {
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
		if s, ok := v.(string); ok {
			institutionID = s
		}
	}

	rows, err := s.queries.WeaveListProjects(ctx, sqlcgen.WeaveListProjectsParams{
		Search:        cfg.Search,
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
		InstitutionID: institutionID,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("count weave projects: %w", err)
	}

	result := make([]*domain.Project, 0, len(rows))
	for _, row := range rows {
		result = append(result, weaveRowToProject(row))
	}
	return result, total, nil
}

// ListChildren returns child projects for a given parent project ID.
func (s *weaveProjectStore) ListChildren(ctx context.Context, parentID string) ([]*domain.Project, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT
			wp.id, wp.created_at, wp.updated_at, wp.system_name, wp.ui_name, wp.description,
			wp.status, wp.namespace, wp.parent_project_id, wp.staging_id, wp.owner_id,
			wp.visibility, wp.deprecated, wp.license, wp.readme, wp.topics, wp.base_url,
			wp.created_by_id
		FROM weave_projects wp
		WHERE EXISTS (
			SELECT 1
			FROM weave_project_inheritance wpi
			WHERE wpi.project_id = wp.id
			  AND wpi.parent_project_id = $1
		)
		OR (
			NOT EXISTS (
				SELECT 1
				FROM weave_project_inheritance wpi
				WHERE wpi.project_id = wp.id
			)
			AND wp.parent_project_id = $1
		)
		ORDER BY wp.id ASC
	`, parentID)
	if err != nil {
		return nil, fmt.Errorf("list child weave projects: %w", err)
	}
	defer rows.Close()

	result := make([]*domain.Project, 0)
	for rows.Next() {
		var row sqlcgen.WeaveProject
		if err := rows.Scan(
			&row.ID,
			&row.CreatedAt,
			&row.UpdatedAt,
			&row.SystemName,
			&row.UiName,
			&row.Description,
			&row.Status,
			&row.Namespace,
			&row.ParentProjectID,
			&row.StagingID,
			&row.OwnerID,
			&row.Visibility,
			&row.Deprecated,
			&row.License,
			&row.Readme,
			&row.Topics,
			&row.BaseUrl,
			&row.CreatedByID,
		); err != nil {
			return nil, fmt.Errorf("scan child weave project: %w", err)
		}
		result = append(result, weaveRowToProject(row))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate child weave projects: %w", err)
	}

	return result, nil
}

// StatsForProjects returns entity counts for the given project IDs by querying weave tables directly.
func (s *weaveProjectStore) StatsForProjects(ctx context.Context, projectIDs []string) (map[string]*domain.WeaveProjectStats, error) {
	if version := weaveauth.ProjectVersionFromContext(ctx); version != "" {
		return s.statsForProjectsVersion(ctx, projectIDs, version)
	}
	result := make(map[string]*domain.WeaveProjectStats, len(projectIDs))
	if len(projectIDs) == 0 {
		return result, nil
	}

	query := `
		SELECT
			wp.id,
			(SELECT COUNT(*) FROM weave_fields wf WHERE wf.project_id = wp.id) AS field_count,
			(SELECT COUNT(*) FROM weave_models wm WHERE wm.project_id = wp.id) AS model_count,
			(SELECT COUNT(*) FROM weave_collections wc WHERE wc.project_id = wp.id) AS collection_count,
			(SELECT COUNT(*) FROM weave_categories wcat WHERE wcat.project_id = wp.id) AS category_count
		FROM weave_projects wp
		WHERE wp.id = ANY($1)`

	rows, err := s.pool.Query(ctx, query, projectIDs)
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
		result[id] = &stats
	}

	return result, rows.Err()
}

func (s *weaveProjectStore) statsForProjectsVersion(ctx context.Context, projectIDs []string, version string) (map[string]*domain.WeaveProjectStats, error) {
	result := make(map[string]*domain.WeaveProjectStats, len(projectIDs))
	if len(projectIDs) == 0 {
		return result, nil
	}

	query := `
		SELECT
			wp.id,
			(SELECT COUNT(*) FROM weave_fields_archive wf WHERE wf.project_id = wp.id AND wf.version_number = $2) AS field_count,
			(SELECT COUNT(*) FROM weave_models_archive wm WHERE wm.project_id = wp.id AND wm.version_number = $2) AS model_count,
			(SELECT COUNT(*) FROM weave_collections_archive wc WHERE wc.project_id = wp.id AND wc.version_number = $2) AS collection_count,
			(SELECT COUNT(*) FROM weave_categories_archive wcat WHERE wcat.project_id = wp.id AND wcat.version_number = $2) AS category_count
		FROM weave_projects_archive wp
		WHERE wp.id = ANY($1) AND wp.version_number = $2`

	rows, err := s.pool.Query(ctx, query, projectIDs, version)
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
		result[id] = &stats
	}

	return result, rows.Err()
}

// ListOwnerInstitutions returns all institutions that own at least one project, sorted by name.
func (s *weaveProjectStore) ListOwnerInstitutions(ctx context.Context) ([]*domain.ProjectActor, error) {
	query := `
		SELECT DISTINCT wa.id, wa.type, wa.display_name
		FROM weave_actors wa
		JOIN weave_project_actors wpa ON wpa.actor_id = wa.id
		WHERE wpa.role = 'owner' AND wa.type = 'institution'
		ORDER BY wa.display_name`

	rows, err := s.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list owner institutions: %w", err)
	}
	defer rows.Close()

	var result []*domain.ProjectActor
	for rows.Next() {
		var a domain.ProjectActor
		if err := rows.Scan(&a.ID, &a.Type, &a.DisplayName); err != nil {
			return nil, fmt.Errorf("scan institution: %w", err)
		}
		a.Role = "owner"
		result = append(result, &a)
	}
	return result, rows.Err()
}

// OwnersForProjects returns the owning institution (role='owner') for each project ID.
// Returns a map keyed by project_id; projects without an owner are absent from the map.
func (s *weaveProjectStore) OwnersForProjects(ctx context.Context, projectIDs []string) (map[string]*domain.ProjectActor, error) {
	result := make(map[string]*domain.ProjectActor, len(projectIDs))
	if len(projectIDs) == 0 {
		return result, nil
	}

	query := `
		SELECT wpa.project_id, wa.id, wa.type, wa.display_name, wpa.role
		FROM weave_project_actors wpa
		JOIN weave_actors wa ON wa.id = wpa.actor_id
		WHERE wpa.project_id = ANY($1) AND wpa.role = 'owner'`

	rows, err := s.pool.Query(ctx, query, projectIDs)
	if err != nil {
		return nil, fmt.Errorf("owners for projects: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var projectID string
		var actor domain.ProjectActor
		if err := rows.Scan(&projectID, &actor.ID, &actor.Type, &actor.DisplayName, &actor.Role); err != nil {
			return nil, fmt.Errorf("scan project owner: %w", err)
		}
		result[projectID] = &actor
	}

	return result, rows.Err()
}

// LinkedOntologies removed. Canonical reader is
// pkg/weave/projectontologyversion.Service.LinkedOntologies — see the
// note on domain.WeaveProjectStore for context.

// GetParentID returns the parent project's ID for the given project.
// Returns (nil, nil) when the project has no parent.
// Returns an error when the project does not exist.
func (s *weaveProjectStore) GetParentID(ctx context.Context, projectID string) (*string, error) {
	parents, err := s.listParentProjectIDs(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("get project parent id: %w", err)
	}
	if len(parents) == 0 {
		return nil, nil
	}
	parent := parents[0]
	return &parent, nil
}

const maxParentDepth = 10

// ResolvedOntologyVersions walks the inheritance graph (bounded depth 10)
// and returns all ontology-version links, deduped by ontology_version_id —
// the current project's row wins over ancestors.
func (s *weaveProjectStore) ResolvedOntologyVersions(ctx context.Context, projectID string, opts domain.ResolvedOntologyVersionOpts) ([]domain.ResolvedOntologyVersion, error) {
	if version := weaveauth.ProjectVersionFromContext(ctx); version != "" {
		return s.resolvedOntologyVersionsArchived(ctx, projectID, version, opts)
	}
	visited := make(map[string]bool)
	seenVersion := make(map[string]bool)
	out := make([]domain.ResolvedOntologyVersion, 0)

	queue := []inheritanceTarget{{ProjectID: projectID}}
	for depth := 0; len(queue) > 0 && depth < maxParentDepth; depth++ {
		current := queue[0]
		queue = queue[1:]
		if visited[current.ProjectID] {
			continue
		}
		visited[current.ProjectID] = true
		isOwn := current.ProjectID == projectID

		if !(opts.OnlyInherited && isOwn) {
			if err := s.appendResolvedOntologyVersionsForTarget(ctx, current, projectID, seenVersion, &out); err != nil {
				return nil, err
			}
		}

		parents, err := s.listParentTargets(ctx, current)
		if err != nil {
			return nil, fmt.Errorf("list parents of %s: %w", current.ProjectID, err)
		}
		for _, parent := range parents {
			if parent.ProjectID == "" || visited[parent.ProjectID] {
				continue
			}
			queue = append(queue, parent)
		}
	}

	if err := s.populateVersionLabels(ctx, out); err != nil {
		return nil, fmt.Errorf("resolve version labels: %w", err)
	}
	return out, nil
}

func (s *weaveProjectStore) resolvedOntologyVersionsArchived(ctx context.Context, projectID, version string, opts domain.ResolvedOntologyVersionOpts) ([]domain.ResolvedOntologyVersion, error) {
	visited := make(map[string]bool)
	seenVersion := make(map[string]bool)
	out := make([]domain.ResolvedOntologyVersion, 0)

	queue := []inheritanceTarget{{ProjectID: projectID, Version: version}}
	for depth := 0; len(queue) > 0 && depth < maxParentDepth; depth++ {
		current := queue[0]
		queue = queue[1:]
		if visited[current.ProjectID] {
			continue
		}
		visited[current.ProjectID] = true
		isOwn := current.ProjectID == projectID

		if !(opts.OnlyInherited && isOwn) {
			if err := s.appendResolvedOntologyVersionsForTarget(ctx, current, projectID, seenVersion, &out); err != nil {
				return nil, err
			}
		}

		parents, err := s.listParentTargets(ctx, current)
		if err != nil {
			return nil, fmt.Errorf("list archived parents of %s: %w", current.ProjectID, err)
		}
		for _, parent := range parents {
			if parent.ProjectID == "" || visited[parent.ProjectID] {
				continue
			}
			queue = append(queue, parent)
		}
	}

	if err := s.populateVersionLabels(ctx, out); err != nil {
		return nil, fmt.Errorf("resolve version labels: %w", err)
	}
	return out, nil
}

func (s *weaveProjectStore) appendResolvedOntologyVersionsForTarget(ctx context.Context, target inheritanceTarget, rootProjectID string, seenVersion map[string]bool, out *[]domain.ResolvedOntologyVersion) error {
	if strings.TrimSpace(target.Version) == "" {
		rows, err := s.queries.WeaveListProjectOntologyVersions(ctx, target.ProjectID)
		if err != nil {
			return fmt.Errorf("list project ontology versions for %s: %w", target.ProjectID, err)
		}
		for _, r := range rows {
			if seenVersion[r.OntologyVersionID] {
				continue
			}
			seenVersion[r.OntologyVersionID] = true
			entry := domain.ResolvedOntologyVersion{
				Link:   rowToProjectOntologyVersion(r),
				Origin: domain.OwnOrigin(),
			}
			if target.ProjectID != rootProjectID {
				entry.SourceProjectID = target.ProjectID
				entry.Origin = domain.InheritedOrigin(target.ProjectID)
			}
			*out = append(*out, entry)
		}
		return nil
	}

	rows, err := s.pool.Query(ctx, `
		SELECT project_id, ontology_version_id, added_at, added_by_id, is_primary, usage_notes
		FROM weave_project_ontology_versions_archive
		WHERE project_id = $1 AND version_number = $2
	`, target.ProjectID, target.Version)
	if err != nil {
		return fmt.Errorf("list archived project ontology versions for %s@%s: %w", target.ProjectID, target.Version, err)
	}
	defer rows.Close()
	for rows.Next() {
		link, err := scanArchivedProjectOntologyVersion(rows)
		if err != nil {
			return fmt.Errorf("scan archived project ontology version for %s@%s: %w", target.ProjectID, target.Version, err)
		}
		if seenVersion[link.OntologyVersionID] {
			continue
		}
		seenVersion[link.OntologyVersionID] = true
		entry := domain.ResolvedOntologyVersion{
			Link:   link,
			Origin: domain.OwnOrigin(),
		}
		if target.ProjectID != rootProjectID {
			entry.SourceProjectID = target.ProjectID
			entry.Origin = domain.InheritedOrigin(target.ProjectID)
		}
		*out = append(*out, entry)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate archived project ontology versions for %s@%s: %w", target.ProjectID, target.Version, err)
	}
	return nil
}

func scanArchivedProjectOntologyVersion(scanner interface{ Scan(...any) error }) (*domain.ProjectOntologyVersion, error) {
	var (
		projectID, ontologyVersionID string
		addedAt                      time.Time
		addedByID                    *string
		isPrimary                    *bool
		usageNotes                   *string
	)
	if err := scanner.Scan(&projectID, &ontologyVersionID, &addedAt, &addedByID, &isPrimary, &usageNotes); err != nil {
		return nil, err
	}
	link := &domain.ProjectOntologyVersion{
		ProjectID:         projectID,
		OntologyVersionID: ontologyVersionID,
		AddedAt:           addedAt,
	}
	if addedByID != nil {
		s := *addedByID
		link.AddedByID = &s
	}
	if isPrimary != nil {
		link.IsPrimary = *isPrimary
	}
	if usageNotes != nil {
		link.UsageNotes = *usageNotes
	}
	return link, nil
}

// populateVersionLabels enriches each entry in rows with OntologyName and
// VersionString, joining ontologies → ontology_versions in a single query.
// Mutates rows in place. No-op when rows is empty.
func (s *weaveProjectStore) populateVersionLabels(ctx context.Context, rows []domain.ResolvedOntologyVersion) error {
	if len(rows) == 0 {
		return nil
	}
	ids := make([]string, 0, len(rows))
	seen := make(map[string]struct{}, len(rows))
	for _, r := range rows {
		if r.Link == nil {
			continue
		}
		id := r.Link.OntologyVersionID
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil
	}

	type label struct {
		Name    string
		Prefix  string
		Version string
	}
	// Migration 029 renamed ontology_versions → weave_ontology_versions
	// and ontologies → weave_ontologies, and dropped the GORM-era
	// ui_name/system_name fallback columns. weave_ontologies.name is
	// canonical now. The renderer consumes Prefix to derive
	// deterministic ontology UUIDs under ids.OntologyNamespace, so this
	// query carries both.
	rs, err := s.pool.Query(ctx,
		`SELECT ov.id,
		        COALESCE(NULLIF(o.name, ''), ov.ontology_id) AS name,
		        COALESCE(o.prefix, '') AS prefix,
		        COALESCE(ov.version_string, '') AS version
		 FROM weave_ontology_versions ov
		 LEFT JOIN weave_ontologies o ON o.id = ov.ontology_id
		 WHERE ov.id = ANY($1)`,
		ids,
	)
	if err != nil {
		return fmt.Errorf("query version labels: %w", err)
	}
	defer rs.Close()

	labels := make(map[string]label, len(ids))
	for rs.Next() {
		var id string
		var l label
		if err := rs.Scan(&id, &l.Name, &l.Prefix, &l.Version); err != nil {
			return fmt.Errorf("scan version label: %w", err)
		}
		labels[id] = l
	}
	if err := rs.Err(); err != nil {
		return fmt.Errorf("iterate version labels: %w", err)
	}

	for i := range rows {
		if rows[i].Link == nil {
			continue
		}
		if l, ok := labels[rows[i].Link.OntologyVersionID]; ok {
			rows[i].OntologyName = l.Name
			rows[i].OntologyPrefix = l.Prefix
			rows[i].VersionString = l.Version
		}
	}
	return nil
}

func (s *weaveProjectStore) listParentProjectIDs(ctx context.Context, projectID string) ([]string, error) {
	targets, err := s.listParentTargets(ctx, inheritanceTarget{ProjectID: projectID})
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(targets))
	for _, target := range targets {
		out = append(out, target.ProjectID)
	}
	return out, nil
}

// ProjectChain returns the inheritance chain starting with projectID
// itself followed by its ancestors in BFS order (parent, grandparent,
// …). Bounded by maxParentDepth. Drives any picker/options call that
// needs to surface inherited entities.
func (s *weaveProjectStore) ProjectChain(ctx context.Context, projectID string) ([]string, error) {
	if projectID == "" {
		return nil, nil
	}
	visited := map[string]bool{}
	out := make([]string, 0, 4)
	queue := []inheritanceTarget{{ProjectID: projectID}}
	for depth := 0; len(queue) > 0 && depth < maxParentDepth; depth++ {
		current := queue[0]
		queue = queue[1:]
		if current.ProjectID == "" || visited[current.ProjectID] {
			continue
		}
		visited[current.ProjectID] = true
		out = append(out, current.ProjectID)
		parents, err := s.listParentTargets(ctx, current)
		if err != nil {
			return nil, fmt.Errorf("list parents of %s: %w", current.ProjectID, err)
		}
		for _, parent := range parents {
			if parent.ProjectID == "" || visited[parent.ProjectID] {
				continue
			}
			queue = append(queue, parent)
		}
	}
	return out, nil
}

func (s *weaveProjectStore) listParentTargets(ctx context.Context, target inheritanceTarget) ([]inheritanceTarget, error) {
	if strings.TrimSpace(target.Version) != "" {
		return s.listParentTargetsArchived(ctx, target.ProjectID, target.Version)
	}
	return s.listParentTargetsDraft(ctx, target.ProjectID)
}

func (s *weaveProjectStore) listParentTargetsDraft(ctx context.Context, projectID string) ([]inheritanceTarget, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT parent_project_id, source_mode, COALESCE(source_version, '')
		FROM weave_project_inheritance
		WHERE project_id = $1
		ORDER BY CASE WHEN is_primary THEN 0 ELSE 1 END, canonical_order ASC, parent_project_id ASC
	`, projectID)
	if err != nil {
		return nil, fmt.Errorf("query project inheritances: %w", err)
	}
	defer rows.Close()

	out := make([]inheritanceTarget, 0)
	for rows.Next() {
		var parentID string
		var sourceMode string
		var sourceVersion string
		if err := rows.Scan(&parentID, &sourceMode, &sourceVersion); err != nil {
			return nil, fmt.Errorf("scan project inheritance parent: %w", err)
		}
		next := inheritanceTarget{ProjectID: parentID}
		if normalizeInheritanceSourceMode(domain.DependencySourceMode(sourceMode)) == domain.DependencySourceRelease {
			next.Version = strings.TrimSpace(sourceVersion)
		}
		out = append(out, next)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate project inheritances: %w", err)
	}
	if len(out) > 0 {
		return out, nil
	}

	parent, err := s.queries.WeaveGetProjectParentID(ctx, projectID)
	if err != nil {
		return nil, err
	}
	if parent == nil || *parent == "" {
		return nil, nil
	}
	return []inheritanceTarget{{ProjectID: *parent}}, nil
}

func (s *weaveProjectStore) listParentProjectIDsArchived(ctx context.Context, projectID, version string) ([]string, error) {
	targets, err := s.listParentTargetsArchived(ctx, projectID, version)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(targets))
	for _, target := range targets {
		out = append(out, target.ProjectID)
	}
	return out, nil
}

func (s *weaveProjectStore) listParentTargetsArchived(ctx context.Context, projectID, version string) ([]inheritanceTarget, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT parent_project_id, source_mode, COALESCE(source_version, '')
		FROM weave_project_inheritance_archive
		WHERE project_id = $1 AND version_number = $2
		ORDER BY CASE WHEN is_primary THEN 0 ELSE 1 END, canonical_order ASC, parent_project_id ASC
	`, projectID, version)
	if err != nil {
		return nil, fmt.Errorf("query archived project inheritances: %w", err)
	}
	defer rows.Close()

	out := make([]inheritanceTarget, 0)
	for rows.Next() {
		var parentID string
		var sourceMode string
		var sourceVersion string
		if err := rows.Scan(&parentID, &sourceMode, &sourceVersion); err != nil {
			return nil, fmt.Errorf("scan archived project inheritance parent: %w", err)
		}
		next := inheritanceTarget{ProjectID: parentID, Version: version}
		out = append(out, next)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate archived project inheritances: %w", err)
	}
	if len(out) > 0 {
		return out, nil
	}

	var parent *string
	if err := s.pool.QueryRow(ctx, `
		SELECT parent_project_id
		FROM weave_projects_archive
		WHERE id = $1 AND version_number = $2
	`, projectID, version).Scan(&parent); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("query archived project parent fallback: %w", err)
	}
	if parent == nil || *parent == "" {
		return nil, nil
	}
	return []inheritanceTarget{{ProjectID: *parent, Version: version}}, nil
}

func syncProjectPrimaryInheritance(ctx context.Context, tx pgx.Tx, projectID string, parentID *string) error {
	if _, err := tx.Exec(ctx, `
		DELETE FROM weave_project_inheritance
		WHERE project_id = $1
	`, projectID); err != nil {
		return fmt.Errorf("clear project inheritances: %w", err)
	}
	if parentID == nil || *parentID == "" {
		return nil
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO weave_project_inheritance (
			project_id, parent_project_id, is_primary, canonical_order
		) VALUES ($1, $2, true, 0)
	`, projectID, *parentID); err != nil {
		return fmt.Errorf("insert primary project inheritance: %w", err)
	}
	return nil
}
