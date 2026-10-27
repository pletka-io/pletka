package gitmaterializer

import (
	"context"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/database/readsession"
	"github.com/pletka-io/pletka/pkg/database/releasedescriptor"
)

func (m *Materializer) releaseDescriptors(ctx context.Context, project, version string) (releasedescriptor.Snapshot, error) {
	var snapshot releasedescriptor.Snapshot
	err := readsession.NewFactory(m.pool).Read(ctx, auth.Release(version), project, func(s *readsession.Session) error {
		var err error
		snapshot, err = s.Descriptors(ctx)
		return err
	})
	return snapshot, err
}
