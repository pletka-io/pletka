package category

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/domain"
)

// Reader is the scope-bound read surface. Every method answers from one scope
// — the draft or one release — fixed when the Reader was obtained. A Reader
// cannot be asked to read "whichever version the request carried", which is
// the ambiguity the split exists to remove.
//
// A release scope answers from the archive and does NOT fall back to the live
// table. A release that archived nothing answers "not here", because draft
// content served under a release tag is the defect, not a convenience.
type Reader interface {
	// GetByID returns the category, or (nil, nil) when this scope does not
	// carry it.
	GetByID(ctx context.Context, projectID, id string) (*domain.Category, error)

	// GetByIdentifier looks up by semantic_id or system_name within the
	// project. Same nil/nil convention as GetByID.
	GetByIdentifier(ctx context.Context, projectID, identifier string) (*domain.Category, error)

	// List returns the categories this scope carries. Under a release that
	// archived none, an empty slice is the right answer.
	List(ctx context.Context, projectID string, opts ...domain.QueryOption) ([]*domain.Category, error)

	// Count counts the same set List would return.
	Count(ctx context.Context, projectID string, opts ...domain.QueryOption) (int64, error)

	// ListWithCounts pairs each category with its usage counts, both from
	// this scope: counts taken live beside an archived category would
	// describe a category the reader is not looking at.
	ListWithCounts(ctx context.Context, projectID string) ([]WithCounts, error)

	// ModelFieldOverrides and CollectionFieldOverrides back the category-usage
	// modal. Under a release they report the usage that release recorded.
	ModelFieldOverrides(ctx context.Context, projectID, categoryID string) ([]domain.OverrideEntry, error)
	CollectionFieldOverrides(ctx context.Context, projectID, semanticID string) ([]domain.OverrideEntry, error)

	// IsInUse answers from this scope's override rows.
	IsInUse(ctx context.Context, projectID, id, semanticID string) (bool, error)
}

// reader binds a scope to the store. An empty version means the draft; any
// other value names a release and selects the archive.
type reader struct {
	s       *postgresStore
	version string
}

func (r reader) isRelease() bool { return r.version != "" }

func (r reader) GetByID(ctx context.Context, projectID, id string) (*domain.Category, error) {
	if r.isRelease() {
		return r.s.GetByIDVersion(ctx, projectID, id, r.version)
	}
	return r.s.GetByID(ctx, projectID, id)
}

func (r reader) GetByIdentifier(ctx context.Context, projectID, identifier string) (*domain.Category, error) {
	if r.isRelease() {
		return r.s.getByIdentifierVersion(ctx, projectID, identifier, r.version)
	}
	return r.s.GetByIdentifier(ctx, projectID, identifier)
}

func (r reader) List(ctx context.Context, projectID string, opts ...domain.QueryOption) ([]*domain.Category, error) {
	if r.isRelease() {
		return r.s.listVersion(ctx, projectID, r.version)
	}
	return r.s.List(ctx, projectID, opts...)
}

func (r reader) Count(ctx context.Context, projectID string, opts ...domain.QueryOption) (int64, error) {
	if r.isRelease() {
		return r.s.countVersion(ctx, projectID, r.version)
	}
	return r.s.Count(ctx, projectID, opts...)
}

func (r reader) ListWithCounts(ctx context.Context, projectID string) ([]WithCounts, error) {
	if r.isRelease() {
		return r.s.ListWithCountsVersion(ctx, projectID, r.version)
	}
	return r.s.ListWithCounts(ctx, projectID)
}

func (r reader) ModelFieldOverrides(ctx context.Context, projectID, categoryID string) ([]domain.OverrideEntry, error) {
	if r.isRelease() {
		return r.s.ModelFieldOverridesVersion(ctx, projectID, categoryID, r.version)
	}
	return r.s.ModelFieldOverrides(ctx, projectID, categoryID)
}

func (r reader) CollectionFieldOverrides(ctx context.Context, projectID, semanticID string) ([]domain.OverrideEntry, error) {
	if r.isRelease() {
		return r.s.CollectionFieldOverridesVersion(ctx, projectID, semanticID, r.version)
	}
	return r.s.CollectionFieldOverrides(ctx, projectID, semanticID)
}

func (r reader) IsInUse(ctx context.Context, projectID, id, semanticID string) (bool, error) {
	if r.isRelease() {
		return r.s.isInUseVersion(ctx, projectID, id, semanticID, r.version)
	}
	return r.s.IsInUse(ctx, projectID, id, semanticID)
}

// At returns the reader for scope, or an error when scope is the invalid zero
// value — which means a caller forgot to name one. Failing here is the point:
// a nil-safe default would reintroduce the silent draft read.
func (s *postgresStore) At(scope auth.ReadScope) (Reader, error) {
	if !scope.Valid() {
		return nil, fmt.Errorf("category: read scope is required; the zero ReadScope means a caller did not name draft or a release")
	}
	if scope.IsRelease() {
		return reader{s: s, version: scope.Version()}, nil
	}
	return reader{s: s}, nil
}

// --- archive reads that had no versioned counterpart yet ---

const archivedCategoryColumns = `
	id, created_at, updated_at, semantic_id, system_name, ui_name, description,
	status, project_id, canonical_order, deprecated, version_number`

func (s *postgresStore) listVersion(ctx context.Context, projectID, version string) ([]*domain.Category, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+archivedCategoryColumns+`
		FROM weave_categories_archive
		WHERE project_id = $1 AND version_number = $2
		ORDER BY canonical_order, semantic_id
	`, projectID, version)
	if err != nil {
		return nil, fmt.Errorf("list archived categories: %w", err)
	}
	defer rows.Close()

	out := make([]*domain.Category, 0)
	for rows.Next() {
		cat, err := scanArchivedCategory(rows)
		if err != nil {
			return nil, fmt.Errorf("scan archived category: %w", err)
		}
		out = append(out, cat)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list archived categories: %w", err)
	}
	return out, nil
}

func (s *postgresStore) countVersion(ctx context.Context, projectID, version string) (int64, error) {
	var n int64
	if err := s.pool.QueryRow(ctx, `
		SELECT count(*) FROM weave_categories_archive
		WHERE project_id = $1 AND version_number = $2
	`, projectID, version).Scan(&n); err != nil {
		return 0, fmt.Errorf("count archived categories: %w", err)
	}
	return n, nil
}

func (s *postgresStore) getByIdentifierVersion(ctx context.Context, projectID, identifier, version string) (*domain.Category, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT `+archivedCategoryColumns+`
		FROM weave_categories_archive
		WHERE project_id = $1 AND version_number = $2
		  AND (semantic_id = $3 OR system_name = $3)
	`, projectID, version, identifier)
	cat, err := scanArchivedCategory(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get archived category by identifier: %w", err)
	}
	return cat, nil
}

// isInUseVersion answers from the release's own override rows. The live
// EXISTS query would report today's usage beside an archived category, which
// is how a pinned view ends up offering a delete button for a category the
// release still uses.
func (s *postgresStore) isInUseVersion(ctx context.Context, projectID, id, semanticID, version string) (bool, error) {
	var inUse bool
	if err := s.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM weave_field_overrides_archive fo
			WHERE fo.project_id = $1 AND fo.version_number = $4
			  AND fo.category_id IN ($2, $3)
		)
	`, projectID, id, semanticID, version).Scan(&inUse); err != nil {
		return false, fmt.Errorf("archived category in use: %w", err)
	}
	return inUse, nil
}
