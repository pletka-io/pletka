package hub

import (
	"context"
	"log/slog"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	weaveauth "github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/integrations"
	"github.com/pletka-io/pletka/pkg/integrations/registry"
	"github.com/pletka-io/pletka/pkg/weave/generators"
	"github.com/pletka-io/pletka/pkg/weave/projectontologyversion"
)

// ProjectArtifactProviderFactory supplies project-level integration artifacts.
type ProjectArtifactProviderFactory func(ctx context.Context, projectID string) registry.ProjectArtifactProvider

// Host is the explicit contract needed by the integrations hub routes.
type Host struct {
	Pool                           *pgxpool.Pool
	Weave                          domain.WeaveStore
	Logger                         *slog.Logger
	Registry                       *registry.Registry
	Cipher                         *integrations.Cipher
	Generators                     *generators.Service
	ManagedConfigResolver          ManagedConfigResolver
	ManagedLabelResolver           ManagedLabelResolver
	ManagedConflictCheck           ManagedConflictChecker
	ProjectArtifactProviderFactory ProjectArtifactProviderFactory
}

// Mount registers the hub's routes on a parent already scoped under
// /projects/{projectID}/integrations.
func Mount(parent chi.Router, h Host) {
	if h.Registry == nil {
		// Hub disabled — no registry means no routes to mount. The
		// settings page schema's Integrations section will surface an
		// empty state.
		return
	}

	if h.Generators == nil {
		panic("integrations hub: generator service is required")
	}

	bundles := projectontologyversion.NewPostgresStore(h.Pool)
	svc := NewService(NewPostgresStore(h.Pool), h.Registry, h.Cipher, h.Logger)
	if h.ManagedConfigResolver != nil {
		svc.SetManagedConfigResolver(h.ManagedConfigResolver)
	}
	if h.ManagedLabelResolver != nil {
		svc.SetManagedLabelResolver(h.ManagedLabelResolver)
	}
	if h.ManagedConflictCheck != nil {
		svc.SetManagedConflictChecker(h.ManagedConflictCheck)
	}
	handler := NewHandler(h.Weave, svc, h.Generators, bundles, h.ProjectArtifactProviderFactory, h.Logger)

	parent.Group(func(r chi.Router) {
		r.Use(weaveauth.WithProjectVersionContext)
		if h.Weave != nil {
			r.Use(weaveauth.WithProjectResource(h.Weave))
		}
		r.Get("/list-schema", handler.ListSchema)
		r.Get("/list-schema/data", handler.ListSchemaData)
		r.Post("/{integrationID}/configs", handler.AddConfig)
		r.Get("/{integrationID}/configs/{configID}/config-schema", handler.ConfigSchema)
		r.Put("/{integrationID}/configs/{configID}/config", handler.UpdateConfig)
		r.Post("/{integrationID}/configs/{configID}/enable", handler.SetEnabled)
		r.Delete("/{integrationID}/configs/{configID}", handler.Remove)
		// Both POST and GET dispatch to the same handler. ActionSpec
		// declares the intended verb via Result; the hub uses GET URLs
		// for read-only "panel" actions so the browser can cache them
		// + CSRF stays trivial.
		r.Post("/{integrationID}/configs/{configID}/actions/{actionID}", handler.RunAction)
		r.Get("/{integrationID}/configs/{configID}/actions/{actionID}", handler.RunAction)
	})
}
