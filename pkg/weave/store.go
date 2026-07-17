package weave

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	weaveauth "github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/domain"
	weavecollection "github.com/pletka-io/pletka/pkg/weave/collection"
	weavefield "github.com/pletka-io/pletka/pkg/weave/field"
	weavemodel "github.com/pletka-io/pletka/pkg/weave/model"
	weaveoverride "github.com/pletka-io/pletka/pkg/weave/override"
)

// PostgresStore implements domain.WeaveStore backed by PostgreSQL + sqlc.
type PostgresStore struct {
	pool    *pgxpool.Pool
	queries *sqlcgen.Queries
}

// Compile-time interface check.
var _ domain.WeaveStore = (*PostgresStore)(nil)

// NewPostgresStore creates a new PostgresStore backed by the given connection pool.
func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{
		pool:    pool,
		queries: sqlcgen.New(pool),
	}
}

// Queries returns the sqlc queries handle for direct access.
// Used by the search handler. Tech debt: ideally search would be a method
// on WeaveStore interface, but the query shape doesn't fit the sub-store pattern.
func (s *PostgresStore) Queries() *sqlcgen.Queries {
	return s.queries
}

// Pool returns the underlying pgxpool for direct pgx queries (stored functions, etc.).
// Used by the search handler for dynamic path matching.
func (s *PostgresStore) Pool() *pgxpool.Pool {
	return s.pool
}

// WeaveCategories returns a WeaveCategoryStore backed by the weave_categories table.
func (s *PostgresStore) WeaveCategories() domain.WeaveCategoryStore {
	return &weaveCategoryStore{queries: s.queries, pool: s.pool}
}

// WeaveFields returns a WeaveFieldStore backed by the pkg/weave/field
// slice store. Single source of truth — see modelStoreAdapter comment.
func (s *PostgresStore) WeaveFields() domain.WeaveFieldStore {
	return &fieldStoreAdapter{inner: weavefield.NewPostgresStore(s.pool)}
}

type fieldStoreAdapter struct {
	inner weavefield.Store
}

func (a *fieldStoreAdapter) Create(ctx context.Context, f *domain.Field) error {
	return a.inner.Create(ctx, f)
}

func (a *fieldStoreAdapter) GetByID(ctx context.Context, id string) (*domain.Field, error) {
	return a.inner.GetByID(ctx, id)
}

func (a *fieldStoreAdapter) Update(ctx context.Context, f *domain.Field) error {
	return a.inner.Update(ctx, f)
}

func (a *fieldStoreAdapter) Delete(ctx context.Context, id string) error {
	return a.inner.Delete(ctx, id)
}

func (a *fieldStoreAdapter) List(ctx context.Context, opts ...domain.QueryOption) ([]*domain.Field, int64, error) {
	return a.inner.List(ctx, opts...)
}

func (a *fieldStoreAdapter) FindByPathSequence(ctx context.Context, q domain.PathQuery) ([]*domain.Field, error) {
	return a.inner.FindByPathSequence(ctx, q)
}

func (a *fieldStoreAdapter) CountUsage(ctx context.Context, fieldID, projectID string) (domain.FieldUsageCounts, error) {
	return a.inner.CountUsage(ctx, fieldID, projectID)
}

func (a *fieldStoreAdapter) ListUsage(ctx context.Context, fieldID, projectID string) (domain.FieldUsageList, error) {
	return a.inner.ListUsage(ctx, fieldID, projectID)
}

func (a *fieldStoreAdapter) ListReceiptFieldClosure(ctx context.Context, seedID, seedKind string) ([]string, error) {
	return a.inner.ListReceiptFieldClosure(ctx, seedID, seedKind)
}

func (a *fieldStoreAdapter) ListReceiptFieldDirect(ctx context.Context, seedID, seedKind string) ([]string, error) {
	return a.inner.ListReceiptFieldDirect(ctx, seedID, seedKind)
}

func (a *fieldStoreAdapter) ListReferenceAdopted(ctx context.Context, projectID string) ([]*domain.Field, error) {
	return a.inner.ListReferenceAdopted(ctx, projectID)
}

// Vocabularies returns a VocabularyStore backed by the weave_vocabularies table.
func (s *PostgresStore) Vocabularies() domain.VocabularyStore {
	return &vocabularyStore{queries: s.queries}
}

// ConceptLists returns a ConceptListStore backed by the weave_concept_lists table.
func (s *PostgresStore) ConceptLists() domain.ConceptListStore {
	return &conceptListStore{queries: s.queries, pool: s.pool}
}

