package namespacebinding

import (
	"context"

	"github.com/pletka-io/pletka/pkg/domain"
)

// Store is the data-access contract for namespace bindings.
//
// List returns BOTH global ("system") rows AND rows scoped to the supplied
// project — they're rendered together in the bindings pane. Mutation
// methods (Create / Update / Delete) only operate on user-sourced rows;
// the underlying SQL guards against system rows so accidental writes
// surface as domain.ErrReadOnly.
type Store interface {
	// List returns global + project-scoped bindings, sorted by weight ASC,
	// then prefix ASC. Pass an empty projectID to fetch only globals.
	List(ctx context.Context, projectID string) ([]*domain.NamespaceBinding, error)

	// ListGlobal returns only global bindings (project_id NULL/empty),
	// sorted by weight ASC then prefix ASC.
	ListGlobal(ctx context.Context) ([]*domain.NamespaceBinding, error)

	// Get returns a single user binding owned by projectID, or an error
	// when the row is missing, read-only (system / imported), or owned by
	// a different project.
	Get(ctx context.Context, projectID, id string) (*domain.NamespaceBinding, error)

	// GetGlobal returns a single user-owned global binding (project_id
	// NULL/empty). Used by the superadmin admin section edit flow.
	GetGlobal(ctx context.Context, id string) (*domain.NamespaceBinding, error)

	// Lookup returns any binding by ID, including read-only/system rows.
	// Used by stats/reporting flows that must surface immutable rows too.
	Lookup(ctx context.Context, id string) (*domain.NamespaceBinding, error)

	// Create inserts a new user binding. Caller must set b.ID (ULID) and
	// b.ProjectID. Returns domain.ErrReadOnly via the underlying SQL guard
	// if the resulting row would be system-sourced.
	Create(ctx context.Context, b *domain.NamespaceBinding) error

	// CreateGlobal inserts a new global user binding.
	CreateGlobal(ctx context.Context, b *domain.NamespaceBinding) error

	// Update persists changes to prefix, namespace, weight on a user-owned
	// row. Returns domain.ErrReadOnly when the target row has source !=
	// "user".
	Update(ctx context.Context, b *domain.NamespaceBinding) error

	// UpdateGlobal persists changes to a user-owned global row.
	UpdateGlobal(ctx context.Context, b *domain.NamespaceBinding) error

	// Delete removes a user-owned binding scoped to projectID. Returns
	// domain.ErrReadOnly when targeting a non-user row.
	Delete(ctx context.Context, projectID, id string) error

	// DeleteGlobal removes a user-owned global binding.
	DeleteGlobal(ctx context.Context, id string) error

	// ExistsByPrefixAndNamespace returns true when another row (excluding
	// excludeID) already uses the given (prefix, namespace) pair. Mirrors
	// the unique index. Pass excludeID="" on create.
	ExistsByPrefixAndNamespace(ctx context.Context, prefix, namespace, excludeID string) (bool, error)

	// UsageByProject reports where a namespace prefix is actually used
	// across fields, models, and collections.
	UsageByProject(ctx context.Context, prefix string) ([]ProjectUsage, error)
}

type ProjectUsage struct {
	ProjectID       string
	FieldCount      int64
	ModelCount      int64
	CollectionCount int64
}

func (p ProjectUsage) TotalCount() int64 {
	return p.FieldCount + p.ModelCount + p.CollectionCount
}
