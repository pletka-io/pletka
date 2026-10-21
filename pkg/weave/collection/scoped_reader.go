package collection

import (
	"context"
	"errors"
	"fmt"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/domain"
)

// Reader is the scope-bound read surface for collections. Every method answers
// from one scope — the draft or one release — fixed when the Reader was
// obtained. A Reader cannot be asked to read "whichever version the request
// carried", which is the ambiguity the split exists to remove.
//
// A release scope answers from the archive and does NOT fall back to the live
// table. A release that archived nothing answers "not here", because draft
// content served under a release tag is the defect, not a convenience.
type Reader interface {
	// Get returns the collection, or (nil, nil) when this scope does not
	// carry it. A collection created after the release was cut is not in
	// that release, so a release reader answers nil.
	Get(ctx context.Context, projectID, id string) (*domain.Collection, error)

	// List returns the collections this scope carries, with the same
	// QueryOption handling on both paths: a filter honoured on the draft
	// and dropped on the archive makes the two disagree for one query.
	List(ctx context.Context, projectID string, opts ...domain.QueryOption) ([]*domain.Collection, int64, error)

	// Usage reports field usage as this scope recorded it.
	Usage(ctx context.Context, projectID, collectionID string) (UsageReport, error)

	// ScopeClasses derives the ontology scope classes from this scope's
	// field paths.
	ScopeClasses(ctx context.Context, projectID string) ([]string, error)

	// CategoriesUsed lists the categories this scope's placements cite,
	// including any deleted since the release was cut.
	CategoriesUsed(ctx context.Context, projectID string) ([]CollectionCategoryOption, error)

	// CompositionCounts returns per-collection field counts from this scope.
	CompositionCounts(ctx context.Context, projectID string, collectionIDs []string) (map[string]domain.CompositionCounts, error)

	// ListAdoptedExplicit and ListAdoptedByReference answer from the same
	// scope as each other. They disagreed before the conversion: one
	// branched on the version and one did not.
	ListAdoptedExplicit(ctx context.Context, projectID string) ([]*domain.Collection, map[string]domain.Origin, error)
	ListAdoptedByReference(ctx context.Context, projectID string) ([]*domain.Collection, error)

	// ForkOrigins reports the ancestors this scope was cut against.
	ForkOrigins(ctx context.Context, projectID string) (map[string]domain.Origin, error)

	// Lookup resolves a collection across projects without a project gate,
	// for internal cross-slice resolution where route auth already ran. It
	// still takes its answer from this Reader's scope, so a draft Lookup is
	// a visible decision at the call site rather than an implicit one.
	Lookup(ctx context.Context, id string) (*domain.Collection, error)
}

// reader binds a scope to the store. An empty version means the draft; any
// other value names a release and selects the archive.
type reader struct {
	s       *postgresStore
	version string
}

func (r reader) isRelease() bool { return r.version != "" }

func (r reader) Get(ctx context.Context, projectID, id string) (*domain.Collection, error) {
	if r.isRelease() {
		return r.s.getByIDVersion(ctx, projectID, id, r.version)
	}
	c, err := r.s.GetByID(ctx, id)
	if err != nil || c == nil {
		return nil, err
	}
	if c.ProjectID != projectID {
		return nil, nil
	}
	return c, nil
}

func (r reader) List(ctx context.Context, projectID string, opts ...domain.QueryOption) ([]*domain.Collection, int64, error) {
	if r.isRelease() {
		return r.s.listVersion(ctx, projectID, r.version, opts...)
	}
	opts = append([]domain.QueryOption{domain.WithProjectID(projectID)}, opts...)
	return r.s.List(ctx, opts...)
}

func (r reader) Usage(ctx context.Context, projectID, collectionID string) (UsageReport, error) {
	if r.isRelease() {
		return r.s.usageVersion(ctx, projectID, collectionID, r.version)
	}
	return r.s.Usage(ctx, projectID, collectionID)
}

func (r reader) ScopeClasses(ctx context.Context, projectID string) ([]string, error) {
	if r.isRelease() {
		return r.s.scopeClassesVersion(ctx, projectID, r.version)
	}
	return r.s.ScopeClasses(ctx, projectID)
}

