package example

import (
	"context"

	"github.com/pletka-io/pletka/pkg/domain"
)

// Mutations owns the transaction and selected inputs for an entire example save.
// Construct it with NewPostgresMutations; it has no live-view fallback.
type Mutations struct {
	run func(context.Context, string, func(mutation) error) error
}

// Service receives only this operation-bound capability. Context, project and
// source selection are retained by the boundary, not repeated at each call.
type mutation interface {
	prepare(kind, id string, maxDepth int) (*slotResolver, valueValidator, error)
	example(id string) (*domain.Example, error)
	linkedExample(id string) (*domain.Example, error)
	stubTarget(id string) error
	create(*domain.Example, []domain.ExampleValue) error
	update(*domain.Example, []domain.ExampleValue) error
	delete(id string) error
}

type exampleNotFound struct{}

func (exampleNotFound) Error() string    { return "example not found" }
func (exampleNotFound) IsNotFound() bool { return true }

type mutationConflict struct{ cause error }

func (e mutationConflict) Error() string { return e.ConflictMessage() }
func (e mutationConflict) Unwrap() error { return e.cause }
func (mutationConflict) ConflictMessage() string {
	return "The project changed during this save. Reload and try again."
}

// Persistence has no read facet; validation cannot reach Store.At through it.
type mutationWriter interface {
	CreateWithValues(context.Context, *domain.Example, []domain.ExampleValue) error
	UpdateWithValues(context.Context, *domain.Example, []domain.ExampleValue) error
	Delete(context.Context, string) error
}
