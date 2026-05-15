package weave

import "context"

// NamespaceBinding maps a compact namespace prefix to a full namespace URI.
// Rows with Source="system" are read-only.
type NamespaceBinding struct {
	ID        string `json:"id"`
	ProjectID string `json:"project_id,omitempty"`
	Prefix    string `json:"prefix"`
	Namespace string `json:"namespace"`
	Weight    int64  `json:"weight"`
	Source    string `json:"source"`
}

// NamespaceBindingStore manages global and project-scoped namespace prefix bindings.
type NamespaceBindingStore interface {
	List(ctx context.Context, projectID string) ([]*NamespaceBinding, error)
	GetUser(ctx context.Context, projectID, id string) (*NamespaceBinding, error)
	CreateUser(ctx context.Context, binding *NamespaceBinding) error
	UpdateUser(ctx context.Context, binding *NamespaceBinding) error
	DeleteUser(ctx context.Context, projectID, id string) error
	ExistsByPrefixAndNamespace(ctx context.Context, prefix, namespace, excludeID string) (bool, error)
}
