package app

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/formschema"
	"github.com/pletka-io/pletka/pkg/i18n"
	"github.com/pletka-io/pletka/pkg/integrations/hub"
	integrationsregistry "github.com/pletka-io/pletka/pkg/integrations/registry"
	"github.com/pletka-io/pletka/pkg/service/gitmaterializer"
	"github.com/pletka-io/pletka/pkg/session"
	weavepkg "github.com/pletka-io/pletka/pkg/weave"
	"github.com/pletka-io/pletka/pkg/weave/actoradmin"
	"github.com/pletka-io/pletka/pkg/weave/actorlabels"
	"github.com/pletka-io/pletka/pkg/weave/attribution"
	"github.com/pletka-io/pletka/pkg/weave/authpages"
	"github.com/pletka-io/pletka/pkg/weave/category"
	"github.com/pletka-io/pletka/pkg/weave/collection"
	weavecontent "github.com/pletka-io/pletka/pkg/weave/content"
	weavedetailview "github.com/pletka-io/pletka/pkg/weave/detailview"
	"github.com/pletka-io/pletka/pkg/weave/drafts"
	"github.com/pletka-io/pletka/pkg/weave/entityschema"
	"github.com/pletka-io/pletka/pkg/weave/errortracking"
	"github.com/pletka-io/pletka/pkg/weave/example"
	weaveexports "github.com/pletka-io/pletka/pkg/weave/exports"
	"github.com/pletka-io/pletka/pkg/weave/field"
	"github.com/pletka-io/pletka/pkg/weave/generators"
	weavecsv "github.com/pletka-io/pletka/pkg/weave/generators/csv"
	"github.com/pletka-io/pletka/pkg/weave/gitrestoreadmin"
	"github.com/pletka-io/pletka/pkg/weave/health"
	"github.com/pletka-io/pletka/pkg/weave/members"
	"github.com/pletka-io/pletka/pkg/weave/model"
	"github.com/pletka-io/pletka/pkg/weave/namespacebinding"
	weaveontology "github.com/pletka-io/pletka/pkg/weave/ontology"
	"github.com/pletka-io/pletka/pkg/weave/ontology/autocomplete"
	"github.com/pletka-io/pletka/pkg/weave/organization"
	"github.com/pletka-io/pletka/pkg/weave/orgmembers"
	overridepkg "github.com/pletka-io/pletka/pkg/weave/override"
	weavepages "github.com/pletka-io/pletka/pkg/weave/pages"
	"github.com/pletka-io/pletka/pkg/weave/project"
	weavecsvexport "github.com/pletka-io/pletka/pkg/weave/project/csvexport"
	"github.com/pletka-io/pletka/pkg/weave/projectontologyversion"
	"github.com/pletka-io/pletka/pkg/weave/projectpage"
	"github.com/pletka-io/pletka/pkg/weave/publication"
	"github.com/pletka-io/pletka/pkg/weave/release"
	weaverouter "github.com/pletka-io/pletka/pkg/weave/router"
	"github.com/pletka-io/pletka/pkg/weave/search"
	"github.com/pletka-io/pletka/pkg/weave/settings"
	weavetemplates "github.com/pletka-io/pletka/pkg/weave/templates"
	"github.com/pletka-io/pletka/pkg/weave/version"
	"github.com/pletka-io/pletka/pkg/weave/visualization"
	"github.com/pletka-io/pletka/pkg/weave/vocabconnector/registry"
	"github.com/pletka-io/pletka/pkg/weave/vocabulary"
	"github.com/pletka-io/pletka/pkg/weave/workspace"
)

func buildAttributionHost(pool *pgxpool.Pool, weave domain.WeaveStore, logger *slog.Logger, languages []formschema.LanguageInfo) attribution.Host {
	return attribution.Host{
		Store:     attribution.NewPostgresStore(pool),
		Projects:  weave.Projects(),
		Logger:    logger,
		Languages: languages,
	}
}

func buildActorAdminHost(
	pool *pgxpool.Pool,
	weave domain.WeaveStore,
	logger *slog.Logger,
	languages []formschema.LanguageInfo,
	langResolver actoradmin.LangResolver,
) actoradmin.Host {
	return actoradmin.Host{
		Store:        actoradmin.NewPostgresStore(pool),
		Auth:         weave.Auth(),
		Logger:       logger,
		Languages:    languages,
		LangResolver: langResolver,
	}
}