// Models returns a WeaveModelStore backed by the weave_models table.
// Routes to the pkg/weave/model slice store so there's a single source
// of truth for model persistence — see the cerebrum note on dual
// store consolidation. The adapter is thin (only GetByIdentifier swaps
// argument order to match the legacy interface).
func (s *PostgresStore) Models() domain.WeaveModelStore {
	return &modelStoreAdapter{inner: weavemodel.NewPostgresStore(s.pool)}
}

// modelStoreAdapter wraps the pkg/weave/model slice store to satisfy
// the legacy domain.WeaveModelStore interface. Most methods forward
// unchanged; GetByIdentifier swaps argument order.
type modelStoreAdapter struct {
	inner weavemodel.Store
}

func (a *modelStoreAdapter) Create(ctx context.Context, m *domain.Model) error {
	return a.inner.Create(ctx, m)
}

func (a *modelStoreAdapter) GetByID(ctx context.Context, id string) (*domain.Model, error) {
	return a.inner.GetByID(ctx, id)
}

func (a *modelStoreAdapter) GetByIdentifier(ctx context.Context, identifier, projectID string) (*domain.Model, error) {
	return a.inner.GetByIdentifier(ctx, projectID, identifier)
}

func (a *modelStoreAdapter) Update(ctx context.Context, m *domain.Model) error {
	return a.inner.Update(ctx, m)
}

func (a *modelStoreAdapter) Delete(ctx context.Context, id string) error {
	return a.inner.Delete(ctx, id)
}

func (a *modelStoreAdapter) List(ctx context.Context, opts ...domain.QueryOption) ([]*domain.Model, int64, error) {
	return a.inner.List(ctx, opts...)
}

func (a *modelStoreAdapter) ListOptions(ctx context.Context, projectID string) ([]domain.EntityOption, error) {
	return a.inner.ListOptions(ctx, projectID)
}

func (a *modelStoreAdapter) ListConnectedModelIDs(ctx context.Context, seedIDs []string) ([]string, error) {
	return a.inner.ListConnectedModelIDs(ctx, seedIDs)
}

func (a *modelStoreAdapter) ListReceiptModelClosure(ctx context.Context, seedID, seedKind string) ([]string, error) {
	return a.inner.ListReceiptModelClosure(ctx, seedID, seedKind)
}

func (a *modelStoreAdapter) ListReceiptModelDirect(ctx context.Context, seedID, seedKind string) ([]string, error) {
	return a.inner.ListReceiptModelDirect(ctx, seedID, seedKind)
}

func (a *modelStoreAdapter) ListReferenceAdopted(ctx context.Context, projectID string) ([]*domain.Model, error) {
	return a.inner.ListReferenceAdopted(ctx, projectID)
}

func (a *modelStoreAdapter) ListUsage(ctx context.Context, modelID string) (domain.FieldUsageList, error) {
	return a.inner.ListUsage(ctx, modelID)
}

// ScopeClasses isn't on the legacy domain.WeaveModelStore interface
// but the slice store needs callers (e.g. the filter dropdown
// endpoint) able to reach it through the aggregate. Exposed as a
// concrete method on the adapter; type-assert when needed.
func (a *modelStoreAdapter) ScopeClasses(ctx context.Context, projectID string) ([]string, error) {
	return a.inner.ScopeClasses(ctx, projectID)
}

// Collections returns a WeaveCollectionStore backed by the
// pkg/weave/collection slice store (single source of truth — see
// modelStoreAdapter comment for the broader consolidation rationale).
func (s *PostgresStore) Collections() domain.WeaveCollectionStore {
	return &collectionStoreAdapter{inner: weavecollection.NewPostgresStore(s.pool)}
}

type collectionStoreAdapter struct {
	inner weavecollection.Store
}

func (a *collectionStoreAdapter) Create(ctx context.Context, c *domain.Collection) error {
	return a.inner.Create(ctx, c)
}

func (a *collectionStoreAdapter) GetByID(ctx context.Context, id string) (*domain.Collection, error) {
	return a.inner.GetByID(ctx, id)
}

func (a *collectionStoreAdapter) GetByIdentifier(ctx context.Context, identifier, projectID string) (*domain.Collection, error) {
	return a.inner.GetByIdentifier(ctx, projectID, identifier)
}

func (a *collectionStoreAdapter) Update(ctx context.Context, c *domain.Collection) error {
	return a.inner.Update(ctx, c)
}

func (a *collectionStoreAdapter) Delete(ctx context.Context, id string) error {
	return a.inner.Delete(ctx, id)
}

func (a *collectionStoreAdapter) List(ctx context.Context, opts ...domain.QueryOption) ([]*domain.Collection, int64, error) {
	return a.inner.List(ctx, opts...)
}

