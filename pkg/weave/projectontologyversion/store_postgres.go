package projectontologyversion

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/pkg/database/dbutil"
	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/domain"
)

// postgresStore is the pgx + sqlc implementation of Store, backed by
// weave_project_ontology_versions and (read-only) ontology_versions for
// compatibility metadata.
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

func (s *postgresStore) List(ctx context.Context, projectID string) ([]*domain.ProjectOntologyVersion, error) {
	rows, err := s.queries.WeaveListProjectOntologyVersions(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("list project ontology versions: %w", err)
	}
	out := make([]*domain.ProjectOntologyVersion, 0, len(rows))
	for _, r := range rows {
		out = append(out, rowToLink(r))
	}
	return out, nil
}

func (s *postgresStore) BundleForProject(ctx context.Context, projectID string) ([]domain.OntologyBundleEntry, error) {
	rows, err := s.queries.WeaveListProjectOntologyBundle(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("list project ontology bundle: %w", err)
	}
	out := make([]domain.OntologyBundleEntry, 0, len(rows))
	for _, r := range rows {
		out = append(out, domain.OntologyBundleEntry{
			Prefix:           r.Prefix,
			Namespace:        r.Namespace,
			Name:             r.OntologyName,
			VersionString:    r.VersionString,
			OriginalFilename: dbutil.NilToEmpty(r.OriginalFilename),
			RDFContent:       dbutil.NilToEmpty(r.RdfContent),
		})
	}
	return out, nil
}

func (s *postgresStore) BundleForVersions(ctx context.Context, versionIDs []string) ([]domain.OntologyBundleEntry, error) {
	if len(versionIDs) == 0 {
		return nil, nil
	}
	rows, err := s.queries.WeaveListOntologyBundleByVersionIDs(ctx, versionIDs)
	if err != nil {
		return nil, fmt.Errorf("list ontology bundle by version ids: %w", err)
	}
	out := make([]domain.OntologyBundleEntry, 0, len(rows))
	for _, r := range rows {
		out = append(out, domain.OntologyBundleEntry{
			Prefix:           r.Prefix,
			Namespace:        r.Namespace,
			Name:             r.OntologyName,
			VersionString:    r.VersionString,
			OriginalFilename: dbutil.NilToEmpty(r.OriginalFilename),
			RDFContent:       dbutil.NilToEmpty(r.RdfContent),
		})
	}
	return out, nil
}

func (s *postgresStore) ListVersion(ctx context.Context, projectID, releaseVersion string) ([]*domain.ProjectOntologyVersion, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT project_id, ontology_version_id, added_at, added_by_id, is_primary, usage_notes
		FROM weave_project_ontology_versions_archive
		WHERE project_id = $1 AND version_number = $2
	`, projectID, releaseVersion)
	if err != nil {
		return nil, fmt.Errorf("list archived project ontology versions: %w", err)
	}
	defer rows.Close()
	out := []*domain.ProjectOntologyVersion{}
	for rows.Next() {
		link, err := scanArchivedLink(rows)
		if err != nil {
			return nil, fmt.Errorf("scan archived project ontology version: %w", err)
		}
		out = append(out, link)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate archived project ontology versions: %w", err)
	}
	return out, nil
}

func (s *postgresStore) Get(ctx context.Context, projectID, versionID string) (*domain.ProjectOntologyVersion, error) {
	r, err := s.queries.WeaveGetProjectOntologyVersion(ctx, sqlcgen.WeaveGetProjectOntologyVersionParams{
		ProjectID:         projectID,
		OntologyVersionID: versionID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get project ontology version: %w", err)
	}
	return rowToLink(r), nil
}

func (s *postgresStore) GetVersion(ctx context.Context, projectID, versionID, releaseVersion string) (*domain.ProjectOntologyVersion, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT project_id, ontology_version_id, added_at, added_by_id, is_primary, usage_notes
		FROM weave_project_ontology_versions_archive
		WHERE project_id = $1 AND ontology_version_id = $2 AND version_number = $3
	`, projectID, versionID, releaseVersion)
	link, err := scanArchivedLink(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get archived project ontology version: %w", err)
	}
	return link, nil
}