func buildErrorTrackingHost(pool *pgxpool.Pool, logger *slog.Logger) errortracking.Host {
	return errortracking.Host{
		Pool:   pool,
		Logger: logger,
	}
}

func buildHealthHost(pool *pgxpool.Pool, logger *slog.Logger) health.Host {
	return health.Host{
		Pool:   pool,
		Logger: logger,
	}
}

func buildVersionHost(pool *pgxpool.Pool, logger *slog.Logger, instance string) version.Host {
	return version.Host{
		Pool:     pool,
		Logger:   logger,
		Instance: instance,
	}
}

func buildGitRestoreAdminHost(pool *pgxpool.Pool, logger *slog.Logger, ontologySvc *weaveontology.Service) gitrestoreadmin.Host {
	mat := gitmaterializer.NewMaterializer(pool, "", logger).WithOntologyImporter(NewOntologyVendorImporter(ontologySvc))
	return gitrestoreadmin.Host{
		Store:  gitrestoreadmin.NewPostgresStore(pool),
		Runner: gitrestoreadmin.NewMaterializerRunner(mat),
		Logger: logger,
	}
}

// buildCategoryHost returns the category.Host used for HTTP routes plus the
// underlying *category.Service — the Host wires the raw Store directly (a
// pre-existing category-slice quirk, not introduced here), so callers that
// need the Service layer (e.g. hosting-repo modules consuming it via the
// services-out seam, ADR-0008) get it as a second return value instead of
// constructing their own instance.
func buildCategoryHost(
	pool *pgxpool.Pool,
	weave domain.WeaveStore,
	logger *slog.Logger,
	changeLog domain.ChangeLogRunner,
	languages []formschema.LanguageInfo,
	langResolver category.LangResolver,
) (category.Host, *category.Service) {
	store := category.NewPostgresStore(pool)
	svc := category.NewService(store, weave.Adoptions(), logger, changeLog, weave)
	return category.Host{
		Store:       store,
		SchemaStore: weave.WeaveCategories(),
		Adoptions:   weave.Adoptions(),
		Numberer:    weave,
		OriginResolver: category.OriginResolverFunc(func(ctx context.Context, projectID string, categories []*domain.Category) (map[string]domain.Origin, error) {
			return weavepkg.CategoryAdoptionOriginsBySystemName(ctx, weave, projectID, categories)
		}),
		Logger:       logger,
		ChangeLog:    changeLog,
		Languages:    languages,
		LangResolver: langResolver,
	}, svc
}

// buildNamespaceBindingHost returns the namespacebinding.Host used for HTTP
// routes plus the underlying *namespacebinding.Service — callers that need
// the Service layer (the CSV export generators and hosting-repo modules
// consuming it via the services-out seam, ADR-0008) get it as a second
// return value instead of constructing their own instance (same
// pattern as buildCategoryHost).
func buildNamespaceBindingHost(
	pool *pgxpool.Pool,
	logger *slog.Logger,
	changeLog domain.ChangeLogRunner,
	languages []formschema.LanguageInfo,
	langResolver namespacebinding.LangResolver,
) (namespacebinding.Host, *namespacebinding.Service) {
	store := namespacebinding.NewPostgresStore(pool)
	svc := namespacebinding.NewService(store, logger, changeLog)
	return namespacebinding.Host{
		Store:        store,
		Logger:       logger,
		ChangeLog:    changeLog,
		Languages:    languages,
		LangResolver: langResolver,
	}, svc
}

func buildOrganizationHosts(
	pool *pgxpool.Pool,
	logger *slog.Logger,
	languages []formschema.LanguageInfo,
	langResolver organization.LangResolver,
) (organization.Host, orgmembers.Host) {
	orgSvc := organization.NewService(organization.NewPostgresStore(pool), pool, logger)
	return organization.Host{
			Service:      orgSvc,
			Logger:       logger,
			Languages:    languages,
			LangResolver: langResolver,
		}, orgmembers.Host{
			Service:      orgmembers.NewService(orgmembers.NewPostgresStore(pool), logger),
			OrgReader:    orgSvc,
			Logger:       logger,
			Languages:    languages,
			LangResolver: orgmembers.LangResolver(langResolver),
		}
}