func (a *collectionStoreAdapter) ListOptions(ctx context.Context, projectID string) ([]domain.EntityOption, error) {
	return a.inner.ListOptions(ctx, projectID)
}

func (a *collectionStoreAdapter) ListUsage(ctx context.Context, collectionID, projectID string) ([]domain.FieldUsageRef, error) {
	return a.inner.ListUsage(ctx, collectionID, projectID)
}

func (a *collectionStoreAdapter) ListReceiptCollectionClosure(ctx context.Context, seedID, seedKind string) ([]string, error) {
	return a.inner.ListReceiptCollectionClosure(ctx, seedID, seedKind)
}

func (a *collectionStoreAdapter) ListReceiptCollectionDirect(ctx context.Context, seedID, seedKind string) ([]string, error) {
	return a.inner.ListReceiptCollectionDirect(ctx, seedID, seedKind)
}

func (a *collectionStoreAdapter) ListReferenceAdopted(ctx context.Context, projectID string) ([]*domain.Collection, error) {
	return a.inner.ListReferenceAdopted(ctx, projectID)
}

// Overrides returns an OverrideStore backed by the pkg/weave/override
// slice store. Same interface as domain.OverrideStore — no method
// adaptation needed.
func (s *PostgresStore) Overrides() domain.OverrideStore {
	return weaveoverride.NewPostgresStore(s.pool)
}

// Adoptions returns an AdoptionStore backed by weave_adoptions.
func (s *PostgresStore) Adoptions() domain.AdoptionStore {
	return &adoptionStore{pool: s.pool}
}

// Forks returns a ForkStore backed by weave_entity_forks.
func (s *PostgresStore) Forks() domain.ForkStore {
	return &forkStore{pool: s.pool}
}

// Projects returns a WeaveProjectStore backed by the weave_projects table.
func (s *PostgresStore) Projects() domain.WeaveProjectStore {
	return &weaveProjectStore{queries: s.queries, pool: s.pool}
}

// ProjectInheritances returns a ProjectInheritanceStore backed by weave_project_inheritance.
func (s *PostgresStore) ProjectInheritances() domain.ProjectInheritanceStore {
	return &projectInheritanceStore{pool: s.pool}
}

// Memberships returns a MembershipStore backed by weave_memberships.
func (s *PostgresStore) Memberships() domain.MembershipStore {
	return &membershipStore{queries: s.queries, pool: s.pool}
}

// Auth returns an AuthStore backed by weave_auth.
func (s *PostgresStore) Auth() domain.AuthStore {
	return &authStore{queries: s.queries, pool: s.pool}
}

// ProjectOntologyVersions returns a ProjectOntologyVersionStore backed by
// the weave_project_ontology_versions table.
func (s *PostgresStore) ProjectOntologyVersions() domain.ProjectOntologyVersionStore {
	return newProjectOntologyVersionStore(s.pool)
}

// NamespaceBindings returns a NamespaceBindingStore backed by this pool.
func (s *PostgresStore) NamespaceBindings() domain.NamespaceBindingStore {
	return newNamespaceBindingStore(s.pool)
}

// AllocateEntityNumber atomically reserves the next sequential number for
// the given (project_id, kind) via the weave_entity_counters UPSERT.
// Returns the value the caller should use; concurrent allocations each
// receive distinct numbers.
func (s *PostgresStore) AllocateEntityNumber(ctx context.Context, projectID, kind string) (int64, error) {
	n, err := s.queries.WeaveAllocateEntityNumber(ctx, sqlcgen.WeaveAllocateEntityNumberParams{
		ProjectID: projectID,
		Kind:      kind,
	})
	if err != nil {
		return 0, fmt.Errorf("allocate entity number (%s, %s): %w", projectID, kind, err)
	}
	return n, nil
}

// ReconcileEntityCounters bumps every counter to MAX(<existing N>) + 1.
// Idempotent. Call after a bulk import that wrote entities with explicit
// IDs (e.g. the wave importer) so subsequent allocations don't collide.
func (s *PostgresStore) ReconcileEntityCounters(ctx context.Context) error {
	if err := s.queries.WeaveReconcileEntityCounters(ctx); err != nil {
		return fmt.Errorf("reconcile entity counters: %w", err)
	}
	return nil
}

// ModelView resolves a model's fields with the override chain applied and
// groups them into category -> collection -> field hierarchy.
func (s *PostgresStore) ModelView(ctx context.Context, modelID, projectID string) (*domain.ModelView, error) {
	r := &resolver{queries: s.queries, pool: s.pool}
	if version := weaveauth.ProjectVersionFromContext(ctx); version != "" {
		return r.buildModelViewVersion(ctx, modelID, projectID, version)
	}
	return r.buildModelView(ctx, modelID, projectID)
}