func (r reader) CategoriesUsed(ctx context.Context, projectID string) ([]CollectionCategoryOption, error) {
	if r.isRelease() {
		return r.s.categoriesUsedVersion(ctx, projectID, r.version)
	}
	return r.s.CategoriesUsed(ctx, projectID)
}

func (r reader) CompositionCounts(ctx context.Context, projectID string, collectionIDs []string) (map[string]domain.CompositionCounts, error) {
	if r.isRelease() {
		return r.s.compositionCountsVersion(ctx, collectionIDs, r.version)
	}
	return r.s.BatchCompositionCounts(ctx, collectionIDs)
}

func (r reader) ListAdoptedExplicit(ctx context.Context, projectID string) ([]*domain.Collection, map[string]domain.Origin, error) {
	if r.s.adoptions == nil {
		return nil, nil, nil
	}
	opts := []domain.QueryOption{
		domain.WithProjectID(projectID),
		domain.WithFilter("context_entity_type", "project"),
		domain.WithFilter("context_entity_id", projectID),
		domain.WithFilter("entity_type", "collection"),
	}
	if r.isRelease() {
		opts = append(opts, domain.WithVersion(r.version))
	}
	adoptions, err := r.s.adoptions.List(ctx, opts...)
	if err != nil {
		return nil, nil, fmt.Errorf("list collection adoptions: %w", err)
	}
	out := make([]*domain.Collection, 0, len(adoptions))
	origins := make(map[string]domain.Origin, len(adoptions))
	seen := map[string]bool{}
	for _, a := range adoptions {
		if seen[a.SourceEntityID] {
			continue
		}
		seen[a.SourceEntityID] = true
		// A dangling receipt (source gone from this scope) is skipped, as
		// the live service does.
		c, err := r.Lookup(ctx, a.SourceEntityID)
		if err != nil || c == nil {
			continue
		}
		out = append(out, c)
		origins[a.SourceEntityID] = domain.AdoptedOrigin(a.SourceProjectID, a.SourceEntityID)
	}
	return out, origins, nil
}

func (r reader) ListAdoptedByReference(ctx context.Context, projectID string) ([]*domain.Collection, error) {
	if r.isRelease() {
		return r.s.listAdoptedByReferenceVersion(ctx, projectID, r.version)
	}
	return r.s.ListReferenceAdopted(ctx, projectID)
}

func (r reader) ForkOrigins(ctx context.Context, projectID string) (map[string]domain.Origin, error) {
	if projectID == "" {
		return map[string]domain.Origin{}, nil
	}
	if r.isRelease() {
		return r.s.forkOriginsVersion(ctx, projectID, r.version)
	}
	if r.s.forks == nil {
		return nil, errors.New("collection: fork store not wired into the store")
	}
	forks, err := r.s.forks.List(ctx,
		domain.WithProjectID(projectID),
		domain.WithFilter("entity_type", "collection"),
	)
	if err != nil {
		return nil, err
	}
	out := make(map[string]domain.Origin, len(forks))
	for _, f := range forks {
		out[f.ForkEntityID] = domain.Origin{
			Kind:            domain.OriginForked,
			SourceProjectID: f.SourceProjectID,
			SourceEntityID:  f.SourceEntityID,
		}
	}
	return out, nil
}

func (r reader) Lookup(ctx context.Context, id string) (*domain.Collection, error) {
	if r.isRelease() {
		return r.s.lookupVersion(ctx, id, r.version)
	}
	return r.s.GetByID(ctx, id)
}

// At returns the reader for scope, or an error when scope is the invalid zero
// value — which means a caller forgot to name one. Failing here is the point:
// a nil-safe default would reintroduce the silent draft read.
func (s *postgresStore) At(scope auth.ReadScope) (Reader, error) {
	if !scope.Valid() {
		return nil, fmt.Errorf("collection: read scope is required; the zero ReadScope means a caller did not name draft or a release")
	}
	if scope.IsRelease() {
		return reader{s: s, version: scope.Version()}, nil
	}
	return reader{s: s}, nil
}