func buildProjectHost(
	pool *pgxpool.Pool,
	weave domain.WeaveStore,
	logger *slog.Logger,
	changeLog domain.ChangeLogRunner,
	languages []formschema.LanguageInfo,
	langResolver project.LangResolver,
) project.Host {
	return project.Host{
		Service:             project.NewService(project.NewPostgresStore(pool), weave.Projects(), weave.Memberships(), vocabulary.NewService(pool, registry.New(http.DefaultClient)), logger),
		Overrides:           overridepkg.NewService(overridepkg.NewPostgresStore(pool), logger, changeLog),
		Weave:               weave,
		Logger:              logger,
		Languages:           languages,
		LangResolver:        langResolver,
		ConceptValueChecker: vocabulary.NewService(pool, nil), // set_value must be a control-list member (#3599)
	}
}

func buildWorkspaceHost(
	logger *slog.Logger,
	templates *weavetemplates.Renderer,
	i18nManager i18n.Manager,
	sessionManager *session.Manager,
	languages []formschema.LanguageInfo,
	langResolver workspace.LangResolver,
	organizationHost organization.Host,
	projectHost project.Host,
	orgMembersHost orgmembers.Host,
) workspace.Host {
	return workspace.Host{
		Logger:        logger,
		Templates:     templates,
		I18n:          i18nManager,
		Session:       sessionManager,
		LangResolver:  langResolver,
		Languages:     languages,
		Organizations: organizationHost.Service,
		Projects:      projectHost.Service,
		Members:       orgMembersHost.Service,
	}
}

func buildAuthPagesHost(
	logger *slog.Logger,
	templates *weavetemplates.Renderer,
	i18nManager i18n.Manager,
	sessionManager *session.Manager,
	langResolver authpages.LangResolver,
	registrationEnabled bool,
	ssoLoginURL string,
) authpages.Host {
	return authpages.Host{
		Logger:              logger,
		Templates:           templates,
		I18n:                i18nManager,
		Session:             sessionManager,
		LangResolver:        langResolver,
		RegistrationEnabled: registrationEnabled,
		SSOLoginURL:         ssoLoginURL,
	}
}

func buildProjectMiddlewareHost(
	weave domain.WeaveStore,
	publicationReader *publication.Reader,
) weaverouter.ProjectMiddlewareHost {
	return weaverouter.ProjectMiddlewareHost{
		Weave:         weave,
		LatestRelease: publicationReader,
	}
}

func buildErrorPageHost(
	templates *weavetemplates.Renderer,
	i18nManager i18n.Manager,
	langResolver func(*http.Request) string,
) weaverouter.ErrorPageHost {
	return weaverouter.ErrorPageHost{
		Templates:    templates,
		I18n:         i18nManager,
		LangResolver: langResolver,
	}
}

func buildProjectPagesHost(
	logger *slog.Logger,
	templates *weavetemplates.Renderer,
	weave domain.WeaveStore,
	i18nManager i18n.Manager,
	sessionManager *session.Manager,
	publicationReader *publication.Reader,
) weavepages.Host {
	return weavepages.Host{
		Logger:        logger,
		Renderer:      templates,
		Weave:         weave,
		I18n:          i18nManager,
		Session:       sessionManager,
		LatestRelease: publicationReader,
	}
}

func buildDetailViewHost(
	pool *pgxpool.Pool,
	logger *slog.Logger,
	templates *weavetemplates.Renderer,
	weave domain.WeaveStore,
	i18nManager i18n.Manager,
	sessionManager *session.Manager,
	integrations *integrationsregistry.Registry,
	preloader weavedetailview.AutocompletePreloader,
	hasFormat func(generators.Format) bool,
	publicationReader *publication.Reader,
) weavedetailview.Host {
	return weavedetailview.Host{
		Logger:       logger,
		Weave:        weave,
		I18n:         i18nManager,
		Session:      sessionManager,
		Renderer:     templates,
		Actors:       actorlabels.NewPostgresReader(pool),
		Integrations: integrations,
		IntLookup:    detailViewIntegrationsLookup{store: hub.NewPostgresStore(pool)},
		Preloader:    preloader,
		Publication:  publicationReader,
		HasFormat:    hasFormat,
	}
}

// detailViewIntegrationsLookup adapts the integrations hub Store to the narrow
// surface the detailview slice needs, keeping the dependency on the hub slice in
// app composition rather than router or detailview.
type detailViewIntegrationsLookup struct {
	store hub.Store
}

