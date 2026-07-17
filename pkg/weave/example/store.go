package example

import (
	"context"

	"github.com/pletka-io/pletka/pkg/domain"
)

type Store interface {
	CreateWithValues(ctx context.Context, ex *domain.Example, values []domain.ExampleValue) error
	GetByID(ctx context.Context, id string) (*domain.Example, error)
	UpdateWithValues(ctx context.Context, ex *domain.Example, values []domain.ExampleValue) error
	Delete(ctx context.Context, id string) error
	List(ctx context.Context, opts ...domain.QueryOption) ([]*domain.Example, int64, error)
	ListValues(ctx context.Context, exampleID string) ([]domain.ExampleValue, error)
}
