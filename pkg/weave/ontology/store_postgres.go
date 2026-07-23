package ontology

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/pkg/database/dbutil"
	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/ids"
)

type postgresStore struct {
	queries *sqlcgen.Queries
	pool    *pgxpool.Pool
}

var _ Store = (*postgresStore)(nil)

// NewPostgresStore returns a Store backed by pgx + sqlc against the
// weave_ontology_* tables (created in migration 029).
func NewPostgresStore(pool *pgxpool.Pool) Store {
	return &postgresStore{queries: sqlcgen.New(pool), pool: pool}
}

// ---------------------------------------------------------------------------
// Families
// ---------------------------------------------------------------------------

func (s *postgresStore) GetFamily(ctx context.Context, id string) (*domain.OntologyFamily, error) {
	row, err := s.queries.WeaveGetOntologyFamilyByID(ctx, id)
	if err != nil {
		return nil, wrapErr(err, "get ontology family")
	}
	f := rowToFamily(row)
	return f, nil
}

func (s *postgresStore) GetFamilyBySlug(ctx context.Context, slug string) (*domain.OntologyFamily, error) {
	row, err := s.queries.WeaveGetOntologyFamilyBySlug(ctx, slug)
	if err != nil {
		return nil, wrapErr(err, "get ontology family by slug")
	}
	return rowToFamily(row), nil
}

func (s *postgresStore) ListFamilies(ctx context.Context) ([]*domain.OntologyFamily, error) {
	rows, err := s.queries.WeaveListOntologyFamilies(ctx)
	if err != nil {
		return nil, fmt.Errorf("list ontology families: %w", err)
	}
	out := make([]*domain.OntologyFamily, 0, len(rows))
	for _, r := range rows {
		out = append(out, rowToFamily(r))
	}
	return out, nil
}

func (s *postgresStore) ListRootFamilies(ctx context.Context) ([]*domain.OntologyFamily, error) {
	rows, err := s.queries.WeaveListRootOntologyFamilies(ctx)
	if err != nil {
		return nil, fmt.Errorf("list root ontology families: %w", err)
	}
	out := make([]*domain.OntologyFamily, 0, len(rows))
	for _, r := range rows {
		out = append(out, rowToFamily(r))
	}
	return out, nil
}