func (s *postgresStore) Create(ctx context.Context, link *domain.ProjectOntologyVersion) error {
	isPrimary := link.IsPrimary
	usageNotes := link.UsageNotes
	_, err := s.queries.WeaveCreateProjectOntologyVersion(ctx, sqlcgen.WeaveCreateProjectOntologyVersionParams{
		ProjectID:         link.ProjectID,
		OntologyVersionID: link.OntologyVersionID,
		AddedByID:         link.AddedByID,
		IsPrimary:         &isPrimary,
		UsageNotes:        &usageNotes,
	})
	if err != nil {
		return fmt.Errorf("create project ontology version: %w", err)
	}
	return nil
}

func (s *postgresStore) Update(ctx context.Context, link *domain.ProjectOntologyVersion) error {
	isPrimary := link.IsPrimary
	usageNotes := link.UsageNotes
	_, err := s.queries.WeaveUpdateProjectOntologyVersion(ctx, sqlcgen.WeaveUpdateProjectOntologyVersionParams{
		ProjectID:         link.ProjectID,
		OntologyVersionID: link.OntologyVersionID,
		IsPrimary:         &isPrimary,
		UsageNotes:        &usageNotes,
	})
	if err != nil {
		return fmt.Errorf("update project ontology version: %w", err)
	}
	return nil
}

