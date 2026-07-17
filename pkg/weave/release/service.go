package release

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	weaveauth "github.com/pletka-io/pletka/pkg/auth"
)

// semverRE matches MAJOR.MINOR.PATCH with an optional pre-release or
// build-metadata suffix per https://semver.org. The regex is permissive
// on the suffix shape (any [0-9A-Za-z.-]+) — strict enough to reject
// shorthand like "0.5" or "1.0" that produced opaque snapshot failures
// previously, loose enough to allow common pre-release labels like
// "1.0.0-rc.1" or "1.0.0+build.42" without re-implementing the full
// grammar.
var semverRE = regexp.MustCompile(`^\d+\.\d+\.\d+(?:[-+][0-9A-Za-z.-]+)?$`)

type ErrValidation struct {
	Fields map[string][]string
}

func (e *ErrValidation) Error() string { return "validation error" }

type ErrConflict struct {
	Message string
}

func (e *ErrConflict) Error() string { return e.Message }

type CreateInput struct {
	Version     string `json:"version"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

type permissionDenied struct {
	capability weaveauth.Capability
}

func (e *permissionDenied) Error() string {
	return fmt.Sprintf("forbidden: requires %s", e.capability)
}

type Service struct {
	store Store
	// sanctioned pool holder: tx-owning service — see docs-oss/architecture/slices.md
	pool *pgxpool.Pool
	log  *slog.Logger
}

func NewService(store Store, pool *pgxpool.Pool, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{store: store, pool: pool, log: log}
}

func (s *Service) ListByProject(ctx context.Context, projectID string) ([]Release, error) {
	if err := s.requireRead(ctx); err != nil {
		return nil, err
	}
	return s.store.ListByProject(ctx, projectID)
}

func (s *Service) Get(ctx context.Context, projectID, version string) (*Release, error) {
	if err := s.requireRead(ctx); err != nil {
		return nil, err
	}
	return s.store.Get(ctx, projectID, version)
}

func (s *Service) Create(ctx context.Context, projectID string, in CreateInput) (*Release, error) {
	if err := s.requireEdit(ctx); err != nil {
		return nil, err
	}
	if errs := validateCreate(in); len(errs) > 0 {
		return nil, &ErrValidation{Fields: errs}
	}

	principal := weaveauth.PrincipalFromContext(ctx)
	if principal == nil || principal.ActorID == "" {
		return nil, errors.New("release: missing principal")
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin release tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var draftCount int64
	if err := tx.QueryRow(ctx, countDraftSnapshotCandidatesSQL, projectID).Scan(&draftCount); err != nil {
		return nil, fmt.Errorf("count release snapshot candidates: %w", err)
	}
	if draftCount == 0 {
		return nil, &ErrValidation{Fields: map[string][]string{
			"version": {"cannot create a release for an empty project"},
		}}
	}
	blockingParents, err := findDraftParentDependencies(ctx, tx, projectID)
	if err != nil {
		return nil, fmt.Errorf("check draft parent dependencies: %w", err)
	}
	if len(blockingParents) > 0 {
		return nil, &ErrValidation{Fields: map[string][]string{
			"parent_dependencies": {fmt.Sprintf("cannot create a release while parent dependencies still follow draft: %s", strings.Join(blockingParents, ", "))},
		}}
	}

	var created Release
	err = tx.QueryRow(ctx, `
		INSERT INTO weave_releases (project_id, version, title, description, created_by_id)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING project_id, version, title, description, created_at, created_by_id
	`, projectID, in.Version, strings.TrimSpace(in.Title), strings.TrimSpace(in.Description), principal.ActorID).Scan(
		&created.ProjectID,
		&created.Version,
		&created.Title,
		&created.Description,
		&created.CreatedAt,
		&created.CreatedByID,
	)
	if err != nil {
		var pgerr *pgconn.PgError
		if errors.As(err, &pgerr) && pgerr.ConstraintName == "weave_releases_pkey" {
			return nil, &ErrConflict{Message: "release version already exists"}
		}
		return nil, fmt.Errorf("insert release metadata: %w", err)
	}

	for _, stmt := range snapshotStatements {
		if _, err := tx.Exec(ctx, stmt, projectID, in.Version); err != nil {
			return nil, fmt.Errorf("snapshot release state: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit release tx: %w", err)
	}
	return &created, nil
}

func (s *Service) requireRead(ctx context.Context) error {
	snap := weaveauth.FromContext(ctx)
	res := weaveauth.ProjectResourceFromContext(ctx)
	if !snap.Can(weaveauth.ProjectRead, res, nil) {
		return &permissionDenied{capability: weaveauth.ProjectRead}
	}
	return nil
}

func (s *Service) requireEdit(ctx context.Context) error {
	snap := weaveauth.FromContext(ctx)
	res := weaveauth.ProjectResourceFromContext(ctx)
	if !snap.Can(weaveauth.ProjectEdit, res, nil) {
		return &permissionDenied{capability: weaveauth.ProjectEdit}
	}
	return nil
}

func validateCreate(in CreateInput) map[string][]string {
	errs := map[string][]string{}
	version := strings.TrimSpace(in.Version)
	switch {
	case version == "":
		errs["version"] = append(errs["version"], "version is required")
	case !semverRE.MatchString(version):
		errs["version"] = append(errs["version"], "version must be in MAJOR.MINOR.PATCH form (for example 1.0.0)")
	}
	return errs
}

func findDraftParentDependencies(ctx context.Context, tx pgx.Tx, projectID string) ([]string, error) {
	rows, err := tx.Query(ctx, `
		SELECT parent_project_id
		FROM weave_project_inheritance
		WHERE project_id = $1
		  AND source_mode = 'draft'
		ORDER BY CASE WHEN is_primary THEN 0 ELSE 1 END, canonical_order ASC, parent_project_id ASC
	`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]string, 0)
	for rows.Next() {
		var parentID string
		if err := rows.Scan(&parentID); err != nil {
			return nil, err
		}
		out = append(out, parentID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

const countDraftSnapshotCandidatesSQL = `
	SELECT
		(SELECT COUNT(*) FROM weave_categories WHERE project_id = $1) +
		(SELECT COUNT(*) FROM weave_fields WHERE project_id = $1) +
		(SELECT COUNT(*) FROM weave_models WHERE project_id = $1) +
		(SELECT COUNT(*) FROM weave_collections WHERE project_id = $1) +
		(SELECT COUNT(*) FROM weave_field_overrides WHERE project_id = $1) +
		(SELECT COUNT(*) FROM weave_project_ontology_versions WHERE project_id = $1) +
		(SELECT COUNT(*) FROM weave_namespace_bindings WHERE project_id = $1)
`

const snapshotProjectChainCTE = `
WITH RECURSIVE project_chain AS (
    SELECT p.id AS project_id, 'draft'::text AS source_mode, NULL::text AS source_version, 0 AS depth, ARRAY[p.id] AS path
    FROM weave_projects p
    WHERE p.id = $1
    UNION ALL
    SELECT next_parent.parent_id AS project_id, next_parent.source_mode, next_parent.source_version, pc.depth + 1, pc.path || next_parent.parent_id
    FROM project_chain pc
    JOIN LATERAL (
        SELECT wpi.parent_project_id AS parent_id,
               wpi.source_mode AS source_mode,
               wpi.source_version AS source_version
        FROM weave_project_inheritance wpi
        WHERE pc.source_mode = 'draft'
          AND wpi.project_id = pc.project_id
        UNION ALL
        SELECT wpia.parent_project_id AS parent_id,
               wpia.source_mode AS source_mode,
               wpia.source_version AS source_version
        FROM weave_project_inheritance_archive wpia
        WHERE pc.source_mode = 'release'
          AND wpia.project_id = pc.project_id
          AND wpia.version_number = pc.source_version
        UNION ALL
        SELECT p.parent_project_id AS parent_id,
               'draft'::text AS source_mode,
               NULL::text AS source_version
        FROM weave_projects p
        WHERE pc.source_mode = 'draft'
          AND p.id = pc.project_id
          AND p.parent_project_id IS NOT NULL
          AND NOT EXISTS (
              SELECT 1 FROM weave_project_inheritance wpi
              WHERE wpi.project_id = pc.project_id
          )
    ) next_parent ON TRUE
    WHERE pc.depth < 10
      AND next_parent.parent_id IS NOT NULL
      AND next_parent.parent_id <> ''
      AND NOT (next_parent.parent_id = ANY(pc.path))
)`

var snapshotStatements = []string{
	snapshotProjectChainCTE + `
	INSERT INTO weave_project_inheritance_archive (
		project_id, parent_project_id, is_primary, canonical_order, adopted_at, source_mode, source_version, version_number
	)
	SELECT
		src.project_id, src.parent_project_id, src.is_primary, src.canonical_order, src.adopted_at, src.source_mode, src.source_version, $2
	FROM (
		SELECT wpi.project_id, wpi.parent_project_id, wpi.is_primary, wpi.canonical_order, wpi.adopted_at, wpi.source_mode, wpi.source_version
		FROM weave_project_inheritance wpi
		JOIN project_chain pc ON pc.project_id = wpi.project_id
		WHERE pc.source_mode = 'draft'
		UNION ALL
		SELECT wpia.project_id, wpia.parent_project_id, wpia.is_primary, wpia.canonical_order, wpia.adopted_at, wpia.source_mode, wpia.source_version
		FROM weave_project_inheritance_archive wpia
		JOIN project_chain pc ON pc.project_id = wpia.project_id
		WHERE pc.source_mode = 'release'
		  AND wpia.version_number = pc.source_version
	) src
	ON CONFLICT (project_id, parent_project_id, version_number) DO NOTHING`,
	snapshotProjectChainCTE + `
	INSERT INTO weave_projects_archive (
		id, created_at, updated_at, system_name, ui_name, description, status,
		namespace, parent_project_id, staging_id, owner_id, visibility,
		deprecated, license, readme, topics, base_url, created_by_id, version_number
	)
	SELECT
		src.id, src.created_at, src.updated_at, src.system_name, src.ui_name, src.description,
		CASE WHEN src.status = 'deprecated' THEN src.status ELSE 'published' END,
		src.namespace, src.parent_project_id, src.staging_id, src.owner_id, src.visibility,
		src.deprecated, src.license, src.readme, src.topics, src.base_url, src.created_by_id, $2
	FROM (
		SELECT p.id, p.created_at, p.updated_at, p.system_name, p.ui_name, p.description, p.status,
		       p.namespace, p.parent_project_id, p.staging_id, p.owner_id, p.visibility,
		       p.deprecated, p.license, p.readme, p.topics, p.base_url, p.created_by_id
		FROM weave_projects p
		JOIN project_chain pc ON pc.project_id = p.id
		WHERE pc.source_mode = 'draft'
		UNION ALL
		SELECT p.id, p.created_at, p.updated_at, p.system_name, p.ui_name, p.description, p.status,
		       p.namespace, p.parent_project_id, p.staging_id, p.owner_id, p.visibility,
		       p.deprecated, p.license, p.readme, p.topics, p.base_url, p.created_by_id
		FROM weave_projects_archive p
		JOIN project_chain pc ON pc.project_id = p.id
		WHERE pc.source_mode = 'release'
		  AND p.version_number = pc.source_version
	) src
	ON CONFLICT (id, version_number) DO NOTHING`,
	`INSERT INTO weave_categories_archive (
		id, created_at, updated_at, semantic_id, system_name, ui_name, description,
		status, project_id, canonical_order, deprecated, version_number
	)
	SELECT
		id, created_at, updated_at, semantic_id, system_name, ui_name, description,
		CASE WHEN status = 'deprecated' THEN status ELSE 'published' END, project_id, canonical_order, deprecated, $2
	FROM weave_categories
	WHERE project_id = $1`,
	`INSERT INTO weave_fields_archive (
		id, created_at, updated_at, semantic_id, system_name, ui_name, description,
		status, project_id, ontology_scope, ontology_path, path_elements,
		expected_value_type, examples, staging_id, deprecated, version_number
	)
	SELECT
		id, created_at, updated_at, semantic_id, system_name, ui_name, description,
		CASE WHEN status = 'deprecated' THEN status ELSE 'published' END, project_id, ontology_scope, ontology_path, path_elements,
		expected_value_type, examples, staging_id, deprecated, $2
	FROM weave_fields
	WHERE project_id = $1`,
	`INSERT INTO weave_models_archive (
		id, created_at, updated_at, system_name, ui_name, description,
		status, project_id, ontology_scope, staging_id, deprecated, version_number
	)
	SELECT
		id, created_at, updated_at, system_name, ui_name, description,
		CASE WHEN status = 'deprecated' THEN status ELSE 'published' END, project_id, ontology_scope, staging_id, deprecated, $2
	FROM weave_models
	WHERE project_id = $1`,
	`INSERT INTO weave_collections_archive (
		id, created_at, updated_at, system_name, ui_name, description,
		status, project_id, ontology_scope, collection_number,
		canonical_collection_order, staging_id, deprecated, default_category_id, version_number
	)
	SELECT
		id, created_at, updated_at, system_name, ui_name, description,
		CASE WHEN status = 'deprecated' THEN status ELSE 'published' END, project_id, ontology_scope, collection_number,
		canonical_collection_order, staging_id, deprecated, default_category_id, $2
	FROM weave_collections
	WHERE project_id = $1`,
	`INSERT INTO weave_concept_lists_archive (
		id, created_at, updated_at, semantic_id, system_name, ui_name, description,
		status, project_id, list_type, vocabulary_id, version_number
	)
	SELECT
		id, created_at, updated_at, semantic_id, system_name, ui_name, description,
		CASE WHEN status = 'deprecated' THEN status ELSE 'published' END, project_id, list_type, vocabulary_id, $2
	FROM weave_concept_lists
	WHERE project_id = $1
	ON CONFLICT (id, version_number) DO NOTHING`,
	`INSERT INTO weave_concept_list_entries_archive (
		id, concept_list_id, vocabulary_entry_id, position, custom_label,
		created_at, updated_at, version_number
	)
	SELECT
		e.id, e.concept_list_id, e.vocabulary_entry_id, e.position, e.custom_label,
		e.created_at, e.updated_at, $2
	FROM weave_concept_list_entries e
	JOIN weave_concept_lists cl ON cl.id = e.concept_list_id
	WHERE cl.project_id = $1
	ON CONFLICT (id, version_number) DO NOTHING`,
	`INSERT INTO weave_field_overrides_archive (
		id, field_id, project_id, entity_type, entity_id, position, collection_order,
		display_name, description, collection_name, category_id, part_of_collection_id,
		expected_value_type, set_value, is_required, min_occurs, max_occurs,
		is_hidden, visibility, staging_id, created_at, updated_at, content_hash, version_number
	)
	SELECT
		id, field_id, project_id, entity_type, entity_id, position, collection_order,
		display_name, description, collection_name, category_id, part_of_collection_id,
		expected_value_type, set_value, is_required, min_occurs, max_occurs,
		is_hidden, visibility, staging_id, created_at, updated_at, content_hash, $2
	FROM weave_field_overrides
	WHERE project_id = $1`,
	snapshotProjectChainCTE + `
	INSERT INTO weave_project_ontology_versions_archive (
		project_id, ontology_version_id, added_at, added_by_id, is_primary, usage_notes, version_number
	)
	SELECT
		src.project_id, src.ontology_version_id, src.added_at, src.added_by_id, src.is_primary, src.usage_notes, $2
	FROM (
		SELECT pov.project_id, pov.ontology_version_id, pov.added_at, pov.added_by_id, pov.is_primary, pov.usage_notes
		FROM weave_project_ontology_versions pov
		JOIN project_chain pc ON pc.project_id = pov.project_id
		WHERE pc.source_mode = 'draft'
		UNION ALL
		SELECT pov.project_id, pov.ontology_version_id, pov.added_at, pov.added_by_id, pov.is_primary, pov.usage_notes
		FROM weave_project_ontology_versions_archive pov
		JOIN project_chain pc ON pc.project_id = pov.project_id
		WHERE pc.source_mode = 'release'
		  AND pov.version_number = pc.source_version
	) src
	ON CONFLICT (project_id, ontology_version_id, version_number) DO NOTHING`,
	`INSERT INTO weave_namespace_bindings_archive (
		id, created_at, updated_at, airtable_raw, airtable_context, airtable_ref,
		semantic_id, long_semantic_id, system_name, ui_name, description, uri,
		project_id, prefix, namespace, weight, source, ontology_id, version_number
	)
	SELECT
		id, created_at, updated_at, airtable_raw, airtable_context, airtable_ref,
		semantic_id, long_semantic_id, system_name, ui_name, description, uri,
		project_id, prefix, namespace, weight, source, ontology_id, $2
	FROM weave_namespace_bindings
	WHERE project_id = $1 OR project_id IS NULL OR project_id = ''
	ON CONFLICT (id, version_number) DO NOTHING`,
	`INSERT INTO weave_override_refs_archive (
		override_id, ref_type, target_id, semantic_id, position, project_id, version_number
	)
	SELECT
		r.override_id, r.ref_type, r.target_id, r.semantic_id, r.position, o.project_id, $2
	FROM weave_override_refs r
	JOIN weave_field_overrides o ON o.id = r.override_id
	WHERE o.project_id = $1`,
	`INSERT INTO weave_adoptions_archive (
		project_id, context_entity_type, context_entity_id, entity_type,
		source_project_id, source_entity_id, source_version, adopted_at, created_by_id, version_number
	)
	SELECT
		project_id, context_entity_type, context_entity_id, entity_type,
		source_project_id, source_entity_id, source_version, adopted_at, created_by_id, $2
	FROM weave_adoptions
	WHERE project_id = $1
	ON CONFLICT (
		project_id, context_entity_type, context_entity_id, entity_type,
		source_project_id, source_entity_id, source_version, version_number
	) DO NOTHING`,
	`INSERT INTO weave_entity_forks_archive (
		project_id, entity_type, fork_entity_id, source_project_id,
		source_entity_id, source_version, forked_at, created_by_id, version_number
	)
	SELECT
		project_id, entity_type, fork_entity_id, source_project_id,
		source_entity_id, source_version, forked_at, created_by_id, $2
	FROM weave_entity_forks
	WHERE project_id = $1
	ON CONFLICT (project_id, entity_type, fork_entity_id, version_number) DO NOTHING`,
}