func (s *postgresStore) ListChildFamilies(ctx context.Context, parentID string) ([]*domain.OntologyFamily, error) {
	rows, err := s.queries.WeaveListChildOntologyFamilies(ctx, &parentID)
	if err != nil {
		return nil, fmt.Errorf("list child ontology families: %w", err)
	}
	out := make([]*domain.OntologyFamily, 0, len(rows))
	for _, r := range rows {
		out = append(out, rowToFamily(r))
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Ontologies
// ---------------------------------------------------------------------------

func (s *postgresStore) GetOntology(ctx context.Context, id string) (*domain.Ontology, error) {
	row, err := s.queries.WeaveGetOntologyByID(ctx, id)
	if err != nil {
		return nil, wrapErr(err, "get ontology")
	}
	return rowToOntology(row), nil
}

func (s *postgresStore) GetOntologyByPrefix(ctx context.Context, prefix string) (*domain.Ontology, error) {
	row, err := s.queries.WeaveGetOntologyByPrefix(ctx, prefix)
	if err != nil {
		return nil, wrapErr(err, "get ontology by prefix")
	}
	return rowToOntology(row), nil
}

func (s *postgresStore) ListOntologies(ctx context.Context) ([]*domain.Ontology, error) {
	rows, err := s.queries.WeaveListOntologies(ctx)
	if err != nil {
		return nil, fmt.Errorf("list ontologies: %w", err)
	}
	out := make([]*domain.Ontology, 0, len(rows))
	for _, r := range rows {
		out = append(out, rowToOntology(r))
	}
	return out, nil
}

func (s *postgresStore) ListOntologiesByFamily(ctx context.Context, familyID string) ([]*domain.Ontology, error) {
	rows, err := s.queries.WeaveListOntologiesByFamily(ctx, &familyID)
	if err != nil {
		return nil, fmt.Errorf("list ontologies by family: %w", err)
	}
	out := make([]*domain.Ontology, 0, len(rows))
	for _, r := range rows {
		out = append(out, rowToOntology(r))
	}
	return out, nil
}

func (s *postgresStore) ListOntologiesByType(ctx context.Context, ontologyType domain.OntologyType) ([]*domain.Ontology, error) {
	rows, err := s.queries.WeaveListOntologiesByType(ctx, string(ontologyType))
	if err != nil {
		return nil, fmt.Errorf("list ontologies by type: %w", err)
	}
	out := make([]*domain.Ontology, 0, len(rows))
	for _, r := range rows {
		out = append(out, rowToOntology(r))
	}
	return out, nil
}

func (s *postgresStore) ListOntologyExtensions(ctx context.Context, baseOntologyID string) ([]*domain.Ontology, error) {
	rows, err := s.queries.WeaveListOntologyExtensions(ctx, &baseOntologyID)
	if err != nil {
		return nil, fmt.Errorf("list ontology extensions: %w", err)
	}
	out := make([]*domain.Ontology, 0, len(rows))
	for _, r := range rows {
		out = append(out, rowToOntology(r))
	}
	return out, nil
}

func (s *postgresStore) SearchOntologies(ctx context.Context, query string, limit int) ([]*domain.Ontology, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.queries.WeaveSearchOntologies(ctx, sqlcgen.WeaveSearchOntologiesParams{
		Column1: &query,
		Limit:   int32(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("search ontologies: %w", err)
	}
	out := make([]*domain.Ontology, 0, len(rows))
	for _, r := range rows {
		out = append(out, rowToOntology(r))
	}
	return out, nil
}

func (s *postgresStore) NamespaceBindings(ctx context.Context) ([]NamespaceBinding, error) {
	rows, err := s.pool.Query(ctx, `
SELECT prefix, namespace, weight, source, ontology_id
FROM (
    SELECT
        prefix,
        namespace,
        100::bigint AS weight,
        'ontology'::text AS source,
        id AS ontology_id
    FROM weave_ontologies
    WHERE prefix <> '' AND namespace <> ''

    UNION ALL

    SELECT
        prefix,
        namespace,
        weight,
        source,
        ontology_id
    FROM weave_namespace_bindings
    WHERE prefix <> ''
      AND namespace <> ''
      AND (project_id IS NULL OR project_id = '')
) ns
ORDER BY length(namespace) DESC, weight DESC, prefix ASC
`)
	if err != nil {
		return nil, fmt.Errorf("list namespace bindings: %w", err)
	}
	defer rows.Close()

	out := []NamespaceBinding{}
	for rows.Next() {
		var b NamespaceBinding
		if err := rows.Scan(&b.Prefix, &b.Namespace, &b.Weight, &b.Source, &b.OntologyID); err != nil {
			return nil, fmt.Errorf("scan namespace bindings: %w", err)
		}
		out = append(out, b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate namespace bindings: %w", err)
	}
	return out, nil
}

func (s *postgresStore) UpsertNamespaceBindings(ctx context.Context, bindings []NamespaceBinding) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin upsert namespace bindings tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if err := upsertNamespaceBindings(ctx, tx, "", bindings); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit upsert namespace bindings tx: %w", err)
	}
	return nil
}

func (s *postgresStore) DeleteNamespaceBindingsBySource(ctx context.Context, source string) error {
	source = strings.TrimSpace(source)
	if source == "" {
		return fmt.Errorf("delete namespace bindings: source required")
	}
	if _, err := s.pool.Exec(ctx, `
DELETE FROM weave_namespace_bindings
WHERE source = $1
  AND (project_id IS NULL OR project_id = '')
`, source); err != nil {
		return fmt.Errorf("delete namespace bindings from source %s: %w", source, err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Versions
// ---------------------------------------------------------------------------

func (s *postgresStore) GetVersion(ctx context.Context, id string) (*domain.OntologyVersion, error) {
	row, err := s.queries.WeaveGetOntologyVersionByID(ctx, id)
	if err != nil {
		return nil, wrapErr(err, "get ontology version")
	}
	return rowToVersion(row), nil
}

func (s *postgresStore) GetVersionByOntologyAndString(ctx context.Context, ontologyID, versionString string) (*domain.OntologyVersion, error) {
	row, err := s.queries.WeaveGetOntologyVersionByOntologyAndString(ctx, sqlcgen.WeaveGetOntologyVersionByOntologyAndStringParams{
		OntologyID:    ontologyID,
		VersionString: versionString,
	})
	if err != nil {
		return nil, wrapErr(err, "get ontology version by ontology+string")
	}
	return rowToVersion(row), nil
}

func (s *postgresStore) GetActiveVersion(ctx context.Context, ontologyID string) (*domain.OntologyVersion, error) {
	row, err := s.queries.WeaveGetActiveOntologyVersion(ctx, ontologyID)
	if err != nil {
		return nil, wrapErr(err, "get active ontology version")
	}
	return rowToVersion(row), nil
}

func (s *postgresStore) ListVersionsByOntology(ctx context.Context, ontologyID string) ([]*domain.OntologyVersion, error) {
	rows, err := s.queries.WeaveListOntologyVersionsByOntology(ctx, ontologyID)
	if err != nil {
		return nil, fmt.Errorf("list ontology versions: %w", err)
	}
	out := make([]*domain.OntologyVersion, 0, len(rows))
	for _, r := range rows {
		out = append(out, rowToVersion(r))
	}
	return out, nil
}

func (s *postgresStore) VersionUsageCount(ctx context.Context, versionID string) (int64, error) {
	n, err := s.queries.WeaveCountProjectsUsingOntologyVersion(ctx, versionID)
	if err != nil {
		return 0, fmt.Errorf("count projects using version: %w", err)
	}
	return n, nil
}

func (s *postgresStore) ListProjectsUsingVersion(ctx context.Context, versionID string, limit int) ([]VersionProjectUsage, error) {
	if limit <= 0 {
		limit = 12
	}
	rows, err := s.queries.WeaveListProjectsUsingOntologyVersion(ctx, sqlcgen.WeaveListProjectsUsingOntologyVersionParams{
		OntologyVersionID: versionID,
		Limit:             int32(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("list projects using version: %w", err)
	}
	out := make([]VersionProjectUsage, 0, len(rows))
	for _, row := range rows {
		projectName := localizedText(unmarshalTranslations(row.UiName), "en")
		if projectName == "" {
			projectName = derefStr(row.SystemName)
		}
		if projectName == "" {
			projectName = row.ProjectID
		}
		out = append(out, VersionProjectUsage{
			ProjectID:   row.ProjectID,
			ProjectName: projectName,
			AddedAt:     row.AddedAt,
			IsPrimary:   row.IsPrimary != nil && *row.IsPrimary,
		})
	}
	return out, nil
}

func (s *postgresStore) VersionUsageCountsForOntology(ctx context.Context, ontologyID string) (map[string]int64, error) {
	rows, err := s.queries.WeaveOntologyVersionUsageCounts(ctx, ontologyID)
	if err != nil {
		return nil, fmt.Errorf("version usage counts: %w", err)
	}
	out := make(map[string]int64, len(rows))
	for _, r := range rows {
		out[r.VersionID] = r.ProjectCount
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Classes
// ---------------------------------------------------------------------------

func (s *postgresStore) GetClass(ctx context.Context, id string) (*domain.OntologyClass, error) {
	row, err := s.queries.WeaveGetOntologyClassByID(ctx, id)
	if err != nil {
		return nil, wrapErr(err, "get ontology class")
	}
	return rowToClass(row), nil
}

func (s *postgresStore) GetClassByURI(ctx context.Context, versionID, uri string) (*domain.OntologyClass, error) {
	row, err := s.queries.WeaveGetOntologyClassByURI(ctx, sqlcgen.WeaveGetOntologyClassByURIParams{
		OntologyVersionID: versionID,
		Uri:               uri,
	})
	if err != nil {
		return nil, wrapErr(err, "get ontology class by URI")
	}
	return rowToClass(row), nil
}

func (s *postgresStore) GetClassByQname(ctx context.Context, versionID, qname string) (*domain.OntologyClass, error) {
	row, err := s.queries.WeaveGetOntologyClassByQname(ctx, sqlcgen.WeaveGetOntologyClassByQnameParams{
		OntologyVersionID: versionID,
		Qname:             &qname,
	})
	if err != nil {
		return nil, wrapErr(err, "get ontology class by qname")
	}
	return rowToClass(row), nil
}

func (s *postgresStore) ListClassesByVersion(ctx context.Context, versionID string) ([]*domain.OntologyClass, error) {
	rows, err := s.queries.WeaveListOntologyClassesByVersion(ctx, versionID)
	if err != nil {
		return nil, fmt.Errorf("list ontology classes: %w", err)
	}
	out := make([]*domain.OntologyClass, 0, len(rows))
	for _, r := range rows {
		out = append(out, rowToClass(r))
	}
	return out, nil
}

func (s *postgresStore) ListClassesByQnames(ctx context.Context, versionID string, qnames []string) ([]*domain.OntologyClass, error) {
	if len(qnames) == 0 {
		return nil, nil
	}
	rows, err := s.queries.WeaveListOntologyClassesByQnames(ctx, sqlcgen.WeaveListOntologyClassesByQnamesParams{
		OntologyVersionID: versionID,
		Column2:           qnames,
	})
	if err != nil {
		return nil, fmt.Errorf("list ontology classes by qnames: %w", err)
	}
	out := make([]*domain.OntologyClass, 0, len(rows))
	for _, r := range rows {
		out = append(out, rowToClass(r))
	}
	return out, nil
}

func (s *postgresStore) ListSubclassesByQname(ctx context.Context, versionID, parentQname string) ([]*domain.OntologyClass, error) {
	rows, err := s.queries.WeaveListSubclassesByQname(ctx, sqlcgen.WeaveListSubclassesByQnameParams{
		OntologyVersionID: versionID,
		TargetQname:       parentQname,
	})
	if err != nil {
		return nil, fmt.Errorf("list subclasses by qname: %w", err)
	}
	out := make([]*domain.OntologyClass, 0, len(rows))
	for _, r := range rows {
		out = append(out, rowToClass(r))
	}
	return out, nil
}

func (s *postgresStore) SearchClasses(ctx context.Context, versionID, query string, limit int) ([]*domain.OntologyClass, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.queries.WeaveSearchOntologyClasses(ctx, sqlcgen.WeaveSearchOntologyClassesParams{
		OntologyVersionID: versionID,
		Column2:           &query,
		Limit:             int32(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("search ontology classes: %w", err)
	}
	out := make([]*domain.OntologyClass, 0, len(rows))
	for _, r := range rows {
		out = append(out, rowToClass(r))
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Properties
// ---------------------------------------------------------------------------

func (s *postgresStore) GetProperty(ctx context.Context, id string) (*domain.OntologyProperty, error) {
	row, err := s.queries.WeaveGetOntologyPropertyByID(ctx, id)
	if err != nil {
		return nil, wrapErr(err, "get ontology property")
	}
	return rowToProperty(row), nil
}

func (s *postgresStore) GetPropertyByURI(ctx context.Context, versionID, uri string) (*domain.OntologyProperty, error) {
	row, err := s.queries.WeaveGetOntologyPropertyByURI(ctx, sqlcgen.WeaveGetOntologyPropertyByURIParams{
		OntologyVersionID: versionID,
		Uri:               uri,
	})
	if err != nil {
		return nil, wrapErr(err, "get ontology property by URI")
	}
	return rowToProperty(row), nil
}

func (s *postgresStore) GetPropertyByQname(ctx context.Context, versionID, qname string) (*domain.OntologyProperty, error) {
	row, err := s.queries.WeaveGetOntologyPropertyByQname(ctx, sqlcgen.WeaveGetOntologyPropertyByQnameParams{
		OntologyVersionID: versionID,
		Qname:             &qname,
	})
	if err != nil {
		return nil, wrapErr(err, "get ontology property by qname")
	}
	return rowToProperty(row), nil
}

func (s *postgresStore) ListPropertiesByVersion(ctx context.Context, versionID string) ([]*domain.OntologyProperty, error) {
	rows, err := s.queries.WeaveListOntologyPropertiesByVersion(ctx, versionID)
	if err != nil {
		return nil, fmt.Errorf("list ontology properties: %w", err)
	}
	out := make([]*domain.OntologyProperty, 0, len(rows))
	for _, r := range rows {
		out = append(out, rowToProperty(r))
	}
	return out, nil
}

func (s *postgresStore) ListPropertiesByQnames(ctx context.Context, versionID string, qnames []string) ([]*domain.OntologyProperty, error) {
	if len(qnames) == 0 {
		return nil, nil
	}
	rows, err := s.queries.WeaveListOntologyPropertiesByQnames(ctx, sqlcgen.WeaveListOntologyPropertiesByQnamesParams{
		OntologyVersionID: versionID,
		Column2:           qnames,
	})
	if err != nil {
		return nil, fmt.Errorf("list ontology properties by qnames: %w", err)
	}
	out := make([]*domain.OntologyProperty, 0, len(rows))
	for _, r := range rows {
		out = append(out, rowToProperty(r))
	}
	return out, nil
}

func (s *postgresStore) SearchProperties(ctx context.Context, versionID, query string, limit int) ([]*domain.OntologyProperty, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.queries.WeaveSearchOntologyProperties(ctx, sqlcgen.WeaveSearchOntologyPropertiesParams{
		OntologyVersionID: versionID,
		Column2:           &query,
		Limit:             int32(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("search ontology properties: %w", err)
	}
	out := make([]*domain.OntologyProperty, 0, len(rows))
	for _, r := range rows {
		out = append(out, rowToProperty(r))
	}
	return out, nil
}

func (s *postgresStore) PropertiesForDomainQname(ctx context.Context, versionID, qname string) ([]*domain.OntologyProperty, error) {
	rows, err := s.queries.WeavePropertiesForDomainQname(ctx, sqlcgen.WeavePropertiesForDomainQnameParams{
		OntologyVersionID: versionID,
		TargetQname:       qname,
	})
	if err != nil {
		return nil, fmt.Errorf("properties for domain: %w", err)
	}
	out := make([]*domain.OntologyProperty, 0, len(rows))
	for _, r := range rows {
		out = append(out, rowToProperty(r))
	}
	return out, nil
}

func (s *postgresStore) PropertiesForRangeQname(ctx context.Context, versionID, qname string) ([]*domain.OntologyProperty, error) {
	rows, err := s.queries.WeavePropertiesForRangeQname(ctx, sqlcgen.WeavePropertiesForRangeQnameParams{
		OntologyVersionID: versionID,
		TargetQname:       qname,
	})
	if err != nil {
		return nil, fmt.Errorf("properties for range: %w", err)
	}
	out := make([]*domain.OntologyProperty, 0, len(rows))
	for _, r := range rows {
		out = append(out, rowToProperty(r))
	}
	return out, nil
}

func (s *postgresStore) GetInverseProperty(ctx context.Context, versionID, propertyID string) (*domain.OntologyProperty, error) {
	row, err := s.queries.WeaveGetInverseOntologyProperty(ctx, sqlcgen.WeaveGetInverseOntologyPropertyParams{
		OntologyVersionID: versionID,
		SourceID:          propertyID,
	})
	if err != nil {
		return nil, wrapErr(err, "get inverse property")
	}
	return rowToProperty(row), nil
}

// ---------------------------------------------------------------------------
// Relations (read)
// ---------------------------------------------------------------------------

func (s *postgresStore) ListRelationsForSource(ctx context.Context, sourceID, sourceKind string) ([]*domain.OntologyRelation, error) {
	rows, err := s.queries.WeaveListOntologyRelationsForSource(ctx, sqlcgen.WeaveListOntologyRelationsForSourceParams{
		SourceID:   sourceID,
		SourceKind: sourceKind,
	})
	if err != nil {
		return nil, fmt.Errorf("list relations for source: %w", err)
	}
	return relationRowsToDomain(rows), nil
}

func (s *postgresStore) ListRelationsForSourceByType(ctx context.Context, sourceID, sourceKind, relType string) ([]*domain.OntologyRelation, error) {
	rows, err := s.queries.WeaveListOntologyRelationsForSourceByType(ctx, sqlcgen.WeaveListOntologyRelationsForSourceByTypeParams{
		SourceID:   sourceID,
		SourceKind: sourceKind,
		RelType:    relType,
	})
	if err != nil {
		return nil, fmt.Errorf("list relations for source by type: %w", err)
	}
	return relationRowsToDomain(rows), nil
}

func (s *postgresStore) ListRelationsForTarget(ctx context.Context, targetQname, relType string) ([]*domain.OntologyRelation, error) {
	rows, err := s.queries.WeaveListOntologyRelationsForTarget(ctx, sqlcgen.WeaveListOntologyRelationsForTargetParams{
		TargetQname: targetQname,
		RelType:     relType,
	})
	if err != nil {
		return nil, fmt.Errorf("list relations for target: %w", err)
	}
	return relationRowsToDomain(rows), nil
}

// ---------------------------------------------------------------------------
// Bulk reads (by version-id set)
// ---------------------------------------------------------------------------

// ListClassesByVersions returns all classes whose ontology_version_id is in
// versionIDs, ordered by version then qname.
func (s *postgresStore) ListClassesByVersions(ctx context.Context, versionIDs []string) ([]*domain.OntologyClass, error) {
	rows, err := s.queries.WeaveListClassesByVersions(ctx, versionIDs)
	if err != nil {
		return nil, fmt.Errorf("list classes by versions: %w", err)
	}
	out := make([]*domain.OntologyClass, 0, len(rows))
	for _, r := range rows {
		out = append(out, rowToClass(r))
	}
	return out, nil
}

// ListPropertiesByVersions returns all properties whose ontology_version_id is
// in versionIDs, ordered by version then qname.
func (s *postgresStore) ListPropertiesByVersions(ctx context.Context, versionIDs []string) ([]*domain.OntologyProperty, error) {
	rows, err := s.queries.WeaveListPropertiesByVersions(ctx, versionIDs)
	if err != nil {
		return nil, fmt.Errorf("list properties by versions: %w", err)
	}
	out := make([]*domain.OntologyProperty, 0, len(rows))
	for _, r := range rows {
		out = append(out, rowToProperty(r))
	}
	return out, nil
}

// ListRelationsByVersionsAndTypes returns relations of the requested types for
// sources belonging to the supplied version IDs. Each row carries the source
// qname so callers can wire edges without a follow-up lookup.
func (s *postgresStore) ListRelationsByVersionsAndTypes(ctx context.Context, versionIDs, relTypes []string) ([]*domain.OntologyRelationWithSource, error) {
	rows, err := s.queries.WeaveListRelationsByVersionsAndTypes(ctx, sqlcgen.WeaveListRelationsByVersionsAndTypesParams{
		Column1: versionIDs,
		Column2: relTypes,
	})
	if err != nil {
		return nil, fmt.Errorf("list relations by versions and types: %w", err)
	}
	out := make([]*domain.OntologyRelationWithSource, 0, len(rows))
	for _, r := range rows {
		out = append(out, &domain.OntologyRelationWithSource{
			OntologyRelation: domain.OntologyRelation{
				SourceID:    r.SourceID,
				SourceKind:  r.SourceKind,
				RelType:     r.RelType,
				TargetQname: r.TargetQname,
				Position:    int(r.Position),
			},
			SourceQname: derefStr(r.SourceQname),
		})
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Field ontology refs (read)
// ---------------------------------------------------------------------------

func (s *postgresStore) ListFieldRefs(ctx context.Context, fieldID string) ([]*domain.FieldOntologyRef, error) {
	rows, err := s.queries.WeaveListFieldOntologyRefs(ctx, fieldID)
	if err != nil {
		return nil, fmt.Errorf("list field refs: %w", err)
	}
	out := make([]*domain.FieldOntologyRef, 0, len(rows))
	for _, r := range rows {
		out = append(out, rowToFieldRef(r))
	}
	return out, nil
}

func (s *postgresStore) FindFieldsUsingQname(ctx context.Context, projectID, qname string, limit int) ([]string, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.queries.WeaveFindFieldsUsingQname(ctx, sqlcgen.WeaveFindFieldsUsingQnameParams{
		ProjectID: projectID,
		Qname:     &qname,
		Limit:     int32(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("find fields using qname: %w", err)
	}
	return rows, nil
}

func (s *postgresStore) CountFieldsUsingQname(ctx context.Context, projectID, qname string) (int64, error) {
	n, err := s.queries.WeaveCountFieldsUsingQname(ctx, sqlcgen.WeaveCountFieldsUsingQnameParams{
		ProjectID: projectID,
		Qname:     &qname,
	})
	if err != nil {
		return 0, fmt.Errorf("count fields using qname: %w", err)
	}
	return n, nil
}

func (s *postgresStore) CountFieldsUsingVersion(ctx context.Context, projectID, versionID string) (int64, error) {
	n, err := s.queries.WeaveCountFieldsUsingOntologyVersion(ctx, sqlcgen.WeaveCountFieldsUsingOntologyVersionParams{
		ProjectID:         projectID,
		OntologyVersionID: versionID,
	})
	if err != nil {
		return 0, fmt.Errorf("count fields using version: %w", err)
	}
	return n, nil
}

func (s *postgresStore) SampleFieldsUsingVersion(ctx context.Context, projectID, versionID string, limit int) ([]FieldVersionRef, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := s.queries.WeaveSampleFieldsUsingOntologyVersion(ctx, sqlcgen.WeaveSampleFieldsUsingOntologyVersionParams{
		ProjectID:         projectID,
		OntologyVersionID: versionID,
		Limit:             int32(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("sample fields using version: %w", err)
	}
	out := make([]FieldVersionRef, 0, len(rows))
	for _, r := range rows {
		out = append(out, FieldVersionRef{FieldID: r.FieldID, Qname: derefStr(r.Qname)})
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Row converters
// ---------------------------------------------------------------------------

func rowToFamily(row sqlcgen.WeaveOntologyFamily) *domain.OntologyFamily {
	return &domain.OntologyFamily{
		ID:             row.ID,
		Slug:           row.Slug,
		Name:           row.Name,
		Description:    unmarshalTranslations(row.Description),
		ParentFamilyID: row.ParentFamilyID,
		HomepageURL:    derefStr(row.HomepageUrl),
		Icon:           derefStr(row.Icon),
		DisplayOrder:   row.DisplayOrder,
		CreatedAt:      row.CreatedAt,
		UpdatedAt:      row.UpdatedAt,
	}
}

func rowToOntology(row sqlcgen.WeaveOntology) *domain.Ontology {
	return &domain.Ontology{
		ID:                row.ID,
		Prefix:            row.Prefix,
		Namespace:         row.Namespace,
		Name:              row.Name,
		Description:       unmarshalTranslations(row.Description),
		FamilyID:          row.FamilyID,
		OntologyType:      domain.OntologyType(row.OntologyType),
		ExtendsOntologyID: row.ExtendsOntologyID,
		HomepageURL:       derefStr(row.HomepageUrl),
		SourceURL:         derefStr(row.SourceUrl),
		CreatedByID:       derefStr(row.CreatedByID),
		CreatedAt:         row.CreatedAt,
		UpdatedAt:         row.UpdatedAt,
	}
}

func rowToVersion(row sqlcgen.WeaveOntologyVersion) *domain.OntologyVersion {
	return &domain.OntologyVersion{
		ID:                     row.ID,
		OntologyID:             row.OntologyID,
		VersionString:          row.VersionString,
		IsActive:               row.IsActive,
		CompatibleBaseVersions: row.CompatibleBaseVersions,
		RDFContent:             derefStr(row.RdfContent),
		ParsedAt:               dbutil.TimestamptzToTimePtr(row.ParsedAt),
		OriginalFilename:       derefStr(row.OriginalFilename),
		FileSize:               derefInt64(row.FileSize),
		FileMD5:                derefStr(row.FileMd5),
		OntologyURI:            derefStr(row.OntologyUri),
		VersionIRI:             derefStr(row.VersionIri),
		VersionInfo:            unmarshalTranslations(row.VersionInfo),
		ImportedOntologies:     row.ImportedOntologies,
		OntologyLabel:          unmarshalTranslations(row.OntologyLabel),
		OntologyComment:        unmarshalTranslations(row.OntologyComment),
		OntologyMetadata:       json.RawMessage(row.OntologyMetadata),
		ClassCount:             row.ClassCount,
		PropertyCount:          row.PropertyCount,
		CreatedAt:              row.CreatedAt,
		UpdatedAt:              row.UpdatedAt,
	}
}

func rowToClass(row sqlcgen.WeaveOntologyClass) *domain.OntologyClass {
	return &domain.OntologyClass{
		ID:                row.ID,
		OntologyVersionID: row.OntologyVersionID,
		Prefix:            row.Prefix,
		LocalName:         row.LocalName,
		URI:               row.Uri,
		Qname:             derefStr(row.Qname),
		Label:             unmarshalTranslations(row.Label),
		Comment:           unmarshalTranslations(row.Comment),
		CreatedAt:         row.CreatedAt,
		UpdatedAt:         row.UpdatedAt,
	}
}

func rowToProperty(row sqlcgen.WeaveOntologyProperty) *domain.OntologyProperty {
	return &domain.OntologyProperty{
		ID:                  row.ID,
		OntologyVersionID:   row.OntologyVersionID,
		Prefix:              row.Prefix,
		LocalName:           row.LocalName,
		URI:                 row.Uri,
		Qname:               derefStr(row.Qname),
		Label:               unmarshalTranslations(row.Label),
		Comment:             unmarshalTranslations(row.Comment),
		PropertyType:        derefStr(row.PropertyType),
		IsFunctional:        row.IsFunctional,
		IsInverseFunctional: row.IsInverseFunctional,
		IsTransitive:        row.IsTransitive,
		IsSymmetric:         row.IsSymmetric,
		IsAsymmetric:        row.IsAsymmetric,
		IsReflexive:         row.IsReflexive,
		IsIrreflexive:       row.IsIrreflexive,
		InversePropertyURI:  derefStr(row.InversePropertyUri),
		CreatedAt:           row.CreatedAt,
		UpdatedAt:           row.UpdatedAt,
	}
}

func rowToFieldRef(row sqlcgen.WeaveFieldOntologyRef) *domain.FieldOntologyRef {
	return &domain.FieldOntologyRef{
		FieldID:   row.FieldID,
		ProjectID: row.ProjectID,
		Prefix:    row.Prefix,
		LocalName: row.LocalName,
		Qname:     derefStr(row.Qname),
		Position:  int(row.Position),
		RefKind:   row.RefKind,
	}
}

func relationRowsToDomain(rows []sqlcgen.WeaveOntologyRelation) []*domain.OntologyRelation {
	out := make([]*domain.OntologyRelation, 0, len(rows))
	for _, r := range rows {
		out = append(out, &domain.OntologyRelation{
			SourceID:    r.SourceID,
			SourceKind:  r.SourceKind,
			RelType:     r.RelType,
			TargetQname: r.TargetQname,
			Position:    int(r.Position),
		})
	}
	return out
}

// ---------------------------------------------------------------------------
// Family writes
// ---------------------------------------------------------------------------

func (s *postgresStore) CreateFamily(ctx context.Context, in CreateFamilyInput) (*domain.OntologyFamily, error) {
	row, err := s.queries.WeaveCreateOntologyFamily(ctx, sqlcgen.WeaveCreateOntologyFamilyParams{
		ID:             in.ID,
		Slug:           in.Slug,
		Name:           in.Name,
		Description:    marshalTranslations(in.Description),
		ParentFamilyID: in.ParentFamilyID,
		HomepageUrl:    dbutil.EmptyToNil(in.HomepageURL),
		Icon:           dbutil.EmptyToNil(in.Icon),
		DisplayOrder:   in.DisplayOrder,
	})
	if err != nil {
		return nil, fmt.Errorf("create family: %w", err)
	}
	return rowToFamily(row), nil
}

func (s *postgresStore) UpdateFamily(ctx context.Context, id string, in UpdateFamilyInput) (*domain.OntologyFamily, error) {
	row, err := s.queries.WeaveUpdateOntologyFamily(ctx, sqlcgen.WeaveUpdateOntologyFamilyParams{
		ID:             id,
		Slug:           in.Slug,
		Name:           in.Name,
		Description:    marshalTranslations(in.Description),
		ParentFamilyID: in.ParentFamilyID,
		HomepageUrl:    dbutil.EmptyToNil(in.HomepageURL),
		Icon:           dbutil.EmptyToNil(in.Icon),
		DisplayOrder:   in.DisplayOrder,
	})
	if err != nil {
		return nil, fmt.Errorf("update family: %w", err)
	}
	return rowToFamily(row), nil
}

func (s *postgresStore) DeleteFamily(ctx context.Context, id string) error {
	if err := s.queries.WeaveDeleteOntologyFamily(ctx, id); err != nil {
		return fmt.Errorf("delete family: %w", err)
	}
	return nil
}

func (s *postgresStore) CountOntologiesInFamily(ctx context.Context, familyID string) (int64, error) {
	n, err := s.queries.WeaveCountOntologiesInFamily(ctx, &familyID)
	if err != nil {
		return 0, fmt.Errorf("count ontologies in family: %w", err)
	}
	return n, nil
}

// ---------------------------------------------------------------------------
// Ontology writes
// ---------------------------------------------------------------------------

func (s *postgresStore) CreateOntology(ctx context.Context, in CreateOntologyInput) (*domain.Ontology, error) {
	row, err := s.queries.WeaveCreateOntology(ctx, sqlcgen.WeaveCreateOntologyParams{
		ID:                in.ID,
		Prefix:            in.Prefix,
		Namespace:         in.Namespace,
		Name:              in.Name,
		Description:       marshalTranslations(in.Description),
		FamilyID:          in.FamilyID,
		OntologyType:      string(in.OntologyType),
		ExtendsOntologyID: in.ExtendsOntologyID,
		HomepageUrl:       dbutil.EmptyToNil(in.HomepageURL),
		SourceUrl:         dbutil.EmptyToNil(in.SourceURL),
		CreatedByID:       dbutil.EmptyToNil(in.CreatedByID),
	})
	if err != nil {
		return nil, fmt.Errorf("create ontology: %w", err)
	}
	return rowToOntology(row), nil
}

func (s *postgresStore) UpdateOntology(ctx context.Context, id string, in UpdateOntologyInput) (*domain.Ontology, error) {
	row, err := s.queries.WeaveUpdateOntology(ctx, sqlcgen.WeaveUpdateOntologyParams{
		ID:                id,
		Prefix:            in.Prefix,
		Namespace:         in.Namespace,
		Name:              in.Name,
		Description:       marshalTranslations(in.Description),
		FamilyID:          in.FamilyID,
		OntologyType:      string(in.OntologyType),
		ExtendsOntologyID: in.ExtendsOntologyID,
		HomepageUrl:       dbutil.EmptyToNil(in.HomepageURL),
		SourceUrl:         dbutil.EmptyToNil(in.SourceURL),
	})
	if err != nil {
		return nil, fmt.Errorf("update ontology: %w", err)
	}
	return rowToOntology(row), nil
}

func (s *postgresStore) DeleteOntology(ctx context.Context, id string) error {
	if err := s.queries.WeaveDeleteOntology(ctx, id); err != nil {
		return fmt.Errorf("delete ontology: %w", err)
	}
	return nil
}

func (s *postgresStore) CountOntologyVersions(ctx context.Context, ontologyID string) (int64, error) {
	n, err := s.queries.WeaveCountOntologyVersions(ctx, ontologyID)
	if err != nil {
		return 0, fmt.Errorf("count ontology versions: %w", err)
	}
	return n, nil
}

// ---------------------------------------------------------------------------
// Version writes
// ---------------------------------------------------------------------------

func (s *postgresStore) CreateVersion(ctx context.Context, in CreateVersionInput) (*domain.OntologyVersion, error) {
	return s.createVersionWith(ctx, s.queries, in)
}

func (s *postgresStore) createVersionWith(ctx context.Context, q *sqlcgen.Queries, in CreateVersionInput) (*domain.OntologyVersion, error) {
	row, err := q.WeaveCreateOntologyVersion(ctx, sqlcgen.WeaveCreateOntologyVersionParams{
		ID:                     in.ID,
		OntologyID:             in.OntologyID,
		VersionString:          in.VersionString,
		IsActive:               in.IsActive,
		CompatibleBaseVersions: in.CompatibleBaseVersions,
		RdfContent:             dbutil.EmptyToNil(in.RDFContent),
		ParsedAt:               dbutil.TimePtrToTimestamptz(in.ParsedAt),
		OriginalFilename:       dbutil.EmptyToNil(in.OriginalFilename),
		FileSize:               int64Ptr(in.FileSize),
		FileMd5:                dbutil.EmptyToNil(in.FileMD5),
		OntologyUri:            dbutil.EmptyToNil(in.OntologyURI),
		VersionIri:             dbutil.EmptyToNil(in.VersionIRI),
		VersionInfo:            marshalTranslations(in.VersionInfo),
		ImportedOntologies:     in.ImportedOntologies,
		OntologyLabel:          marshalTranslations(in.OntologyLabel),
		OntologyComment:        marshalTranslations(in.OntologyComment),
		OntologyMetadata:       []byte(in.OntologyMetadata),
		ClassCount:             in.ClassCount,
		PropertyCount:          in.PropertyCount,
	})
	if err != nil {
		return nil, fmt.Errorf("create version: %w", err)
	}
	return rowToVersion(row), nil
}

func (s *postgresStore) UpdateVersionMetadata(ctx context.Context, id string, in UpdateVersionMetadataInput) (*domain.OntologyVersion, error) {
	row, err := s.queries.WeaveUpdateOntologyVersionMetadata(ctx, sqlcgen.WeaveUpdateOntologyVersionMetadataParams{
		ID:                     id,
		VersionString:          in.VersionString,
		CompatibleBaseVersions: in.CompatibleBaseVersions,
		OntologyUri:            dbutil.EmptyToNil(in.OntologyURI),
		VersionIri:             dbutil.EmptyToNil(in.VersionIRI),
		VersionInfo:            marshalTranslations(in.VersionInfo),
		ImportedOntologies:     in.ImportedOntologies,
		OntologyLabel:          marshalTranslations(in.OntologyLabel),
		OntologyComment:        marshalTranslations(in.OntologyComment),
		OntologyMetadata:       []byte(in.OntologyMetadata),
	})
	if err != nil {
		return nil, fmt.Errorf("update version metadata: %w", err)
	}
	return rowToVersion(row), nil
}

func (s *postgresStore) UpdateVersionCounts(ctx context.Context, id string, classCount, propertyCount int64) error {
	if err := s.queries.WeaveUpdateOntologyVersionCounts(ctx, sqlcgen.WeaveUpdateOntologyVersionCountsParams{
		ID:            id,
		ClassCount:    classCount,
		PropertyCount: propertyCount,
	}); err != nil {
		return fmt.Errorf("update version counts: %w", err)
	}
	return nil
}

func (s *postgresStore) SetActiveVersion(ctx context.Context, ontologyID, versionID string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin set active version tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := setActiveOntologyVersion(ctx, tx, ontologyID, versionID); err != nil {
		return fmt.Errorf("set active version: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit set active version tx: %w", err)
	}
	return nil
}

type ontologyVersionExec interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

func setActiveOntologyVersion(ctx context.Context, exec ontologyVersionExec, ontologyID, versionID string) error {
	if _, err := exec.Exec(ctx, `
UPDATE weave_ontology_versions
SET is_active = false,
    updated_at = now()
WHERE ontology_id = $1
  AND is_active = true
`, ontologyID); err != nil {
		return fmt.Errorf("clear active version: %w", err)
	}
	tag, err := exec.Exec(ctx, `
UPDATE weave_ontology_versions
SET is_active = true,
    updated_at = now()
WHERE ontology_id = $1
  AND id = $2
`, ontologyID, versionID)
	if err != nil {
		return fmt.Errorf("activate target version: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (s *postgresStore) DeleteVersion(ctx context.Context, id string) error {
	if err := s.queries.WeaveDeleteOntologyVersion(ctx, id); err != nil {
		return fmt.Errorf("delete version: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// ImportVersion — high-level transactional entry for the importer.
// ---------------------------------------------------------------------------

func (s *postgresStore) ImportVersion(ctx context.Context, in ImportVersionInput) (*domain.OntologyVersion, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin import-version tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	qtx := s.queries.WithTx(tx)

	// Upsert version row. If a version with this ID exists we replace its
	// content (force-import) — first clear the dependent class/property
	// rows + relations rows by FK cascade on DELETE.
	if err := qtx.WeaveDeleteOntologyVersion(ctx, in.Version.ID); err != nil {
		return nil, fmt.Errorf("clear existing version: %w", err)
	}
	version, err := s.createVersionWith(ctx, qtx, in.Version)
	if err != nil {
		return nil, err
	}
	if err := upsertNamespaceBindings(ctx, tx, in.Version.OntologyID, in.NamespaceBindings); err != nil {
		return nil, err
	}

	// Persist companion sources verbatim (the delete above cascaded any
	// previous rows) so self-contained snapshots can vendor them.
	for i, companion := range in.CompanionFiles {
		if err := qtx.WeaveUpsertOntologyVersionCompanion(ctx, sqlcgen.WeaveUpsertOntologyVersionCompanionParams{
			OntologyVersionID: version.ID,
			Filename:          companion.Filename,
			Description:       companion.Description,
			Content:           companion.Content,
			Position:          int32(i),
		}); err != nil {
			return nil, fmt.Errorf("import companion %s: %w", companion.Filename, err)
		}
	}

	// Bulk-insert classes. Versions typically have hundreds-to-thousands
	// of rows; one INSERT per row stays manageable inside a single tx for
	// CRM-scale ontologies (5000 classes + properties takes ~1-2s).
	for _, c := range in.Classes {
		if _, err := qtx.WeaveCreateOntologyClass(ctx, sqlcgen.WeaveCreateOntologyClassParams{
			ID:                c.ID,
			OntologyVersionID: version.ID,
			Prefix:            c.Prefix,
			LocalName:         c.LocalName,
			Uri:               c.URI,
			Label:             marshalTranslations(c.Label),
			Comment:           marshalTranslations(c.Comment),
			NaturalSortKey:    naturalSortKey(c.LocalName),
			SourceModule:      dbutil.EmptyToNil(c.SourceModule),
		}); err != nil {
			return nil, fmt.Errorf("import class %s: %w", c.URI, err)
		}
	}

	// Bulk-insert properties.
	for _, p := range in.Properties {
		if _, err := qtx.WeaveCreateOntologyProperty(ctx, sqlcgen.WeaveCreateOntologyPropertyParams{
			ID:                  p.ID,
			OntologyVersionID:   version.ID,
			Prefix:              p.Prefix,
			LocalName:           p.LocalName,
			Uri:                 p.URI,
			Label:               marshalTranslations(p.Label),
			Comment:             marshalTranslations(p.Comment),
			PropertyType:        dbutil.EmptyToNil(p.PropertyType),
			IsFunctional:        p.IsFunctional,
			IsInverseFunctional: p.IsInverseFunctional,
			IsTransitive:        p.IsTransitive,
			IsSymmetric:         p.IsSymmetric,
			IsAsymmetric:        p.IsAsymmetric,
			IsReflexive:         p.IsReflexive,
			IsIrreflexive:       p.IsIrreflexive,
			InversePropertyUri:  dbutil.EmptyToNil(p.InversePropertyURI),
			NaturalSortKey:      naturalSortKey(p.LocalName),
			SourceModule:        dbutil.EmptyToNil(p.SourceModule),
		}); err != nil {
			return nil, fmt.Errorf("import property %s: %w", p.URI, err)
		}
	}

	// Bulk-insert relations. Source IDs reference the just-inserted
	// classes/properties; the importer derives them via the same id_helpers
	// that produced the class/property IDs.
	for _, r := range in.Relations {
		if err := qtx.WeaveInsertOntologyRelation(ctx, sqlcgen.WeaveInsertOntologyRelationParams{
			SourceID:    r.SourceID,
			SourceKind:  r.SourceKind,
			RelType:     r.RelType,
			TargetQname: r.TargetQname,
			Position:    int32(r.Position),
		}); err != nil {
			return nil, fmt.Errorf("import relation: %w", err)
		}
	}

	// Sync the cached counts on the version row.
	if err := qtx.WeaveUpdateOntologyVersionCounts(ctx, sqlcgen.WeaveUpdateOntologyVersionCountsParams{
		ID:            version.ID,
		ClassCount:    int64(len(in.Classes)),
		PropertyCount: int64(len(in.Properties)),
	}); err != nil {
		return nil, fmt.Errorf("update version counts: %w", err)
	}

	if in.SetActive {
		if err := setActiveOntologyVersion(ctx, tx, version.OntologyID, version.ID); err != nil {
			return nil, fmt.Errorf("set active during import: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit import-version tx: %w", err)
	}

	// Re-read so caller sees updated counts + is_active.
	return s.GetVersion(ctx, version.ID)
}

func upsertNamespaceBindings(ctx context.Context, tx pgx.Tx, defaultOntologyID string, bindings []NamespaceBinding) error {
	for _, binding := range bindings {
		prefix := strings.TrimSpace(binding.Prefix)
		namespace := strings.TrimSpace(binding.Namespace)
		if prefix == "" || namespace == "" {
			continue
		}
		weight := binding.Weight
		if weight == 0 {
			weight = NamespaceBindingWeightRDFDeclared
		}
		source := strings.TrimSpace(binding.Source)
		if source == "" {
			source = NamespaceBindingSourceRDFDeclared
		}
		ontologyID := binding.OntologyID
		if ontologyID == nil && defaultOntologyID != "" {
			id := defaultOntologyID
			ontologyID = &id
		}
		id := ids.GenerateID("namespace-binding:" + prefix + "|" + namespace)
		if _, err := tx.Exec(ctx, `
INSERT INTO weave_namespace_bindings (
    id, prefix, namespace, weight, source, ontology_id, project_id, created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, NULL, now(), now())
ON CONFLICT (prefix, namespace) DO UPDATE
SET weight = EXCLUDED.weight,
    source = EXCLUDED.source,
    ontology_id = COALESCE(weave_namespace_bindings.ontology_id, EXCLUDED.ontology_id),
    project_id = NULL,
    updated_at = now()
WHERE weave_namespace_bindings.weight <= EXCLUDED.weight
`, id, prefix, namespace, weight, source, ontologyID); err != nil {
			return fmt.Errorf("upsert import namespace binding %s=%s: %w", prefix, namespace, err)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Field-ref bulk replace
// ---------------------------------------------------------------------------

func (s *postgresStore) ReplaceFieldRefs(ctx context.Context, fieldID, projectID string, refs []domain.FieldOntologyRef) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin replace-field-refs tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	qtx := s.queries.WithTx(tx)

	if err := qtx.WeaveDeleteFieldOntologyRefs(ctx, fieldID); err != nil {
		return fmt.Errorf("clear field refs: %w", err)
	}
	for _, r := range refs {
		if err := qtx.WeaveInsertFieldOntologyRef(ctx, sqlcgen.WeaveInsertFieldOntologyRefParams{
			FieldID:   fieldID,
			ProjectID: projectID,
			Prefix:    r.Prefix,
			LocalName: r.LocalName,
			Position:  int32(r.Position),
			RefKind:   r.RefKind,
		}); err != nil {
			return fmt.Errorf("insert field ref: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit replace-field-refs tx: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// wrapErr folds pgx.ErrNoRows into nil; otherwise wraps with the op
// label. Caller pattern:
//
//	row, err := s.queries.X(ctx, id)
//	if err != nil {
//	    return nil, wrapErr(err, "get X")
//	}
//	return rowToX(row), nil
//
// Returns (nil, nil) on not-found so the slice service can decide
// whether to surface a 404.
func wrapErr(err error, op string) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	return fmt.Errorf("%s: %w", op, err)
}

func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func derefInt64(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
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

func int64Ptr(i int64) *int64 {
	if i == 0 {
		return nil
	}
	return &i
}