func (s *postgresStore) Delete(ctx context.Context, projectID, versionID string) error {
	if err := s.queries.WeaveDeleteProjectOntologyVersion(ctx, sqlcgen.WeaveDeleteProjectOntologyVersionParams{
		ProjectID:         projectID,
		OntologyVersionID: versionID,
	}); err != nil {
		return fmt.Errorf("delete project ontology version: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Lifecycle
// ---------------------------------------------------------------------------

func (s *postgresStore) SetPrimary(ctx context.Context, projectID, versionID string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	q := s.queries.WithTx(tx)
	if err := q.WeaveClearProjectPrimaryOntologies(ctx, projectID); err != nil {
		return fmt.Errorf("clear primaries: %w", err)
	}
	if err := q.WeaveSetProjectPrimaryOntology(ctx, sqlcgen.WeaveSetProjectPrimaryOntologyParams{
		ProjectID:         projectID,
		OntologyVersionID: versionID,
	}); err != nil {
		return fmt.Errorf("set primary: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Usage gates
// ---------------------------------------------------------------------------

// CountPathElementUsage returns the number of distinct entities (fields,
// models, collections) in the project that reference any qname defined
// in the given ontology version — through a field's path_elements or
// through an entity's ontology_scope. Drives the usage badge on the
// project overview's deployed-ontologies card.
//
// Walks path_elements inline rather than reading
// weave_field_ontology_refs because that materialised projection is not
// populated by the field write path — see the comment on
// WeaveOntologyUsageByProject.
func (s *postgresStore) CountPathElementUsage(ctx context.Context, projectID, versionID string) (int64, error) {
	var n int64
	if err := s.pool.QueryRow(ctx, `
		WITH version_qnames AS (
			SELECT qname FROM weave_ontology_classes WHERE ontology_version_id = $2
			UNION
			SELECT qname FROM weave_ontology_properties WHERE ontology_version_id = $2
		), entities_using AS (
			SELECT DISTINCT 'field:' || f.id AS entity_key
			FROM weave_fields f,
				 LATERAL jsonb_array_elements(coalesce(f.path_elements, '[]'::jsonb)) AS elem
			WHERE f.project_id = $1
			  AND coalesce(elem->>'prefix', '') <> ''
			  AND coalesce(elem->>'local_name', '') <> ''
			  AND ((elem->>'prefix') || ':' || (elem->>'local_name')) IN (SELECT qname FROM version_qnames)
			UNION
			SELECT DISTINCT 'field:' || id
			FROM weave_fields
			WHERE project_id = $1
			  AND ontology_scope ? 'prefix' AND ontology_scope ? 'local_name'
			  AND ((ontology_scope->>'prefix') || ':' || (ontology_scope->>'local_name')) IN (SELECT qname FROM version_qnames)
			UNION
			SELECT DISTINCT 'model:' || id
			FROM weave_models
			WHERE project_id = $1
			  AND ontology_scope ? 'prefix' AND ontology_scope ? 'local_name'
			  AND ((ontology_scope->>'prefix') || ':' || (ontology_scope->>'local_name')) IN (SELECT qname FROM version_qnames)
			UNION
			SELECT DISTINCT 'collection:' || id
			FROM weave_collections
			WHERE project_id = $1
			  AND ontology_scope ? 'prefix' AND ontology_scope ? 'local_name'
			  AND ((ontology_scope->>'prefix') || ':' || (ontology_scope->>'local_name')) IN (SELECT qname FROM version_qnames)
		)
		SELECT count(*)::bigint FROM entities_using
	`, projectID, versionID).Scan(&n); err != nil {
		return 0, fmt.Errorf("count ontology refs: %w", err)
	}
	return n, nil
}

func (s *postgresStore) CountPathElementUsageVersion(ctx context.Context, projectID, versionID, releaseVersion string) (int64, error) {
	var n int64
	if err := s.pool.QueryRow(ctx, `
		WITH archived_qnames AS (
			SELECT DISTINCT
				f.id AS field_id,
				(elem->>'prefix') || ':' || (elem->>'local_name') AS qname
			FROM weave_fields_archive f,
				 LATERAL jsonb_array_elements(coalesce(f.path_elements, '[]'::jsonb)) AS elem
			WHERE f.project_id = $1
			  AND f.version_number = $3
			  AND coalesce(elem->>'prefix', '') <> ''
			  AND coalesce(elem->>'local_name', '') <> ''
		)
		SELECT count(DISTINCT q.field_id)::bigint
		FROM archived_qnames q
		WHERE EXISTS (
		    SELECT 1 FROM weave_ontology_classes c
		    WHERE c.ontology_version_id = $2
		      AND c.qname = q.qname
		)
		OR EXISTS (
		    SELECT 1 FROM weave_ontology_properties p
		    WHERE p.ontology_version_id = $2
		      AND p.qname = q.qname
		)
	`, projectID, versionID, releaseVersion).Scan(&n); err != nil {
		return 0, fmt.Errorf("count archived path elements: %w", err)
	}
	return n, nil
}

func (s *postgresStore) SamplePathElementFields(ctx context.Context, projectID, versionID string, limit int) ([]domain.FieldUsageSample, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := s.pool.Query(ctx, `
		SELECT DISTINCT f.id, f.semantic_id, f.system_name, f.ui_name
		FROM weave_field_ontology_refs r
		JOIN weave_fields f
		  ON f.id = r.field_id
		 AND f.project_id = $1
		WHERE (
		    EXISTS (
		      SELECT 1 FROM weave_ontology_classes c
		      WHERE c.ontology_version_id = $2
		        AND c.qname = r.qname
		    )
		    OR EXISTS (
		      SELECT 1 FROM weave_ontology_properties p
		      WHERE p.ontology_version_id = $2
		        AND p.qname = r.qname
		    )
		)
		ORDER BY f.id ASC
		LIMIT $3
	`, projectID, versionID, limit)
	if err != nil {
		return nil, fmt.Errorf("sample path element fields: %w", err)
	}
	defer rows.Close()
	out := []domain.FieldUsageSample{}
	for rows.Next() {
		var (
			sample     domain.FieldUsageSample
			semanticID *string
			systemName *string
			uiName     []byte
		)
		if err := rows.Scan(&sample.ID, &semanticID, &systemName, &uiName); err != nil {
			return nil, fmt.Errorf("scan path element sample: %w", err)
		}
		sample.SemanticID = dbutil.NilToEmpty(semanticID)
		sample.SystemName = dbutil.NilToEmpty(systemName)
		sample.UIName = unmarshalTranslations(uiName)
		out = append(out, domain.FieldUsageSample{
			ID:         sample.ID,
			SemanticID: sample.SemanticID,
			SystemName: sample.SystemName,
			UIName:     sample.UIName,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate path element samples: %w", err)
	}
	return out, nil
}

func (s *postgresStore) SamplePathElementFieldsVersion(ctx context.Context, projectID, versionID, releaseVersion string, limit int) ([]domain.FieldUsageSample, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := s.pool.Query(ctx, `
		SELECT DISTINCT
			f.id, f.semantic_id, f.system_name, f.ui_name
		FROM weave_fields_archive f,
			 LATERAL jsonb_array_elements(coalesce(f.path_elements, '[]'::jsonb)) AS elem
		WHERE f.project_id = $1
		  AND f.version_number = $3
		  AND coalesce(elem->>'prefix', '') <> ''
		  AND coalesce(elem->>'local_name', '') <> ''
		  AND (
		    EXISTS (
		      SELECT 1 FROM weave_ontology_classes c
		      WHERE c.ontology_version_id = $2
		        AND c.qname = (elem->>'prefix') || ':' || (elem->>'local_name')
		    )
		    OR EXISTS (
		      SELECT 1 FROM weave_ontology_properties p
		      WHERE p.ontology_version_id = $2
		        AND p.qname = (elem->>'prefix') || ':' || (elem->>'local_name')
		    )
		  )
		LIMIT $4
	`, projectID, versionID, releaseVersion, limit)
	if err != nil {
		return nil, fmt.Errorf("sample archived path element fields: %w", err)
	}
	defer rows.Close()
	out := []domain.FieldUsageSample{}
	for rows.Next() {
		var (
			sample     domain.FieldUsageSample
			semanticID *string
			systemName *string
			uiName     []byte
		)
		if err := rows.Scan(&sample.ID, &semanticID, &systemName, &uiName); err != nil {
			return nil, fmt.Errorf("scan archived path element sample: %w", err)
		}
		sample.SemanticID = dbutil.NilToEmpty(semanticID)
		sample.SystemName = dbutil.NilToEmpty(systemName)
		sample.UIName = unmarshalTranslations(uiName)
		out = append(out, sample)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate archived path element samples: %w", err)
	}
	return out, nil
}

func (s *postgresStore) OntologyUsage(ctx context.Context, projectID, versionID string) (int, int, error) {
	row, err := s.queries.WeaveOntologyUsageByProject(ctx, sqlcgen.WeaveOntologyUsageByProjectParams{
		ProjectID:         projectID,
		OntologyVersionID: versionID,
	})
	if err != nil {
		return 0, 0, fmt.Errorf("ontology usage: %w", err)
	}
	return int(row.ClassesUsed), int(row.PropertiesUsed), nil
}

func (s *postgresStore) OntologyUsageVersion(ctx context.Context, projectID, versionID, releaseVersion string) (int, int, error) {
	var classesUsed, propertiesUsed int64
	if err := s.pool.QueryRow(ctx, `
		WITH path_steps AS (
			SELECT elem->>'prefix' AS prefix,
				   elem->>'local_name' AS local_name
			FROM weave_fields_archive f,
				 LATERAL jsonb_array_elements(coalesce(f.path_elements, '[]'::jsonb)) AS elem
			WHERE f.project_id = $1
			  AND f.version_number = $3
			  AND coalesce(elem->>'prefix', '') <> ''
			  AND coalesce(elem->>'local_name', '') <> ''
		), project_qnames AS (
			SELECT (prefix || ':' || local_name) AS qname FROM path_steps
			UNION
			SELECT (ontology_scope->>'prefix') || ':' || (ontology_scope->>'local_name') AS qname
			FROM weave_models_archive
			WHERE project_id = $1
			  AND version_number = $3
			  AND ontology_scope ? 'prefix' AND ontology_scope ? 'local_name'
			  AND coalesce(ontology_scope->>'prefix', '') <> ''
			  AND coalesce(ontology_scope->>'local_name', '') <> ''
			UNION
			SELECT (ontology_scope->>'prefix') || ':' || (ontology_scope->>'local_name') AS qname
			FROM weave_collections_archive
			WHERE project_id = $1
			  AND version_number = $3
			  AND ontology_scope ? 'prefix' AND ontology_scope ? 'local_name'
			  AND coalesce(ontology_scope->>'prefix', '') <> ''
			  AND coalesce(ontology_scope->>'local_name', '') <> ''
			UNION
			SELECT (ontology_scope->>'prefix') || ':' || (ontology_scope->>'local_name') AS qname
			FROM weave_fields_archive
			WHERE project_id = $1
			  AND version_number = $3
			  AND ontology_scope ? 'prefix' AND ontology_scope ? 'local_name'
			  AND coalesce(ontology_scope->>'prefix', '') <> ''
			  AND coalesce(ontology_scope->>'local_name', '') <> ''
		)
		SELECT
			(
				SELECT count(DISTINCT c.qname)::bigint
				FROM weave_ontology_classes c
				WHERE c.ontology_version_id = $2
				  AND c.qname IN (SELECT qname FROM project_qnames)
			) AS classes_used,
			(
				SELECT count(DISTINCT p.qname)::bigint
				FROM weave_ontology_properties p
				WHERE p.ontology_version_id = $2
				  AND p.qname IN (SELECT qname FROM project_qnames)
			) AS properties_used
	`, projectID, versionID, releaseVersion).Scan(&classesUsed, &propertiesUsed); err != nil {
		return 0, 0, fmt.Errorf("archived ontology usage: %w", err)
	}
	return int(classesUsed), int(propertiesUsed), nil
}

// ---------------------------------------------------------------------------
// Read-side composition
// ---------------------------------------------------------------------------

func (s *postgresStore) ListWithCounts(ctx context.Context, projectID string) ([]domain.ProjectOntologyVersionWithCounts, error) {
	links, err := s.List(ctx, projectID)
	if err != nil {
		return nil, err
	}
	out := make([]domain.ProjectOntologyVersionWithCounts, 0, len(links))
	for _, link := range links {
		count, err := s.CountPathElementUsage(ctx, projectID, link.OntologyVersionID)
		if err != nil {
			return nil, err
		}
		out = append(out, domain.ProjectOntologyVersionWithCounts{Link: link, UsageCount: count})
	}
	return out, nil
}

func (s *postgresStore) ListGrouped(ctx context.Context, projectID string) ([]domain.LinkedOntologyGroup, error) {
	withCounts, err := s.ListWithCounts(ctx, projectID)
	if err != nil {
		return nil, err
	}
	if len(withCounts) == 0 {
		return nil, nil
	}

	// Fetch compatibility metadata via raw SQL — ontology_versions is a
	// legacy GORM table not yet in the weave sqlc schema.
	type compatMeta struct {
		CompatibleBaseVersions []string
		VersionString          string
		OntologyID             string
	}
	byVersion := make(map[string]compatMeta, len(withCounts))

	versionIDs := make([]string, 0, len(withCounts))
	for _, item := range withCounts {
		versionIDs = append(versionIDs, item.Link.OntologyVersionID)
	}

	rows, err := s.pool.Query(ctx,
		`SELECT id, COALESCE(compatible_base_versions, '{}'), version_string, ontology_id
		 FROM weave_ontology_versions WHERE id = ANY($1)`,
		versionIDs,
	)
	if err != nil {
		return nil, fmt.Errorf("list linked versions with compat: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var id string
		var m compatMeta
		if err := rows.Scan(&id, &m.CompatibleBaseVersions, &m.VersionString, &m.OntologyID); err != nil {
			return nil, fmt.Errorf("scan compat meta: %w", err)
		}
		byVersion[id] = m
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list linked versions with compat rows: %w", err)
	}

	bases := make([]domain.ProjectOntologyVersionWithCounts, 0)
	extensionsByBase := make(map[string][]domain.ProjectOntologyVersionWithCounts)
	for _, item := range withCounts {
		m := byVersion[item.Link.OntologyVersionID]
		if len(m.CompatibleBaseVersions) == 0 {
			bases = append(bases, item)
			continue
		}
		for _, baseVID := range m.CompatibleBaseVersions {
			extensionsByBase[baseVID] = append(extensionsByBase[baseVID], item)
		}
	}

	out := make([]domain.LinkedOntologyGroup, 0, len(bases))
	for _, base := range bases {
		m := byVersion[base.Link.OntologyVersionID]
		// extensionsByBase is keyed by VERSION STRING (e.g. "7.1.3"),
		// not the base's ULID — that's what compatible_base_versions
		// records. Look up by the base's resolved VersionString.
		items := append([]domain.ProjectOntologyVersionWithCounts{base}, extensionsByBase[m.VersionString]...)
		out = append(out, domain.LinkedOntologyGroup{
			BaseVersionID: base.Link.OntologyVersionID,
			BaseLabel:     m.VersionString,
			Primary:       base.Link.IsPrimary,
			Items:         items,
			Source:        "own",
		})
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Row converter + small helpers (slice-private)
// ---------------------------------------------------------------------------

func rowToLink(r sqlcgen.WeaveProjectOntologyVersion) *domain.ProjectOntologyVersion {
	link := &domain.ProjectOntologyVersion{
		ProjectID:         r.ProjectID,
		OntologyVersionID: r.OntologyVersionID,
		AddedAt:           r.AddedAt,
	}
	if r.AddedByID != nil {
		s := *r.AddedByID
		link.AddedByID = &s
	}
	if r.IsPrimary != nil {
		link.IsPrimary = *r.IsPrimary
	}
	if r.UsageNotes != nil {
		link.UsageNotes = *r.UsageNotes
	}
	return link
}

type archivedLinkScanner interface {
	Scan(dest ...any) error
}

func scanArchivedLink(scanner archivedLinkScanner) (*domain.ProjectOntologyVersion, error) {
	var (
		projectID         string
		ontologyVersionID string
		addedAt           time.Time
		addedByID         *string
		isPrimary         *bool
		usageNotes        *string
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
