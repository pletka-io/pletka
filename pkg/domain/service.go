package domain

import (
	"context"

	"github.com/go-chi/chi/v5"
)

// Service is the core interface for all feature services.
// Services are independent -- they never import each other.
type Service interface {
	Name() string
	Routes(r chi.Router)
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}
