package weave

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/pletka-io/pletka/domain"
	domainweave "github.com/pletka-io/pletka/domain/weave"
	"github.com/pletka-io/pletka/store/sqlcgen"
)

type namespaceBindingStore struct {
	queries *sqlcgen.Queries
}

var _ domainweave.NamespaceBindingStore = (*namespaceBindingStore)(nil)

// NamespaceBindings returns the store slice for namespace prefix bindings.
func (s *Store) NamespaceBindings() domainweave.NamespaceBindingStore {
	if s == nil {
		return &namespaceBindingStore{}
	}
	return &namespaceBindingStore{queries: s.queries}
}

func (s *namespaceBindingStore) List(ctx context.Context, projectID string) ([]*domainweave.NamespaceBinding, error) {
	queries, err := s.queryFacade()
	if err != nil {
		return nil, err
	}
	var pid *string
	if projectID != "" {
		pid = &projectID
	}
	rows, err := queries.WeaveListNamespaceBindings(ctx, pid)
	if err != nil {
		return nil, fmt.Errorf("list namespace bindings: %w", err)
	}
	bindings := make([]*domainweave.NamespaceBinding, 0, len(rows))
	for _, row := range rows {
		bindings = append(bindings, namespaceBindingFromRow(row))
	}
	return bindings, nil
}

func (s *namespaceBindingStore) GetUser(ctx context.Context, projectID, id string) (*domainweave.NamespaceBinding, error) {
	queries, err := s.queryFacade()
	if err != nil {
		return nil, err
	}
	row, err := queries.WeaveGetNamespaceBinding(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("get namespace binding %s: not found", id)
	}
	if err != nil {
		return nil, fmt.Errorf("get namespace binding: %w", err)
	}
	if row.Source == "system" {
		return nil, fmt.Errorf("get namespace binding %s: %w", id, domain.ErrReadOnly)
	}
	if !namespaceProjectMatches(row.ProjectID, projectID) {
		return nil, fmt.Errorf("get namespace binding %s: not owned by project %s", id, projectID)
	}
	return namespaceBindingFromRow(row), nil
}

func (s *namespaceBindingStore) CreateUser(ctx context.Context, binding *domainweave.NamespaceBinding) error {
	queries, err := s.queryFacade()
	if err != nil {
		return err
	}
	if binding == nil {
		return errors.New("namespace binding is nil")
	}
	if binding.ID == "" {
		return errors.New("namespace binding ID must be set by caller")
	}
	var pid *string
	if binding.ProjectID != "" {
		pid = &binding.ProjectID
	}
	_, err = queries.WeaveCreateUserNamespaceBinding(ctx, sqlcgen.WeaveCreateUserNamespaceBindingParams{
		ID:        binding.ID,
		ProjectID: pid,
		Prefix:    binding.Prefix,
		Namespace: binding.Namespace,
		Weight:    binding.Weight,
	})
	if err != nil {
		return fmt.Errorf("create namespace binding: %w", err)
	}
	return nil
}

func (s *namespaceBindingStore) UpdateUser(ctx context.Context, binding *domainweave.NamespaceBinding) error {
	queries, err := s.queryFacade()
	if err != nil {
		return err
	}
	if binding == nil {
		return errors.New("namespace binding is nil")
	}

	if binding.ProjectID == "" {
		_, err = queries.WeaveUpdateGlobalUserNamespaceBinding(ctx, sqlcgen.WeaveUpdateGlobalUserNamespaceBindingParams{
			ID:        binding.ID,
			Prefix:    binding.Prefix,
			Namespace: binding.Namespace,
			Weight:    binding.Weight,
		})
	} else {
		_, err = queries.WeaveUpdateUserNamespaceBinding(ctx, sqlcgen.WeaveUpdateUserNamespaceBindingParams{
			ID:        binding.ID,
			ProjectID: &binding.ProjectID,
			Prefix:    binding.Prefix,
			Namespace: binding.Namespace,
			Weight:    binding.Weight,
		})
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("update namespace binding %s: %w", binding.ID, domain.ErrReadOnly)
	}
	if err != nil {
		return fmt.Errorf("update namespace binding: %w", err)
	}
	return nil
}

func (s *namespaceBindingStore) DeleteUser(ctx context.Context, projectID, id string) error {
	queries, err := s.queryFacade()
	if err != nil {
		return err
	}
	row, err := queries.WeaveGetNamespaceBinding(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("delete namespace binding %s: not found", id)
	}
	if err != nil {
		return fmt.Errorf("delete namespace binding %s: %w", id, err)
	}
	if row.Source == "system" {
		return fmt.Errorf("delete namespace binding %s: %w", id, domain.ErrReadOnly)
	}
	if !namespaceProjectMatches(row.ProjectID, projectID) {
		return fmt.Errorf("delete namespace binding %s: not owned by project %s", id, projectID)
	}

	if projectID == "" {
		err = queries.WeaveDeleteGlobalUserNamespaceBinding(ctx, id)
	} else {
		err = queries.WeaveDeleteUserNamespaceBinding(ctx, sqlcgen.WeaveDeleteUserNamespaceBindingParams{
			ID:        id,
			ProjectID: &projectID,
		})
	}
	if err != nil {
		return fmt.Errorf("delete namespace binding: %w", err)
	}
	return nil
}

func (s *namespaceBindingStore) ExistsByPrefixAndNamespace(ctx context.Context, prefix, namespace, excludeID string) (bool, error) {
	queries, err := s.queryFacade()
	if err != nil {
		return false, err
	}
	exists, err := queries.WeavePrefixNamespaceBindingExists(ctx, sqlcgen.WeavePrefixNamespaceBindingExistsParams{
		Prefix:    prefix,
		Namespace: namespace,
		ID:        excludeID,
	})
	if err != nil {
		return false, fmt.Errorf("prefix namespace binding exists: %w", err)
	}
	return exists, nil
}

func (s *namespaceBindingStore) queryFacade() (*sqlcgen.Queries, error) {
	if s == nil || s.queries == nil {
		return nil, errors.New("namespace binding store is not initialized")
	}
	return s.queries, nil
}

func namespaceBindingFromRow(row sqlcgen.WeaveNamespaceBinding) *domainweave.NamespaceBinding {
	binding := &domainweave.NamespaceBinding{
		ID:        row.ID,
		Prefix:    row.Prefix,
		Namespace: row.Namespace,
		Weight:    row.Weight,
		Source:    row.Source,
	}
	if row.ProjectID != nil {
		binding.ProjectID = *row.ProjectID
	}
	return binding
}

func namespaceProjectMatches(rowProjectID *string, projectID string) bool {
	if projectID == "" {
		return rowProjectID == nil || *rowProjectID == ""
	}
	return rowProjectID != nil && *rowProjectID == projectID
}
