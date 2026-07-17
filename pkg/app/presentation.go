package app

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"

	"github.com/pletka-io/pletka/pkg/assets"
	"github.com/pletka-io/pletka/pkg/frontendmanifest"
	"github.com/pletka-io/pletka/pkg/i18n"
	i18nbootstrap "github.com/pletka-io/pletka/pkg/i18n/bootstrap"
	weavecontent "github.com/pletka-io/pletka/pkg/weave/content"
	weavetemplates "github.com/pletka-io/pletka/pkg/weave/templates"
)

// PresentationOptions configures static presentation setup shared by app.New
// and serve --validate-only.
type PresentationOptions struct {
	Logger *slog.Logger

	DevMode                  bool
	ContentOverlayPath       string
	FrontendManifestPaths    []string
	CoreFrontendManifestPath string
	StaticAssets             []StaticAssetSet
	FrontendManifests        []FrontendManifestSet
	Analytics                weavetemplates.AnalyticsConfig
}

type presentationRuntime struct {
	I18n                  i18n.Manager
	Templates             *weavetemplates.Renderer
	ContentSources        []weavecontent.ContentSource
	FrontendManifestCount int
}

// ValidatePresentation validates static frontend, content, i18n, and template
// setup without requiring a database connection.
func ValidatePresentation(ctx context.Context, opts PresentationOptions) error {
	_, err := buildPresentation(ctx, opts)
	return err
}

func buildPresentation(ctx context.Context, opts PresentationOptions) (*presentationRuntime, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}

	logger.Info("Loading translations")
	i18nManager, err := i18nbootstrap.NewEmbeddedManager(logger)
	if err != nil {
		return nil, fmt.Errorf("initialize i18n manager: %w", err)
	}
	logger.Info("i18n manager initialized")

	logger.Info("Validating frontend manifests")
	islandResolver := assets.NewIslandResolverWithStaticFS(opts.DevMode, staticAssetFilesystemsFromSets(opts.StaticAssets)...)
	frontendManifestPaths := append([]string{}, opts.FrontendManifestPaths...)
	frontendManifestPaths = append(frontendManifestPaths, assets.FrontendManifestPathsFromEnv()...)
	frontendManifests, err := frontendManifestsFromSets(opts.FrontendManifests)
	if err != nil {
		return nil, err
	}
	frontendManifestValidation := assets.FrontendManifestValidation{
		CoreManifestPath:      opts.CoreFrontendManifestPath,
		PlatformManifestPaths: frontendManifestPaths,
		PlatformManifests:     frontendManifests,
		ValidateSourceFiles:   true,
	}
	if err := islandResolver.ValidateFrontendManifests(frontendManifestValidation); err != nil {
		return nil, fmt.Errorf("validate frontend manifests: %w", err)
	}

	frontendCatalog, err := assets.FrontendManifestCatalog(frontendManifestValidation)
	if err != nil {
		return nil, fmt.Errorf("build frontend manifest catalog: %w", err)
	}

	logger.Info("Validating content widget references")
	contentSources, err := weavecontent.ConfiguredSources(logger, opts.ContentOverlayPath)
	if err != nil {
		return nil, fmt.Errorf("configure content sources: %w", err)
	}
	if err := weavecontent.ValidateWidgetReferences(ctx, contentSources, frontendCatalog); err != nil {
		return nil, fmt.Errorf("validate content widget references: %w", err)
	}

	logger.Info("Validating weave templates")
	templateRenderer, err := weavetemplates.NewRenderer(islandResolver.IslandScriptTags, i18nManager, opts.Analytics)
	if err != nil {
		return nil, fmt.Errorf("initialize weave template renderer: %w", err)
	}

	return &presentationRuntime{
		I18n:                  i18nManager,
		Templates:             templateRenderer,
		ContentSources:        contentSources,
		FrontendManifestCount: len(frontendManifestPaths) + len(frontendManifests) + 1,
	}, nil
}

func staticAssetFilesystemsFromSets(sets []StaticAssetSet) []fs.FS {
	filesystems := make([]fs.FS, 0, len(sets))
	for _, set := range sets {
		if set.FS != nil {
			filesystems = append(filesystems, set.FS)
		}
	}
	return filesystems
}

func frontendManifestsFromSets(sets []FrontendManifestSet) ([]frontendmanifest.Manifest, error) {
	manifests := make([]frontendmanifest.Manifest, 0, len(sets))
	for _, set := range sets {
		body, err := fs.ReadFile(set.FS, set.Path)
		if err != nil {
			return nil, fmt.Errorf("read frontend manifest %s:%s: %w", set.ID, set.Path, err)
		}
		manifest, err := frontendmanifest.LoadBytes("embedded:"+set.ID+":"+set.Path, body, false)
		if err != nil {
			return nil, err
		}
		manifests = append(manifests, manifest)
	}
	return manifests, nil
}
