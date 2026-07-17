package weave

import (
	"context"
	"errors"
	"fmt"

	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// namespaceBindingStore is the pgx/sqlc-backed implementation of
// domain.NamespaceBindingStore over weave_namespace_bindings.
type namespaceBindingStore struct {
	queries *sqlcgen.Queries
	pool    *pgxpool.Pool
}

// Compile-time interface check.
var _ domain.NamespaceBindingStore = (*namespaceBindingStore)(nil)

// newNamespaceBindingStore returns a new namespaceBindingStore backed by pool.
func newNamespaceBindingStore(pool *pgxpool.Pool) *namespaceBindingStore {
	return &namespaceBindingStore{
		queries: sqlcgen.New(pool),
		pool:    pool,
	}
}

// List returns all namespace bindings visible to a project: global rows
// (project_id IS NULL) plus project-scoped rows. Passing an empty projectID
// returns only global rows because the SQL matches NULL-only rows when $1 is NULL.
func (s *namespaceBindingStore) List(ctx context.Context, projectID string) ([]*domain.NamespaceBinding, error) {
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
		out = append(out, weaveRowToNamespaceBinding(r))
	}
	return out, nil
}

// GetUser returns a single user-owned namespace binding. It returns
// domain.ErrReadOnly if the row has source='system', and an error if the row
// belongs to a different project.
func (s *namespaceBindingStore) GetUser(ctx context.Context, projectID, id string) (*domain.NamespaceBinding, error) {
	r, err := s.queries.WeaveGetNamespaceBinding(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("get namespace binding %s: not found", id)
	}
	if err != nil {
		return nil, fmt.Errorf("get namespace binding: %w", err)
	}
	if r.Source == "system" {
		return nil, fmt.Errorf("get namespace binding %s: %w", id, domain.ErrReadOnly)
	}
	if r.ProjectID == nil || *r.ProjectID != projectID {
		return nil, fmt.Errorf("get namespace binding %s: not owned by project %s", id, projectID)
	}
	return weaveRowToNamespaceBinding(r), nil
}

// CreateUser inserts a new user-owned namespace binding. The caller must
// set b.ID to a ULID generated via ids.GenerateULID().
func (s *namespaceBindingStore) CreateUser(ctx context.Context, b *domain.NamespaceBinding) error {
	if b.ID == "" {
		return errors.New("namespace binding: ID must be set by caller")
	}
	var pid *string
	if b.ProjectID != "" {
		pid = &b.ProjectID
	}
	_, err := s.queries.WeaveCreateUserNamespaceBinding(ctx, sqlcgen.WeaveCreateUserNamespaceBindingParams{
		ID:        b.ID,
		ProjectID: pid,
		Prefix:    b.Prefix,
		Namespace: b.Namespace,
		Weight:    b.Weight,
	})
	if err != nil {
		return fmt.Errorf("create namespace binding: %w", err)
	}
	return nil
}

// UpdateUser persists prefix, namespace, and weight changes on a user-owned
// row. Returns domain.ErrReadOnly when the row has source='system' (the SQL
// WHERE source='user' matches 0 rows, causing pgx.ErrNoRows on RETURNING).
func (s *namespaceBindingStore) UpdateUser(ctx context.Context, b *domain.NamespaceBinding) error {
	var pid *string
	if b.ProjectID != "" {
		pid = &b.ProjectID
	}
	_, err := s.queries.WeaveUpdateUserNamespaceBinding(ctx, sqlcgen.WeaveUpdateUserNamespaceBindingParams{
		ID:        b.ID,
		ProjectID: pid,
		Prefix:    b.Prefix,
		Namespace: b.Namespace,
		Weight:    b.Weight,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("update namespace binding %s: %w", b.ID, domain.ErrReadOnly)
	}
	if err != nil {
		return fmt.Errorf("update namespace binding: %w", err)
	}
	return nil
}

// DeleteUser removes a user-owned namespace binding. It probes the row first
// to distinguish a not-found case from a system-row guard violation.
func (s *namespaceBindingStore) DeleteUser(ctx context.Context, projectID, id string) error {
	row, err := s.queries.WeaveGetNamespaceBinding(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("delete namespace binding %s: not found", id)
	}
	if err != nil {
		return fmt.Errorf("delete namespace binding %s: %w", id, err)
	}
	if row.Source == "system" {
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

// ExistsByPrefixAndNamespace reports whether a binding with the given prefix
// and namespace already exists, excluding the row identified by excludeID
// (pass an empty string when creating). This mirrors the unique index on
// (prefix, namespace).
func (s *namespaceBindingStore) ExistsByPrefixAndNamespace(ctx context.Context, prefix, namespace, excludeID string) (bool, error) {
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

// weaveRowToNamespaceBinding converts a sqlcgen row to a *domain.NamespaceBinding.
func weaveRowToNamespaceBinding(r sqlcgen.WeaveNamespaceBinding) *domain.NamespaceBinding {
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
