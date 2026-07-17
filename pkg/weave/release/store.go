package release

import "context"

type Store interface {
	ListByProject(ctx context.Context, projectID string) ([]Release, error)
	Get(ctx context.Context, projectID, version string) (*Release, error)
}
