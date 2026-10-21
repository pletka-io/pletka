package collection

import (
	"context"
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

	// CategoriesUsed lists every category this scope recorded for the
	// project, not only the placed ones — the live rule, preserved. A
	// placements-derived answer empties the filter dropdown on a project
	// whose collections carry no default category.
	CategoriesUsed(ctx context.Context, projectID string) ([]CollectionCategoryOption, error)

	// CompositionCounts returns per-collection field counts from this scope.
	CompositionCounts(ctx context.Context, projectID string, collectionIDs []string) (map[string]domain.CompositionCounts, error)

	// ListAdoptedByReference returns the foreign collections this scope's
	// own field overrides point at.
	//
	// There is deliberately no ListAdoptedExplicit or ForkOrigins here.
	// Neither is a read of the collection tables: explicit adoptions come
	// from the adoption store and fork origins from the fork store, both of
	// which already answer at a version. They stay on the service, which
	// holds those dependencies, and take a scope there. Mirroring them onto
	// this Reader would mean handing the collection store two foreign stores
	// to proxy.
	ListAdoptedByReference(ctx context.Context, projectID string) ([]*domain.Collection, error)

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

func (r reader) ListAdoptedByReference(ctx context.Context, projectID string) ([]*domain.Collection, error) {
	if r.isRelease() {
		return r.s.listAdoptedByReferenceVersion(ctx, projectID, r.version)
	}
	return r.s.ListReferenceAdopted(ctx, projectID)
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
