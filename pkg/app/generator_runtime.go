package app

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/projectsnapshot"

	"github.com/pletka-io/pletka/pkg/domain"

	"github.com/pletka-io/pletka/pkg/weave/generators"
	"github.com/pletka-io/pletka/pkg/weave/genwiring"
)

// GeneratorRuntime exposes the app-assembled generator service plus the
// identifier resolution helpers needed by CLI workflows.
type GeneratorRuntime struct {
	Service *generators.Service

	value   projectsnapshot.CompleteProject
	project string
}

// NewGeneratorRuntime builds the same core generator service used by the HTTP
// app, without mounting routes or starting unrelated background services.
func NewGeneratorRuntime(opts Options) (*GeneratorRuntime, error) {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if opts.Pool == nil {
		return nil, fmt.Errorf("app generator runtime: pgx pool is required")
	}
	if err := opts.Contributions.validate(); err != nil {
		return nil, fmt.Errorf("app generator runtime: invalid contributions: %w", err)
	}

	renderers := opts.GeneratorRenderers
	if len(renderers) == 0 {
		renderers = genwiring.CoreRenderers()
	}
	// CLI is a trusted operator boundary; source policy is explicit.
	source := buildGeneratorSource(opts, func(context.Context, auth.Resource) error { return nil })
	genSvc := buildGeneratorService(source, renderers)
	return &GeneratorRuntime{Service: genSvc}, nil
}

// BindProject returns an operation-scoped runtime over one detached graph.
func (r *GeneratorRuntime) BindProject(ctx context.Context, scope auth.ReadScope, project string) (*GeneratorRuntime, error) {
	service, value, err := r.Service.BindProject(ctx, scope, project)
	if err != nil {
		return nil, err
	}
	return &GeneratorRuntime{Service: service, value: value, project: project}, nil
}

// ListModels reads the already selected project; it cannot reopen current data.
func (r *GeneratorRuntime) ListModels(ctx context.Context, project string) ([]*domain.Model, error) {
	if project != r.project {
		return nil, fmt.Errorf("bind generator project before listing")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	items, err := generators.EntityOptions(r.value, "model")
	if err != nil {
		return nil, err
	}
	out := make([]*domain.Model, 0, len(items))
	for _, item := range items {
		out = append(out, &domain.Model{Entity: domain.Entity{ID: item.ID, SemanticID: item.SemanticID, SystemName: item.SystemName, UIName: item.UIName, ProjectID: project}})
	}
	return out, nil
}

// ModelID resolves identifiers inside the bound operation.
func (r *GeneratorRuntime) ModelID(ctx context.Context, project, identifier string) (string, error) {
	if project != r.project {
		return "", fmt.Errorf("bind generator project before lookup")
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return generators.ResolveIdentifier(r.value, "model", identifier)
}

// CollectionID resolves identifiers inside the bound operation.
func (r *GeneratorRuntime) CollectionID(ctx context.Context, project, identifier string) (string, error) {
	if project != r.project {
		return "", fmt.Errorf("bind generator project before lookup")
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return generators.ResolveIdentifier(r.value, "collection", identifier)
}

// FieldID resolves identifiers inside the bound operation.
func (r *GeneratorRuntime) FieldID(ctx context.Context, project, identifier string) (string, error) {
	if project != r.project {
		return "", fmt.Errorf("bind generator project before lookup")
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return generators.ResolveIdentifier(r.value, "field", identifier)
}
