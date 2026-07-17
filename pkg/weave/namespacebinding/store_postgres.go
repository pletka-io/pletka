package namespacebinding

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"time"

	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/oklog/ulid/v2"
)

// postgresStore is the pgx + sqlc implementation of Store, backed by the
// weave_namespace_bindings table.
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

// List returns global + project-scoped bindings, sorted by weight ASC.
// Empty projectID returns only globals.
func (s *postgresStore) List(ctx context.Context, projectID string) ([]*domain.NamespaceBinding, error) {
	var pid *string
	if projectID != "" {
		pid = &projectID
	}
	rows, err := s.queries.WeaveListNamespaceBindings(ctx, pid)
	if err != nil {
		return nil, fmt.Errorf("list namespace bindings: %w", err)
	}
	out := make([]*domain.NamespaceBinding, 0, len(rows))
	for _, r := range rows {
		out = append(out, rowToBinding(r))
	}
	return out, nil
}

func (s *postgresStore) ListVersion(ctx context.Context, projectID, releaseVersion string) ([]*domain.NamespaceBinding, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, project_id, prefix, namespace, weight, source
		FROM weave_namespace_bindings_archive
		WHERE version_number = $2
		  AND (project_id = $1 OR project_id IS NULL)
		ORDER BY weight ASC, prefix ASC
	`, projectID, releaseVersion)
	if err != nil {
		return nil, fmt.Errorf("list archived namespace bindings: %w", err)
	}
	defer rows.Close()
	out := []*domain.NamespaceBinding{}
	for rows.Next() {
		var (
			id        string
			projectID *string
			prefix    string
			namespace string
			weight    int64
			source    string
		)
		if err := rows.Scan(&id, &projectID, &prefix, &namespace, &weight, &source); err != nil {
			return nil, fmt.Errorf("scan archived namespace binding: %w", err)
		}
		b := &domain.NamespaceBinding{
			ID:        id,
			Prefix:    prefix,
			Namespace: namespace,
			Weight:    weight,
			Source:    source,
		}
		if projectID != nil {
			b.ProjectID = *projectID
		}
		out = append(out, b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate archived namespace bindings: %w", err)
	}
	return out, nil
}

func (s *postgresStore) ListGlobal(ctx context.Context) ([]*domain.NamespaceBinding, error) {
	rows, err := s.queries.WeaveListGlobalNamespaceBindings(ctx)
	if err != nil {
		return nil, fmt.Errorf("list global namespace bindings: %w", err)
	}
	out := make([]*domain.NamespaceBinding, 0, len(rows))
	for _, r := range rows {
		out = append(out, rowToBinding(r))
	}
	return out, nil
}

// Get returns a user-owned binding scoped to projectID. Errors:
//   - "not found" if row is missing
//   - domain.ErrReadOnly if row has source != "user"
//   - "not owned by project" if row's project_id mismatches
func (s *postgresStore) Get(ctx context.Context, projectID, id string) (*domain.NamespaceBinding, error) {
	r, err := s.queries.WeaveGetNamespaceBinding(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("get namespace binding %s: not found", id)
	}
	if err != nil {
		return nil, fmt.Errorf("get namespace binding: %w", err)
	}
	if r.Source != "user" {
		return nil, fmt.Errorf("get namespace binding %s: %w", id, domain.ErrReadOnly)
	}
	if r.ProjectID == nil || *r.ProjectID != projectID {
		return nil, fmt.Errorf("get namespace binding %s: not owned by project %s", id, projectID)
	}
	return rowToBinding(r), nil
}

func (s *postgresStore) GetGlobal(ctx context.Context, id string) (*domain.NamespaceBinding, error) {
	r, err := s.queries.WeaveGetNamespaceBinding(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("get global namespace binding %s: not found", id)
	}
	if err != nil {
		return nil, fmt.Errorf("get global namespace binding: %w", err)
	}
	if r.Source != "user" {
		return nil, fmt.Errorf("get global namespace binding %s: %w", id, domain.ErrReadOnly)
	}
	if r.ProjectID != nil && *r.ProjectID != "" {
		return nil, fmt.Errorf("get global namespace binding %s: not global", id)
	}
	return rowToBinding(r), nil
}

func (s *postgresStore) Lookup(ctx context.Context, id string) (*domain.NamespaceBinding, error) {
	r, err := s.queries.WeaveGetNamespaceBinding(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("lookup namespace binding %s: not found", id)
	}
	if err != nil {
		return nil, fmt.Errorf("lookup namespace binding: %w", err)
	}
	return rowToBinding(r), nil
}

// Create inserts a new user binding. Caller may leave b.ID empty — the
// store generates a ULID.
func (s *postgresStore) Create(ctx context.Context, b *domain.NamespaceBinding) error {
	if b.ID == "" {
		b.ID = generateULID()
	}
	var pid *string
	if b.ProjectID != "" {
		pid = &b.ProjectID
	}
	if _, err := s.queries.WeaveCreateUserNamespaceBinding(ctx, sqlcgen.WeaveCreateUserNamespaceBindingParams{
		ID:        b.ID,
		ProjectID: pid,
		Prefix:    b.Prefix,
		Namespace: b.Namespace,
		Weight:    b.Weight,
	}); err != nil {
		return fmt.Errorf("create namespace binding: %w", err)
	}
	return nil
}

func (s *postgresStore) CreateGlobal(ctx context.Context, b *domain.NamespaceBinding) error {
	b.ProjectID = ""
	return s.Create(ctx, b)
}

// Update persists prefix, namespace, weight changes on a user-owned row.
// Returns domain.ErrReadOnly when the row has source != "user" (the
// underlying SQL filters by source = 'user' and surfaces pgx.ErrNoRows).
func (s *postgresStore) Update(ctx context.Context, b *domain.NamespaceBinding) error {
	var pid *string
	if b.ProjectID != "" {
		pid = &b.ProjectID
	}
	if _, err := s.queries.WeaveUpdateUserNamespaceBinding(ctx, sqlcgen.WeaveUpdateUserNamespaceBindingParams{
		ID:        b.ID,
		ProjectID: pid,
		Prefix:    b.Prefix,
		Namespace: b.Namespace,
		Weight:    b.Weight,
	}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("update namespace binding %s: %w", b.ID, domain.ErrReadOnly)
		}
		return fmt.Errorf("update namespace binding: %w", err)
	}
	return nil
}

func (s *postgresStore) UpdateGlobal(ctx context.Context, b *domain.NamespaceBinding) error {
	if _, err := s.queries.WeaveUpdateGlobalUserNamespaceBinding(ctx, sqlcgen.WeaveUpdateGlobalUserNamespaceBindingParams{
		ID:        b.ID,
		Prefix:    b.Prefix,
		Namespace: b.Namespace,
		Weight:    b.Weight,
	}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("update global namespace binding %s: %w", b.ID, domain.ErrReadOnly)
		}
		return fmt.Errorf("update global namespace binding: %w", err)
	}
	return nil
}

// Delete probes the row first to distinguish "not found" from "read only".
func (s *postgresStore) Delete(ctx context.Context, projectID, id string) error {
	row, err := s.queries.WeaveGetNamespaceBinding(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("delete namespace binding %s: not found", id)
	}
	if err != nil {
		return fmt.Errorf("delete namespace binding %s: %w", id, err)
	}
	if row.Source != "user" {
		return fmt.Errorf("delete namespace binding %s: %w", id, domain.ErrReadOnly)
	}

	var pid *string
	if projectID != "" {
		pid = &projectID
	}
	if err := s.queries.WeaveDeleteUserNamespaceBinding(ctx, sqlcgen.WeaveDeleteUserNamespaceBindingParams{
		ID:        id,
		ProjectID: pid,
	}); err != nil {
		return fmt.Errorf("delete namespace binding: %w", err)
	}
	return nil
}

func (s *postgresStore) DeleteGlobal(ctx context.Context, id string) error {
	row, err := s.queries.WeaveGetNamespaceBinding(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("delete global namespace binding %s: not found", id)
	}
	if err != nil {
		return fmt.Errorf("delete global namespace binding %s: %w", id, err)
	}
	if row.Source != "user" {
		return fmt.Errorf("delete global namespace binding %s: %w", id, domain.ErrReadOnly)
	}
	if row.ProjectID != nil && *row.ProjectID != "" {
		return fmt.Errorf("delete global namespace binding %s: not global", id)
	}
	if err := s.queries.WeaveDeleteGlobalUserNamespaceBinding(ctx, id); err != nil {
		return fmt.Errorf("delete global namespace binding: %w", err)
	}
	return nil
}

// ExistsByPrefixAndNamespace mirrors the unique index check.
func (s *postgresStore) ExistsByPrefixAndNamespace(ctx context.Context, prefix, namespace, excludeID string) (bool, error) {
	exists, err := s.queries.WeavePrefixNamespaceBindingExists(ctx, sqlcgen.WeavePrefixNamespaceBindingExistsParams{
		Prefix:    prefix,
		Namespace: namespace,
		ID:        excludeID,
	})
	if err != nil {
		return false, fmt.Errorf("prefix namespace exists: %w", err)
	}
	return exists, nil
}

func (s *postgresStore) UsageByProject(ctx context.Context, prefix string) ([]ProjectUsage, error) {
	rows, err := s.queries.WeaveGlobalNamespaceBindingUsageByProject(ctx, prefix)
	if err != nil {
		return nil, fmt.Errorf("namespace binding usage by project: %w", err)
	}
	out := make([]ProjectUsage, 0, len(rows))
	for _, r := range rows {
		out = append(out, ProjectUsage{
			ProjectID:       r.ProjectID,
			FieldCount:      r.FieldCount,
			ModelCount:      r.ModelCount,
			CollectionCount: r.CollectionCount,
		})
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Row converter
// ---------------------------------------------------------------------------

func rowToBinding(r sqlcgen.WeaveNamespaceBinding) *domain.NamespaceBinding {
	b := &domain.NamespaceBinding{
		ID:        r.ID,
		Prefix:    r.Prefix,
		Namespace: r.Namespace,
		Weight:    r.Weight,
		Source:    r.Source,
	}
	if r.ProjectID != nil {
		b.ProjectID = *r.ProjectID
	}
	return b
}

// generateULID returns a new monotonic ULID. Inlined for slice
// self-containment; extract to a shared helpers package once a third slice
// needs the same function.
func generateULID() string {
	entropy := ulid.Monotonic(rand.Reader, 0)
	return ulid.MustNew(ulid.Timestamp(time.Now()), entropy).String()
}