func (l detailViewIntegrationsLookup) EnabledForProject(ctx context.Context, projectID string) ([]weavedetailview.EnabledIntegrationConfig, error) {
	rows, err := l.store.ListEnabledForProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	out := make([]weavedetailview.EnabledIntegrationConfig, 0, len(rows))
	for _, r := range rows {
		out = append(out, weavedetailview.EnabledIntegrationConfig{
			IntegrationID: r.IntegrationID,
			ConfigID:      r.ConfigID,
			Label:         r.Label,
		})
	}
	return out, nil
}

func buildContentHost(
	logger *slog.Logger,
	templates *weavetemplates.Renderer,
	i18nManager i18n.Manager,
	langResolver func(*http.Request) string,
) weavecontent.Host {
	return weavecontent.Host{
		Logger:       logger,
		Templates:    templates,
		I18n:         i18nManager,
		LangResolver: langResolver,
	}
}

func buildSearchHost(weave domain.WeaveStore, logger *slog.Logger, publicationReader *publication.Reader) search.Host {
	return search.Host{
		Weave:         weave,
		Logger:        logger,
		LatestRelease: publicationReader,
	}
}

func buildVocabularyHost(
	pool *pgxpool.Pool,
	weave domain.WeaveStore,
	logger *slog.Logger,
	languages []formschema.LanguageInfo,
	langResolver vocabulary.LangResolver,
) vocabulary.Host {
	return vocabulary.Host{
		Service:      vocabulary.NewService(pool, registry.New(http.DefaultClient), weave),
		Projects:     weave.Projects(),
		Logger:       logger,
		Languages:    languages,
		LangResolver: langResolver,
	}
}

func buildEntitySchemaHost(
	weave domain.WeaveStore,
	logger *slog.Logger,
	languages []formschema.LanguageInfo,
	langResolver entityschema.LangResolver,
	i18nManager i18n.Manager,
	organizationHost organization.Host,
	publicationReader *publication.Reader,
) entityschema.Host {
	return entityschema.Host{
		Logger:        logger,
		Weave:         weave,
		Organizations: organizationHost.Service,
		Languages:     languages,
		LangResolver:  langResolver,
		I18n:          i18nManager,
		LatestRelease: publicationReader,
	}
}

func buildProjectPageHost(
	pool *pgxpool.Pool,
	weave domain.WeaveStore,
	logger *slog.Logger,
	changeLog domain.ChangeLogRunner,
	languages []formschema.LanguageInfo,
	langResolver projectpage.LangResolver,
	i18nManager i18n.Manager,
	ontologyReader projectontologyversion.OntologyReader,
	ontologyVersionReader projectontologyversion.OntologyVersionReader,
	publicationReader *publication.Reader,
	examplesMaxNestingDepth int,
) projectpage.Host {
	povSvc := projectontologyversion.NewService(
		projectontologyversion.NewPostgresStore(pool),
		weave.Projects(),
		ontologyReader,
		ontologyVersionReader,
		logger,
		changeLog,
	)
	return projectpage.Host{
		Logger:           logger,
		Weave:            weave,
		LinkedOntologies: povSvc,
		Releases:         release.NewService(release.NewPostgresStore(pool), pool, logger),
		Examples:         example.NewService(example.NewPostgresStore(pool), weave, example.WithMaxNestingDepth(examplesMaxNestingDepth)),
		Attributions:     attribution.NewPostgresStore(pool),
		ActorLabels:      actorlabels.NewPostgresReader(pool),
		Languages:        languages,
		LangResolver:     langResolver,
		I18n:             i18nManager,
		Publication:      publicationReader,
	}
}

func buildProjectOntologyVersionHost(
	pool *pgxpool.Pool,
	weave domain.WeaveStore,
	logger *slog.Logger,
	changeLog domain.ChangeLogRunner,
	languages []formschema.LanguageInfo,
	langResolver projectontologyversion.LangResolver,
	ontologyReader projectontologyversion.OntologyReader,
	ontologyVersionReader projectontologyversion.OntologyVersionReader,
	eventBus domain.EventBus,
) projectontologyversion.Host {
	svc := projectontologyversion.NewService(
		projectontologyversion.NewPostgresStore(pool),
		weave.Projects(),
		ontologyReader,
		ontologyVersionReader,
		logger,
		changeLog,
	)
	if eventBus != nil {
		svc.WithEventBus(eventBus)
	}
	return projectontologyversion.Host{
		Service:      svc,
		Logger:       logger,
		Languages:    languages,
		LangResolver: langResolver,
	}
}

