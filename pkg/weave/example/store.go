package example

import (
	"context"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/domain"
)

// Reader is every read of example content, with the scope already bound.
// There is one method per read and no unscoped alternative: a caller cannot
// hold a Reader without having named a version, so reading the draft under a
// release is not forbidden, it is unrepresentable.
type Reader interface {
	// GetByID returns one example, or (nil, nil) when the scope does not
	// carry it. A release that never archived an example answers "not
	// here" rather than falling back to the draft.
	GetByID(ctx context.Context, id string) (*domain.Example, error)

	// List returns the examples in scope plus the total matching the
	// filters. A version that archived nothing returns an empty slice and
	// a zero total — a legitimate answer the view layer renders.
	List(ctx context.Context, opts ...domain.QueryOption) ([]*domain.Example, int64, error)

	// ListValues returns an example's values, from the same scope as the
	// example itself. Reading values live under a release would pair a
	// released example with edited values, which is worse than either.
	ListValues(ctx context.Context, exampleID string) ([]domain.ExampleValue, error)
}

// Store carries the mutations and hands out readers. Mutations take no scope:
// they always run hot, which is structural here rather than a rule to
// remember.
type Store interface {
	// At returns the reader for scope, or an error when scope is the
	// invalid zero value — which means a caller forgot to name one.
	At(scope auth.ReadScope) (Reader, error)

	CreateWithValues(ctx context.Context, ex *domain.Example, values []domain.ExampleValue) error
	UpdateWithValues(ctx context.Context, ex *domain.Example, values []domain.ExampleValue) error
	Delete(ctx context.Context, id string) error
}
