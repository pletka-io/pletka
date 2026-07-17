package weave

import (
	"context"
	"errors"
	"fmt"

	"github.com/pletka-io/pletka/pkg/database/dbutil"
	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// projectOntologyVersionStore is the pgx/sqlc-backed implementation of
// domain.ProjectOntologyVersionStore over weave_project_ontology_versions.
type projectOntologyVersionStore struct {
	queries *sqlcgen.Queries
	pool    *pgxpool.Pool
}

// Compile-time interface check.
var _ domain.ProjectOntologyVersionStore = (*projectOntologyVersionStore)(nil)

// newProjectOntologyVersionStore returns a new projectOntologyVersionStore backed by pool.
func newProjectOntologyVersionStore(pool *pgxpool.Pool) *projectOntologyVersionStore {
	return &projectOntologyVersionStore{
		queries: sqlcgen.New(pool),
		pool:    pool,
	}
}

// List returns the project's linked ontology versions, primary first.
func (s *projectOntologyVersionStore) List(ctx context.Context, projectID string) ([]*domain.ProjectOntologyVersion, error) {
	rows, err := s.queries.WeaveListProjectOntologyVersions(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("list project ontology versions: %w", err)
	}
	out := make([]*domain.ProjectOntologyVersion, 0, len(rows))
	for _, r := range rows {
		out = append(out, rowToProjectOntologyVersion(r))
	}
	return out, nil
}

// Get returns a single link, or (nil, nil) when no row matches. Other
// errors propagate. Matches the GetByID convention elsewhere in WeaveStore
// so callers can distinguish "not found" from real failures without
// matching driver-specific error strings.
func (s *projectOntologyVersionStore) Get(ctx context.Context, projectID, versionID string) (*domain.ProjectOntologyVersion, error) {
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
	return rowToProjectOntologyVersion(r), nil
}

// Create inserts a new link row.
func (s *projectOntologyVersionStore) Create(ctx context.Context, link *domain.ProjectOntologyVersion) error {
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

// Update persists is_primary and usage_notes changes. Other columns are
// immutable through this method.
func (s *projectOntologyVersionStore) Update(ctx context.Context, link *domain.ProjectOntologyVersion) error {
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

// Delete removes a link. Callers should run usage guard (CountPathElementUsage)
// before invoking this — the store does not block deletes that leave orphaned
// path elements.
func (s *projectOntologyVersionStore) Delete(ctx context.Context, projectID, versionID string) error {
	if err := s.queries.WeaveDeleteProjectOntologyVersion(ctx, sqlcgen.WeaveDeleteProjectOntologyVersionParams{
		ProjectID:         projectID,
		OntologyVersionID: versionID,
	}); err != nil {
		return fmt.Errorf("delete project ontology version: %w", err)
	}
	return nil
}

// SetPrimary makes the given version the project's primary, unsetting any
// other row in the same project atomically.
func (s *projectOntologyVersionStore) SetPrimary(ctx context.Context, projectID, versionID string) error {
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

// CountPathElementUsage returns how many path elements in the project reference
// the given ontology version.
func (s *projectOntologyVersionStore) CountPathElementUsage(ctx context.Context, projectID, versionID string) (int64, error) {
	n, err := s.queries.WeaveCountPathElementsForVersionInProject(ctx, sqlcgen.WeaveCountPathElementsForVersionInProjectParams{
		OntologyVersionID: &versionID,
		ProjectID:         projectID,
	})
	if err != nil {
		return 0, fmt.Errorf("count path elements: %w", err)
	}
	return n, nil
}

// SamplePathElementFields returns up to limit fields referencing the version,
// suitable for surfacing in 409 payloads. limit is clamped to at most 10 (a SQL-level cap).
func (s *projectOntologyVersionStore) SamplePathElementFields(ctx context.Context, projectID, versionID string, limit int) ([]domain.FieldUsageSample, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := s.queries.WeaveSamplePathElementFieldsForVersion(ctx, sqlcgen.WeaveSamplePathElementFieldsForVersionParams{
		OntologyVersionID: &versionID,
		ProjectID:         projectID,
	})
	if err != nil {
		return nil, fmt.Errorf("sample path element fields: %w", err)
	}
	if len(rows) > limit {
		rows = rows[:limit]
	}
	out := make([]domain.FieldUsageSample, 0, len(rows))
	for _, r := range rows {
		sample := domain.FieldUsageSample{
			ID:         r.ID,
			SemanticID: dbutil.NilToEmpty(r.SemanticID),
			SystemName: dbutil.NilToEmpty(r.SystemName),
			UIName:     unmarshalDomainTranslations(r.UiName),
		}
		out = append(out, sample)
	}
	return out, nil
}

// ListWithCounts returns every linked row alongside the number of path elements
// in the project that reference it. Used by the settings pane to surface a
// usage column on the linked-ontologies list.
func (s *projectOntologyVersionStore) ListWithCounts(ctx context.Context, projectID string) ([]domain.ProjectOntologyVersionWithCounts, error) {
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

// ListGrouped returns the project's OWN linked ontology versions organised
// into groups: one group per linked base version, with its extensions
// (those whose compatible_base_versions includes the base's version_id)
// as items. Inherited groups are composed at the handler layer by
// intersecting ResolvedOntologyVersions(OnlyInherited=true) with this
// structure.
func (s *projectOntologyVersionStore) ListGrouped(ctx context.Context, projectID string) ([]domain.LinkedOntologyGroup, error) {
	withCounts, err := s.ListWithCounts(ctx, projectID)
	if err != nil {
		return nil, err
	}
	if len(withCounts) == 0 {
		return nil, nil
	}

	// Fetch compatibility metadata via raw SQL (ontology_versions is a legacy
	// GORM table not included in the weave sqlc schema).
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

	// Split into bases and extensions.
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

	// Compose groups, bases in order (primary first, then added_at).
	out := make([]domain.LinkedOntologyGroup, 0, len(bases))
	for _, base := range bases {
		m := byVersion[base.Link.OntologyVersionID]
		items := append([]domain.ProjectOntologyVersionWithCounts{base}, extensionsByBase[base.Link.OntologyVersionID]...)
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

// rowToProjectOntologyVersion converts a sqlcgen row to a *domain.ProjectOntologyVersion.
func rowToProjectOntologyVersion(r sqlcgen.WeaveProjectOntologyVersion) *domain.ProjectOntologyVersion {
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