func buildOntologyHosts(
	store weaveontology.Store,
	weave domain.WeaveStore,
	logger *slog.Logger,
	templates *weavetemplates.Renderer,
	i18nManager i18n.Manager,
	sessionManager *session.Manager,
	languages []formschema.LanguageInfo,
	langResolver func(*http.Request) string,
	eventBus domain.EventBus,
	autocompleteMode string,
	autocompleteAllowOverride bool,
) (*weaveontology.Service, weaveontology.AdminHost, weaveontology.APIHost, weaveontology.PagesHost) {
	svc := weaveontology.NewService(store, weave.Projects(), logger)
	if autocompleteMode != "" {
		svc.WithDispatchConfig(autocomplete.DispatchConfig{
			Mode:                    autocomplete.Mode(autocompleteMode),
			AllowPerRequestOverride: autocompleteAllowOverride,
		})
	}
	svc.WireEventBus(eventBus)
	return svc, weaveontology.AdminHost{
			Service:      svc,
			Logger:       logger,
			Templates:    templates,
			I18n:         i18nManager,
			Session:      sessionManager,
			Languages:    languages,
			LangResolver: langResolver,
		}, weaveontology.APIHost{
			Service:      svc,
			Logger:       logger,
			Languages:    languages,
			LangResolver: langResolver,
			Projects:     weave.Projects(),
		}, weaveontology.PagesHost{
			Service:   svc,
			Logger:    logger,
			Templates: templates,
			I18n:      i18nManager,
			Session:   sessionManager,
		}
}

func buildDraftsHost(weave domain.WeaveStore, logger *slog.Logger) drafts.Host {
	return drafts.Host{
		Weave:  weave,
		Logger: logger,
	}
}

func buildSettingsHost(
	pool *pgxpool.Pool,
	weave domain.WeaveStore,
	logger *slog.Logger,
	languages []formschema.LanguageInfo,
) settings.Host {
	return settings.Host{
		Weave:     weave,
		Store:     settings.NewPostgresStore(pool),
		Logger:    logger,
		Languages: languages,
	}
}

type coreEntityDeps struct {
	Pool      *pgxpool.Pool
	Weave     domain.WeaveStore
	Logger    *slog.Logger
	ChangeLog domain.ChangeLogRunner
	Languages []formschema.LanguageInfo
	I18n      i18n.Manager
}

func buildCoreEntityHosts(
	deps coreEntityDeps,
	langResolver func(*http.Request) string,
) (field.Host, model.Host, collection.Host, *overridepkg.Service) {
	overrideStore := overridepkg.NewPostgresStore(deps.Pool)
	overrideSvc := overridepkg.NewService(overrideStore, deps.Logger, deps.ChangeLog)
	projects := deps.Weave.Projects()
	categories := deps.Weave.WeaveCategories()

	ontologySvc := weaveontology.NewService(
		weaveontology.NewPostgresStore(deps.Pool),
		projects,
		deps.Logger,
	)

	fieldSvc := field.NewService(
		field.NewPostgresStore(deps.Pool),
		overrideStore,
		deps.Weave.Adoptions(),
		deps.Weave.Forks(),
		projects,
		projects,
		deps.Weave,
		ontologySvc,
		deps.Logger,
		deps.ChangeLog,
	)
	modelSvc := model.NewService(
		model.NewPostgresStore(deps.Pool),
		overrideSvc,
		deps.Weave.Adoptions(),
		deps.Weave.Forks(),
		projects,
		projects,
		deps.Weave,
		deps.Weave,
		categories,
		deps.Logger,
		deps.ChangeLog,
	)
	collectionSvc := collection.NewService(
		collection.NewPostgresStore(deps.Pool),
		overrideSvc,
		deps.Weave.Adoptions(),
		deps.Weave.Forks(),
		projects,
		projects,
		deps.Weave,
		deps.Weave,
		categories,
		deps.Logger,
		deps.ChangeLog,
	)

	pub := publication.NewReader(deps.Pool)
	return field.Host{
			Service:      fieldSvc,
			Projects:     projects,
			Logger:       deps.Logger,
			Languages:    deps.Languages,
			LangResolver: field.LangResolver(langResolver),
			I18n:         deps.I18n,
			Publication:  pub,
		}, model.Host{
			Service:      modelSvc,
			Projects:     projects,
			Logger:       deps.Logger,
			Languages:    deps.Languages,
			LangResolver: model.LangResolver(langResolver),
			I18n:         deps.I18n,
			Publication:  pub,
		}, collection.Host{
			Service:      collectionSvc,
			Projects:     projects,
			Logger:       deps.Logger,
			Languages:    deps.Languages,
			LangResolver: collection.LangResolver(langResolver),
			I18n:         deps.I18n,
			Publication:  pub,
		}, overrideSvc
}

