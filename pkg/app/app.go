// Package app assembles the HTTP application from explicit dependencies.
package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/pkg/app/observability"
	weaveauth "github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/i18n"
	"github.com/pletka-io/pletka/pkg/integrations"
	"github.com/pletka-io/pletka/pkg/integrations/hub"
	"github.com/pletka-io/pletka/pkg/integrations/registry"
	"github.com/pletka-io/pletka/pkg/service/gitmaterializer"
	"github.com/pletka-io/pletka/pkg/session"
	"github.com/pletka-io/pletka/pkg/weave"
	"github.com/pletka-io/pletka/pkg/weave/admin"
	"github.com/pletka-io/pletka/pkg/weave/apikey"
	weavecontent "github.com/pletka-io/pletka/pkg/weave/content"
	"github.com/pletka-io/pletka/pkg/weave/generators"
	"github.com/pletka-io/pletka/pkg/weave/genwiring"
	"github.com/pletka-io/pletka/pkg/weave/materializationadmin"
	ontologyslice "github.com/pletka-io/pletka/pkg/weave/ontology"
	"github.com/pletka-io/pletka/pkg/weave/pathaudit"
	weaverouter "github.com/pletka-io/pletka/pkg/weave/router"
	weavetemplates "github.com/pletka-io/pletka/pkg/weave/templates"
)

// IntegrationProjectArtifactProviderFactory supplies project-level integration
// artifacts to the integrations hub from a narrow app-built context.
type IntegrationProjectArtifactProviderFactory func(ctx context.Context, artifact integrations.ProjectArtifactContext, projectID string) registry.ProjectArtifactProvider

// Options configures application assembly. Runtime concerns such as listener
// setup, process signals, migrations, and config loading stay in cmd for now.
type Options struct {
	Pool   *pgxpool.Pool
	Logger *slog.Logger

	DevMode             bool
	Instance            string
	LogStaticFiles      bool
	AllowedOrigins      []string
	RegistrationEnabled bool
	SSOLoginURL         string
	GitDataDir          string
	// ModuleHost/OntologyHost seed the gitmaterializer module-path namespace
	// written into pletka.mod/ontology.yaml files. Empty falls back to the
	// public-clean defaults (pletka.io / ontology.pletka.io) — see
	// gitmaterializer.NewMaterializer.
	ModuleHost               string
	OntologyHost             string
	MetricsAddr              string
	Analytics                weavetemplates.AnalyticsConfig
	ContentOverlayPath       string
	FrontendManifestPaths    []string
	CoreFrontendManifestPath string

	IntegrationRegistry *registry.Registry
	IntegrationCipher   *integrations.Cipher

	Contributions Contributions

	GeneratorRenderers []generators.Renderer

	AutocompleteMode          string
	AutocompleteAllowOverride bool

	ManagedConfigResolver hub.ManagedConfigResolver
	ManagedLabelResolver  hub.ManagedLabelResolver
	ManagedConflictCheck  hub.ManagedConflictChecker

	ProjectArtifactProviderFactory IntegrationProjectArtifactProviderFactory
}

// App is the assembled HTTP application and the services it owns.
type App struct {
	Handler   http.Handler
	Weave     domain.WeaveStore
	I18n      i18n.Manager
	Session   *session.Manager
	Templates *weavetemplates.Renderer

	closeFns []func(context.Context) error
}

