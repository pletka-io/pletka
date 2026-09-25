package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/formschema"
	"github.com/pletka-io/pletka/pkg/weave"
	"github.com/pletka-io/pletka/pkg/weave/collection"
	"github.com/pletka-io/pletka/pkg/weave/field"
	"github.com/pletka-io/pletka/pkg/weave/generators"
	"github.com/pletka-io/pletka/pkg/weave/genwiring"
	"github.com/pletka-io/pletka/pkg/weave/model"
	"github.com/pletka-io/pletka/pkg/weave/vocabulary"
)

// GeneratorRuntime exposes the app-assembled generator service plus the
// identifier resolution helpers needed by CLI workflows.
type GeneratorRuntime struct {
	Service *generators.Service

	models      model.Store
	collections collection.Store
	fields      field.Store
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

	weaveStore := weave.NewPostgresStore(opts.Pool)
	changeLog := domain.NoopChangeLogRunner()
	languages := []formschema.LanguageInfo(nil)
	langResolver := func(*http.Request) string { return "" }
	projectHost := buildProjectHost(opts.Pool, weaveStore, opts.Logger, changeLog, languages, nil)
	_, namespaceSvc := buildNamespaceBindingHost(opts.Pool, opts.Logger, changeLog, languages, nil)
	fieldHost, modelHost, collectionHost, _ := buildCoreEntityHosts(coreEntityDeps{
		Pool:      opts.Pool,
		Weave:     weaveStore,
		Logger:    opts.Logger,
		ChangeLog: changeLog,
		Languages: languages,
	}, langResolver)

	renderers := opts.GeneratorRenderers
	if len(renderers) == 0 {
		renderers = genwiring.CoreRenderers()
	}
	genSvc := buildGeneratorService(
		opts.Logger,
		projectHost,
		fieldHost,
		modelHost,
		collectionHost,
		namespaceSvc,
		renderers,
	)
	// Sealed-list value enums in generated schemas (#3599).
	genSvc.SetConceptEnumReader(vocabulary.NewService(opts.Pool, nil))
	return &GeneratorRuntime{
		Service:     genSvc,
		models:      model.NewPostgresStore(opts.Pool),
		collections: collection.NewPostgresStore(opts.Pool),
		fields:      field.NewPostgresStore(opts.Pool),
	}, nil
}

// ListModels returns every model in the given project. Used by CLI --all and
// pack workflows.
func (r *GeneratorRuntime) ListModels(ctx context.Context, projectID string) ([]*domain.Model, error) {
	models, _, err := r.models.List(ctx, domain.WithProjectID(projectID))
	if err != nil {
		return nil, fmt.Errorf("list models for project %s: %w", projectID, err)
	}
	return models, nil
}

func (r *GeneratorRuntime) ModelID(ctx context.Context, projectID, identifier string) (string, error) {
	model, err := r.models.GetByIdentifier(ctx, projectID, identifier)
	if err != nil {
		return "", fmt.Errorf("resolve model %q: %w", identifier, err)
	}
	if model == nil {
		return "", fmt.Errorf("model %q not found in project %s", identifier, projectID)
	}
	return model.ID, nil
}

func (r *GeneratorRuntime) CollectionID(ctx context.Context, projectID, identifier string) (string, error) {
	collection, err := r.collections.GetByIdentifier(ctx, projectID, identifier)
	if err != nil {
		return "", fmt.Errorf("resolve collection %q: %w", identifier, err)
	}
	if collection == nil {
		return "", fmt.Errorf("collection %q not found in project %s", identifier, projectID)
	}
	return collection.ID, nil
}

func (r *GeneratorRuntime) FieldID(ctx context.Context, projectID, identifier string) (string, error) {
	field, err := r.fields.GetByIdentifier(ctx, projectID, identifier)
	if err != nil {
		return "", fmt.Errorf("resolve field %q: %w", identifier, err)
	}
	if field == nil {
		return "", fmt.Errorf("field %q not found in project %s", identifier, projectID)
	}
	return field.ID, nil
}