func buildExportHosts(
	weave domain.WeaveStore,
	logger *slog.Logger,
	i18nManager i18n.Manager,
	sessionManager *session.Manager,
	projectHost project.Host,
	fieldHost field.Host,
	modelHost model.Host,
	collectionHost collection.Host,
	namespaceSvc *namespacebinding.Service,
	ontologyReader weavecsvexport.OntologyLookup,
	ontologyVersionReader weavecsvexport.OntologyVersionLookup,
) (weavecsvexport.Host, weaveexports.Host) {
	csvSvc, err := weavecsvexport.NewService(
		weave,
		ontologyReader,
		ontologyVersionReader,
		i18nManager,
		sessionManager,
		logger,
	)
	if err != nil {
		panic(err)
	}

	reg, err := generators.NewRegistry(weavecsv.NewRenderer())
	if err != nil {
		panic(err)
	}
	exportSvc := generators.NewService(
		projectHost.Service,
		modelHost.Service,
		collectionHost.Service,
		fieldHost.Service,
		namespaceSvc,
		reg,
	)

	return weavecsvexport.Host{
			Service: csvSvc,
		}, weaveexports.Host{
			Service:  exportSvc,
			Projects: weave.Projects(),
			Logger:   logger,
		}
}

func buildGeneratorService(
	logger *slog.Logger,
	projectHost project.Host,
	fieldHost field.Host,
	modelHost model.Host,
	collectionHost collection.Host,
	namespaceSvc *namespacebinding.Service,
	renderers []generators.Renderer,
) *generators.Service {
	reg, err := generators.NewRegistry(renderers...)
	if err != nil {
		panic(err)
	}
	return generators.NewService(
		projectHost.Service,
		modelHost.Service,
		collectionHost.Service,
		fieldHost.Service,
		namespaceSvc,
		reg,
	)
}

func buildVisualizationHost(
	pool *pgxpool.Pool,
	weave domain.WeaveStore,
	logger *slog.Logger,
	gens *generators.Service,
	publicationReader *publication.Reader,
) visualization.Host {
	return visualization.Host{
		Generators:    gens,
		Weave:         weave,
		Bundles:       projectontologyversion.NewPostgresStore(pool),
		Logger:        logger,
		LatestRelease: publicationReader,
	}
}

func buildMembersHost(
	pool *pgxpool.Pool,
	logger *slog.Logger,
	languages []formschema.LanguageInfo,
	langResolver members.LangResolver,
) members.Host {
	return members.Host{
		Service:      members.NewService(members.NewPostgresStore(pool), logger),
		Logger:       logger,
		Languages:    languages,
		LangResolver: langResolver,
	}
}

func buildReleaseHost(
	pool *pgxpool.Pool,
	logger *slog.Logger,
	languages []formschema.LanguageInfo,
	langResolver release.LangResolver,
) release.Host {
	return release.Host{
		Service:      release.NewService(release.NewPostgresStore(pool), pool, logger),
		Logger:       logger,
		Languages:    languages,
		LangResolver: langResolver,
	}
}

func buildExampleHost(
	pool *pgxpool.Pool,
	weave domain.WeaveStore,
	logger *slog.Logger,
	languages []formschema.LanguageInfo,
	langResolver example.LangResolver,
	examplesMaxNestingDepth int,
) example.Host {
	return example.Host{
		Service:      example.NewService(example.NewPostgresStore(pool), weave, example.WithMaxNestingDepth(examplesMaxNestingDepth)),
		Projects:     weave.Projects(),
		Logger:       logger,
		Languages:    languages,
		LangResolver: langResolver,
	}
}