// CollectionView resolves a collection's fields with overrides applied.
// Returns a flat list of resolved fields ordered by position.
func (s *PostgresStore) CollectionView(ctx context.Context, collectionID, projectID string) ([]domain.ResolvedField, error) {
	r := &resolver{queries: s.queries, pool: s.pool}
	if version := weaveauth.ProjectVersionFromContext(ctx); version != "" {
		return r.resolveForCollectionVersion(ctx, collectionID, projectID, version)
	}
	return r.resolveForCollection(ctx, collectionID, projectID)
}

// WithChangeLog opens a write transaction and runs fn with a ChangeLogger.
// On success, all accumulated change_log entries are inserted, the change_set
// is closed, and the transaction commits.
func (s *PostgresStore) WithChangeLog(
	ctx context.Context,
	fn func(*ChangeLogger) error,
) error {
	cs, err := resolveChangeSet(ctx, s)
	if err != nil {
		return fmt.Errorf("resolve change set: %w", err)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin change log tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	cl := &ChangeLogger{
		tx:      tx,
		queries: s.queries.WithTx(tx),
		csID:    cs.ID,
	}

	if err := fn(cl); err != nil {
		return err
	}

	// Backfill an implicit change set's project from its entries. A
	// CLI/background write with no request-scoped project hint gets an
	// empty-project change set (see resolveChangeSet); without this the
	// materializer can't route it (WeaveGetProjectByID("") → no rows) and it
	// clogs the outbox with failures forever. The entries carry the real
	// project, so adopt it when they agree on one.
	if cs.ProjectID == "" {
		if pid := singleEntryProjectID(cl.pending); pid != "" {
			if _, err := tx.Exec(ctx, `UPDATE weave_change_set SET project_id = $1 WHERE id = $2`, pid, cs.ID); err != nil {
				return fmt.Errorf("backfill change set project: %w", err)
			}
			cs.ProjectID = pid
		}
	}

	if err := cl.writeChangeLogEntries(ctx); err != nil {
		return err
	}

	if err := cl.queries.WeaveCloseChangeSet(ctx, cs.ID); err != nil {
		return fmt.Errorf("close change set: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit change log tx: %w", err)
	}

	return nil
}

// singleEntryProjectID returns the common project id of the entries, or ""
// when they are empty, absent, or span more than one project (in which case a
// single-project change set can't represent them).
func singleEntryProjectID(entries []domain.ChangeLogEntry) string {
	pid := ""
	for _, e := range entries {
		if e.ProjectID == "" {
			continue
		}
		if pid == "" {
			pid = e.ProjectID
		} else if pid != e.ProjectID {
			return ""
		}
	}
	return pid
}

// resolveChangeSet returns the change set from context or creates an implicit one.
func resolveChangeSet(ctx context.Context, s *PostgresStore) (*domain.ChangeSet, error) {
	if cs := domain.ChangeSetFromContext(ctx); cs != nil {
		return cs, nil
	}

	params := sqlcgen.WeaveCreateChangeSetParams{
		ProjectID:     "",
		ActorName:     "pletka-system",
		ActorEmail:    "system@pletka.local",
		CommitMessage: "implicit change",
	}
	// A request-scoped hint (set by mutation middleware) stamps the real
	// project and actor so the materializer routes the commit to
	// baseDir/<project_id> and records real authorship. Absent the hint
	// (CLI, background jobs) the system fallback above is used.
	if h, ok := domain.ChangeSetHintFromContext(ctx); ok {
		if h.ProjectID != "" {
			params.ProjectID = h.ProjectID
		}
		if h.ActorID != "" {
			params.ActorID = &h.ActorID
		}
		if h.ActorName != "" {
			params.ActorName = h.ActorName
		}
		if h.ActorEmail != "" {
			params.ActorEmail = h.ActorEmail
		}
		if h.CommitMessage != "" {
			params.CommitMessage = h.CommitMessage
		}
	}

	row, err := s.queries.WeaveCreateChangeSet(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("create implicit change set: %w", err)
	}

	cs := &domain.ChangeSet{
		ID:            row.ID,
		ProjectID:     row.ProjectID,
		ActorName:     row.ActorName,
		ActorEmail:    row.ActorEmail,
		CommitMessage: row.CommitMessage,
		StartedAt:     row.StartedAt,
	}
	if row.ActorID != nil {
		cs.ActorID = *row.ActorID
	}
	return cs, nil
}
