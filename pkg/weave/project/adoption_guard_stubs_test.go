package project

import (
	"context"

	weaveauth "github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/release"
)

type stubModelLister struct{}

func (stubModelLister) ListAt(context.Context, weaveauth.ReadScope, string, ...domain.QueryOption) ([]*domain.Model, int64, error) {
	return nil, 0, nil
}

type stubCollectionLister struct{}

func (stubCollectionLister) ListAt(context.Context, weaveauth.ReadScope, string, ...domain.QueryOption) ([]*domain.Collection, int64, error) {
	return nil, 0, nil
}

type stubReleaseLister struct{}

func (stubReleaseLister) ListByProject(context.Context, string) ([]release.Release, error) {
	return nil, nil
}