// New assembles the Pletka HTTP app from an already-open pgx pool.
func New(ctx context.Context, opts Options) (*App, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if opts.Pool == nil {
		return nil, fmt.Errorf("app: pgx pool is required")
	}
	if err := opts.Contributions.validate(); err != nil {
		return nil, fmt.Errorf("app: invalid contributions: %w", err)
	}

	logger := opts.Logger

	presentation, err := buildPresentation(ctx, PresentationOptions{
		Logger:                   logger,
		DevMode:                  opts.DevMode,
		ContentOverlayPath:       opts.ContentOverlayPath,
		FrontendManifestPaths:    opts.FrontendManifestPaths,
		CoreFrontendManifestPath: opts.CoreFrontendManifestPath,
		StaticAssets:             opts.Contributions.StaticAssets,
		FrontendManifests:        opts.Contributions.FrontendManifests,
		Analytics:                opts.Analytics,
	})
	if err != nil {
		return nil, err
	}
	i18nManager := presentation.I18n
	templateRenderer := presentation.Templates
	contentSources := presentation.ContentSources
	logger.Info("Weave template renderer initialized", "devMode", opts.DevMode, "frontend_manifest_count", presentation.FrontendManifestCount)

	logger.Info("Initializing session manager")
	sessionManager := session.NewManager(logger)
	logger.Info("Session manager initialized")

	weaveStore := weave.NewPostgresStore(opts.Pool)
	logger.Info("WeaveStore initialized with pgx pool")

	apikeyService := apikey.NewService(apikey.NewPostgresStore(opts.Pool), logger)

	if err := sessionManager.SetupPgxStore(opts.Pool); err != nil {
		return nil, fmt.Errorf("attach pgx session store: %w", err)
	}
	logger.Info("Session store attached (pgx)")

	gitDataDir := opts.GitDataDir
	if gitDataDir == "" {
		gitDataDir = "./data/git-projects"
	}
	materializer := gitmaterializer.NewMaterializer(opts.Pool, gitDataDir, logger).
		WithModuleHosts(opts.ModuleHost, opts.OntologyHost)
	gitMatService := gitmaterializer.NewService(materializer, nil, logger)
	// Started below, after WithOntologyImporter — the importer field must be
	// set before any goroutine of the service is running.

	// Observability runtime: request metrics middleware + /metrics listener
	// (started when MetricsAddr is set). One per process = one instance.
	obs := observability.New(observability.Config{MetricsAddr: opts.MetricsAddr}, opts.Pool, logger)

	// Path audit: stored-path validity sweep, sanctioned raw-pool holder
	// (see database-patterns.md). One instance, exposed via Services.
	pathAuditSvc := pathaudit.NewService(opts.Pool)

	closeFns := []func(context.Context) error{
		func(ctx context.Context) error {
			return gitMatService.Stop(ctx)
		},
		func(ctx context.Context) error {
			return obs.Close(ctx)
		},
	}
	appBuilt := false
	defer func() {
		if appBuilt {
			return
		}
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		for _, closeFn := range closeFns {
			if closeFn != nil {
				_ = closeFn(shutdownCtx)
			}
		}
	}()

	routerStart := time.Now()
	logger.Info("Setting up Chi router with middleware stack")

	handler := buildRootMux(rootDependencies{
		Weave:          weaveStore,
		Pool:           opts.Pool,
		Logger:         logger,
		DevMode:        opts.DevMode,
		LogStaticFiles: opts.LogStaticFiles,
		AllowedOrigins: opts.AllowedOrigins,
		Session:        sessionManager,
		StaticAssets:   opts.Contributions.StaticAssets,
		ObsMiddleware:  obs.Middleware,
	})

	authHandler := weaveauth.NewAuthHandler(logger, weaveStore, sessionManager)
	handler.Group(func(r chi.Router) {
		r.Use(authRateLimit)
		weaveauth.MountAPIRoutes(r, authHandler, opts.RegistrationEnabled, opts.SSOLoginURL != "")
	})
	admin.Mount(handler, admin.Host{
		Logger:    logger,
		Templates: templateRenderer,
		I18n:      i18nManager,
		Session:   sessionManager,
		Sections:  opts.Contributions.AdminSections,
	})

	ontologyStore := ontologyslice.NewPostgresStore(opts.Pool)
	languages := weaveLanguages(i18nManager)
	langResolver := weaveLangResolver(sessionManager)
	changeLog := weave.NewChangeLogRunner(weaveStore)
	ontologyBus := domain.NewSimpleEventBus()
	ontologyReader := sliceOntologyReader{store: ontologyStore}
	ontologyVersionReader := sliceOntologyVersionReader{store: ontologyStore}
	generatorRenderers := opts.GeneratorRenderers
	if len(generatorRenderers) == 0 {
		generatorRenderers = genwiring.CoreRenderers()
	}
	// formatAvailable mirrors the registered renderer set so detailview only
	// advertises a derivative URL when a renderer can actually produce it. A
	// core-only build lacks shacl/arches/cytoscape renderers, so those URLs
	// are omitted instead of 500'ing when clicked.
	formatAvailable := make(map[generators.Format]bool, len(generatorRenderers))
	for _, r := range generatorRenderers {
		formatAvailable[r.Spec().Format] = true
	}
	hasFormat := func(f generators.Format) bool { return formatAvailable[f] }
	organizationHost, orgMembersHost := buildOrganizationHosts(opts.Pool, logger, languages, langResolver)
	projectHost := buildProjectHost(opts.Pool, weaveStore, logger, changeLog, languages, langResolver)
	workspaceHost := buildWorkspaceHost(logger, templateRenderer, i18nManager, sessionManager, languages, langResolver, organizationHost, projectHost, orgMembersHost)
	authPagesHost := buildAuthPagesHost(logger, templateRenderer, i18nManager, sessionManager, langResolver, opts.RegistrationEnabled, opts.SSOLoginURL)
	projectPagesHost := buildProjectPagesHost(logger, templateRenderer, weaveStore, i18nManager, sessionManager)
	searchHost := buildSearchHost(weaveStore, logger)
	vocabularyHost := buildVocabularyHost(opts.Pool, weaveStore, logger, languages, langResolver)
	entitySchemaHost := buildEntitySchemaHost(weaveStore, logger, languages, langResolver, i18nManager, organizationHost)
	projectPageHost := buildProjectPageHost(opts.Pool, weaveStore, logger, changeLog, languages, langResolver, i18nManager, ontologyReader, ontologyVersionReader)
	draftsHost := buildDraftsHost(weaveStore, logger)
	settingsHost := buildSettingsHost(opts.Pool, weaveStore, logger, languages)
	ontologySvc, ontologyAdminHost, ontologyAPIHost, ontologyPagesHost := buildOntologyHosts(
		ontologyStore,
		weaveStore,
		logger,
		templateRenderer,
		i18nManager,
		sessionManager,
		languages,
		langResolver,
		ontologyBus,
		opts.AutocompleteMode,
		opts.AutocompleteAllowOverride,
	)
	materializer.WithOntologyImporter(NewOntologyVendorImporter(ontologySvc))
	if err := gitMatService.Start(ctx); err != nil {
		logger.Warn("git materializer failed to start", "err", err)
	}
	membersHost := buildMembersHost(opts.Pool, logger, languages, langResolver)
	releaseHost := buildReleaseHost(opts.Pool, logger, languages, langResolver)
	exampleHost := buildExampleHost(opts.Pool, weaveStore, logger, languages, langResolver)
	namespaceBindingHost, namespaceSvc := buildNamespaceBindingHost(opts.Pool, logger, changeLog, languages, langResolver)
	projectOntologyVersionHost := buildProjectOntologyVersionHost(
		opts.Pool,
		weaveStore,
		logger,
		changeLog,
		languages,
		langResolver,
		ontologyReader,
		ontologyVersionReader,
		ontologyBus,
	)
	fieldHost, modelHost, collectionHost, overrideSvc := buildCoreEntityHosts(coreEntityDeps{
		Pool:      opts.Pool,
		Weave:     weaveStore,
		Logger:    logger,
		ChangeLog: changeLog,
		Languages: languages,
		I18n:      i18nManager,
	}, langResolver)
	projectCSVExportHost, exportsHost := buildExportHosts(
		weaveStore,
		logger,
		i18nManager,
		sessionManager,
		projectHost,
		fieldHost,
		modelHost,
		collectionHost,
		namespaceSvc,
		ontologyReader,
		ontologyVersionReader,
	)
	generatorService := buildGeneratorService(
		logger,
		projectHost,
		fieldHost,
		modelHost,
		collectionHost,
		namespaceSvc,
		generatorRenderers,
	)
	visualizationHost := buildVisualizationHost(opts.Pool, weaveStore, logger, generatorService)
	detailViewHost := buildDetailViewHost(opts.Pool, logger, templateRenderer, weaveStore, i18nManager, sessionManager, opts.IntegrationRegistry, ontologySvc, hasFormat)
	categoryHost, categoryService := buildCategoryHost(opts.Pool, weaveStore, logger, changeLog, languages, langResolver)
	weaverouter.Mount(handler, buildProjectMiddlewareHost(weaveStore), buildErrorPageHost(templateRenderer, i18nManager, langResolver), weaverouter.Options{
		Integrations: opts.IntegrationRegistry,
		ActorAdmin:   buildActorAdminHost(opts.Pool, weaveStore, logger, languages, langResolver),
		APIKey: apikey.Host{
			Service:      apikeyService,
			Logger:       logger,
			Languages:    languages,
			LangResolver: langResolver,
		},
		Attribution:            buildAttributionHost(opts.Pool, weaveStore, logger, languages),
		AuthPages:              authPagesHost,
		Category:               categoryHost,
		Collection:             collectionHost,
		DetailView:             detailViewHost,
		Drafts:                 draftsHost,
		EntitySchema:           entitySchemaHost,
		ErrorTracking:          buildErrorTrackingHost(opts.Pool, logger),
		MaterializationAdmin:   materializationadmin.Host{Pool: opts.Pool, Logger: logger},
		Example:                exampleHost,
		Exports:                exportsHost,
		Field:                  fieldHost,
		GitRestoreAdmin:        buildGitRestoreAdminHost(opts.Pool, logger, ontologySvc),
		Health:                 buildHealthHost(opts.Pool, logger),
		Members:                membersHost,
		Model:                  modelHost,
		NamespaceBinding:       namespaceBindingHost,
		OntologyAdmin:          ontologyAdminHost,
		OntologyAPI:            ontologyAPIHost,
		OntologyPages:          ontologyPagesHost,
		OntologyService:        ontologySvc,
		Organization:           organizationHost,
		OrgMembers:             orgMembersHost,
		Project:                projectHost,
		ProjectCSVExport:       projectCSVExportHost,
		ProjectOntologyVersion: projectOntologyVersionHost,
		ProjectPages:           projectPagesHost,
		ProjectPage:            projectPageHost,
		Release:                releaseHost,
		Search:                 searchHost,
		Settings:               settingsHost,
		Vocabulary:             vocabularyHost,
		Version:                buildVersionHost(opts.Pool, logger, opts.Instance),
		Visualization:          visualizationHost,
		Workspace:              workspaceHost,
	})
	mountIntegrationsHub(handler, weaveStore, logger, opts, generatorService)
	mountRouteContributions(handler, opts.Contributions.Routes, Host{
		Pool:                opts.Pool,
		Logger:              logger,
		Templates:           templateRenderer,
		I18n:                i18nManager,
		Session:             sessionManager,
		IntegrationRegistry: opts.IntegrationRegistry,
		IntegrationCipher:   opts.IntegrationCipher,
		Services: &Services{
			Weave:             weaveStore,
			APIKeys:           apikeyService,
			Projects:          projectHost.Service,
			ProjectOntologies: projectOntologyVersionHost.Service,
			Namespaces:        namespaceSvc,
			Fields:            fieldHost.Service,
			Models:            modelHost.Service,
			Collections:       collectionHost.Service,
			Categories:        categoryService,
			Ontology:          ontologySvc,
			Vocabulary:        vocabularyHost.Service,
			Override:          overrideSvc,
			PathAudit:         pathAuditSvc,
			Languages:         languages,
			Obs:               obs,
		},
	})

	if err := weavecontent.MountWithSources(handler, buildContentHost(logger, templateRenderer, i18nManager, langResolver), contentSources); err != nil {
		return nil, fmt.Errorf("mount content pages: %w", err)
	}

	logger.Info("Chi router configured with middleware and routes", "duration", time.Since(routerStart).String())

	appBuilt = true
	return &App{
		Handler:   handler,
		Weave:     weaveStore,
		I18n:      i18nManager,
		Session:   sessionManager,
		Templates: templateRenderer,
		closeFns:  closeFns,
	}, nil
}

func mountIntegrationsHub(parent chi.Router, weaveStore domain.WeaveStore, logger *slog.Logger, opts Options, gens *generators.Service) {
	if opts.IntegrationRegistry == nil {
		return
	}
	sub := chi.NewMux()
	var artifacts hub.ProjectArtifactProviderFactory
	if opts.ProjectArtifactProviderFactory != nil {
		artifactCtx := integrations.ProjectArtifactContext{
			Pool:       opts.Pool,
			Weave:      weaveStore,
			Logger:     logger,
			Generators: gens,
		}
		artifacts = func(ctx context.Context, projectID string) registry.ProjectArtifactProvider {
			return opts.ProjectArtifactProviderFactory(ctx, artifactCtx, projectID)
		}
	}
	hub.Mount(sub, hub.Host{
		Pool:                           opts.Pool,
		Weave:                          weaveStore,
		Logger:                         logger,
		Registry:                       opts.IntegrationRegistry,
		Cipher:                         opts.IntegrationCipher,
		Generators:                     gens,
		ManagedConfigResolver:          opts.ManagedConfigResolver,
		ManagedLabelResolver:           opts.ManagedLabelResolver,
		ManagedConflictCheck:           opts.ManagedConflictCheck,
		ProjectArtifactProviderFactory: artifacts,
	})
	parent.Mount("/projects/{projectID}/integrations", sub)
}

func mountRouteContributions(parent chi.Router, routes []RouteContribution, host Host) {
	for _, route := range routes {
		route.Mount(parent, host)
	}
}

// Close stops services owned by the app. It is safe to call multiple times.
func (a *App) Close(ctx context.Context) error {
	if a == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var firstErr error
	for _, closeFn := range a.closeFns {
		if closeFn == nil {
			continue
		}
		if err := closeFn(ctx); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	a.closeFns = nil
	return firstErr
}
