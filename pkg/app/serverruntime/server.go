package serverruntime

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"os/user"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/viper"

	"github.com/pletka-io/pletka/pkg/app"
	"github.com/pletka-io/pletka/pkg/app/cliruntime"
	"github.com/pletka-io/pletka/pkg/buildinfo"
	"github.com/pletka-io/pletka/pkg/database"
	"github.com/pletka-io/pletka/pkg/integrations"
	weavetemplates "github.com/pletka-io/pletka/pkg/weave/templates"
)

// Config captures the server runtime settings after CLI flags, env, config
// file values, and instance-derived overrides have been applied.
type Config struct {
	Host                string
	Port                string
	SocketPath          string
	Instance            string
	Env                 string
	ValidateOnly        bool
	SkipMigrations      bool
	LogStaticFiles      bool
	AllowedOrigins      []string
	RegistrationEnabled bool
	SSOLoginURL         string
	GitDataDir          string
	// ModuleHost/OntologyHost override the gitmaterializer module-path
	// namespace (git_materializer.module_host / .ontology_host). Empty falls
	// back to the public-clean defaults — see app.Options.ModuleHost.
	ModuleHost                string
	OntologyHost              string
	MetricsAddr               string
	AnalyticsEnabled          bool
	AnalyticsMatomoBaseURL    string
	AnalyticsMatomoSiteID     string
	AnalyticsTrackQueryString bool
	ContentOverlayPath        string
	FrontendManifestPaths     []string
	CoreFrontendManifestPath  string
	StaticAssets              []app.StaticAssetSet
	FrontendManifests         []app.FrontendManifestSet
	AutocompleteMode          string
	AutocompleteAllowOverride bool
	IntegrationSecretKey      string
	Database                  database.Settings

	// ConfigureApp lets the embedding binary extend app options —
	// integrations, generator renderers, route/admin contributions —
	// before app.New. nil means core-only wiring.
	ConfigureApp func(opts *app.Options, pool *pgxpool.Pool, cipher *integrations.Cipher) error

	// MigrateExtra runs the embedding binary's own migrations right after
	// core migrations, on the same connection. nil = none.
	MigrateExtra func(db *sql.DB) error
}

// Runtime owns resources created during server startup.
type Runtime struct {
	App   *app.App
	Pool  *pgxpool.Pool
	SQLDB *sql.DB
}

// parseAllowedOrigins normalizes cors.allowed_origins regardless of whether
// it came from a YAML list (viper.GetStringSlice returns one element per
// origin already) or a comma-separated env var override
// (PLETKA_CORS_ALLOWED_ORIGINS — viper.GetStringSlice on an env-sourced
// string splits on whitespace via cast.ToStringSliceE, so a comma-joined
// value arrives as a single element). Splitting every element on "," is a
// no-op for the YAML-list case and recovers the intended list for the env
// case; empty elements produced by trimming are dropped. Deduping happens
// later in mergeAllowedOrigins.
func parseAllowedOrigins(raw []string) []string {
	origins := make([]string, 0, len(raw))
	for _, entry := range raw {
		for _, part := range strings.Split(entry, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			origins = append(origins, part)
		}
	}
	return origins
}

// ConfigFromViper reads the current process configuration.
func ConfigFromViper() Config {
	return Config{
		Host:                      viper.GetString("server.host"),
		Port:                      viper.GetString("server.port"),
		SocketPath:                viper.GetString("server.socket"),
		Instance:                  viper.GetString("server.instance"),
		Env:                       viper.GetString("env"),
		ValidateOnly:              viper.GetBool("server.validate_only"),
		SkipMigrations:            viper.GetBool("server.skip_migrations"),
		LogStaticFiles:            viper.GetBool("log.static_files"),
		AllowedOrigins:            parseAllowedOrigins(viper.GetStringSlice("cors.allowed_origins")),
		RegistrationEnabled:       viper.GetBool("features.registration"),
		SSOLoginURL:               viper.GetString("auth.sso_login_url"),
		GitDataDir:                viper.GetString("git_materializer.data_dir"),
		ModuleHost:                viper.GetString("git_materializer.module_host"),
		OntologyHost:              viper.GetString("git_materializer.ontology_host"),
		MetricsAddr:               viper.GetString("observability.metrics_addr"),
		AnalyticsEnabled:          viper.GetBool("analytics.enabled"),
		AnalyticsMatomoBaseURL:    viper.GetString("analytics.matomo.base_url"),
		AnalyticsMatomoSiteID:     viper.GetString("analytics.matomo.site_id"),
		AnalyticsTrackQueryString: viper.GetBool("analytics.track_query_string"),
		ContentOverlayPath:        viper.GetString("content.overlay_path"),
		AutocompleteMode:          viper.GetString("autocomplete.mode"),
		AutocompleteAllowOverride: viper.GetBool("autocomplete.allow_per_request_override"),
		IntegrationSecretKey:      viper.GetString("integrations.secret_key"),
		Database:                  cliruntime.DatabaseSettingsFromViper().WithEnvOverrides(),
	}
}

// PresentationOptions maps server config to the static presentation validation
// config used by serve --validate-only.
func (cfg Config) PresentationOptions(logger *slog.Logger) app.PresentationOptions {
	return app.PresentationOptions{
		Logger:                   logger,
		DevMode:                  cfg.Env == "development",
		ContentOverlayPath:       cfg.ContentOverlayPath,
		FrontendManifestPaths:    cfg.FrontendManifestPaths,
		CoreFrontendManifestPath: cfg.CoreFrontendManifestPath,
		StaticAssets:             cfg.StaticAssets,
		FrontendManifests:        cfg.FrontendManifests,
	}
}

// ValidatePresentation validates static frontend, content, i18n, and template
// setup without opening runtime database resources.
func ValidatePresentation(ctx context.Context, cfg Config, logger *slog.Logger) error {
	return app.ValidatePresentation(ctx, cfg.PresentationOptions(logger))
}

// devDefaultDBPassword is the database password shipped in configs/config.yaml
// for local development. Production boot refuses this value — see
// validateProductionConfig.
const devDefaultDBPassword = "pw123"

// validateProductionConfig fails closed when a production boot would use an
// unset or dev-default database password, so a config.yaml copied verbatim
// into production cannot silently run with a known credential.
func validateProductionConfig(cfg Config) error {
	if cfg.Env != "production" {
		return nil
	}
	if cfg.Database.Password == "" || cfg.Database.Password == devDefaultDBPassword {
		return fmt.Errorf("production database password is unset or the dev default; set database.password")
	}
	return nil
}

// Run applies instance overrides, validates presentation-only startup when
// requested, starts the HTTP server, and blocks until shutdown.
func Run(ctx context.Context, cfg Config, logger *slog.Logger) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if logger == nil {
		logger = slog.Default()
	}

	startTime := time.Now()
	cfg = ApplyInstanceOverrides(cfg)
	if cfg.ValidateOnly {
		logger.Info("Running weave template validation only (no database connection)")
		if err := ValidatePresentation(ctx, cfg, logger); err != nil {
			return fmt.Errorf("validate presentation: %w", err)
		}
		logger.Info("Weave template validation PASSED - all templates parsed successfully")
		return nil
	}
	// validate-only intentionally short-circuits above and never opens a DB
	// pool, so `make validate`'s production-mode template check is unaffected
	// by this guard. Real boot below does open the pool, so the guard runs
	// before it.
	if err := validateProductionConfig(cfg); err != nil {
		return err
	}

	logger.Info("Starting Pletka server",
		"version", buildinfo.Version,
		"commit", buildinfo.GitCommit,
		"branch", buildinfo.GitBranch,
		"dirty", buildinfo.GitDirty != "",
		"built_at", buildinfo.BuildTime,
		"env", cfg.Env,
		"instance", cfg.Instance)

	runtime, err := Start(ctx, cfg, logger)
	if err != nil {
		return err
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := runtime.Close(shutdownCtx); err != nil {
			logger.Warn("runtime cleanup failed", "err", err)
		}
	}()

	server := &http.Server{
		Handler:      runtime.App.Handler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	serverErr := make(chan error, 1)
	addr := fmt.Sprintf("%s:%s", cfg.Host, cfg.Port)
	if cfg.SocketPath != "" {
		if err := os.Remove(cfg.SocketPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove stale socket %q: %w", cfg.SocketPath, err)
		}
		listener, err := net.Listen("unix", cfg.SocketPath)
		if err != nil {
			return fmt.Errorf("listen on socket %q: %w", cfg.SocketPath, err)
		}
		if err := os.Chmod(cfg.SocketPath, 0o660); err != nil {
			_ = listener.Close()
			return fmt.Errorf("chmod socket %q: %w", cfg.SocketPath, err)
		}
		if grp, err := user.LookupGroup("www-data"); err == nil {
			if gid, err := strconv.Atoi(grp.Gid); err == nil {
				_ = os.Chown(cfg.SocketPath, -1, gid)
			}
		}
		logger.Info("Server starting on Unix socket", "path", cfg.SocketPath)
		go func() {
			if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
				serverErr <- err
				return
			}
			serverErr <- nil
		}()
		logger.Info("Server started successfully",
			"socket", cfg.SocketPath,
			"total_startup_duration", time.Since(startTime).String())
	} else {
		server.Addr = addr
		logger.Info("Server starting on TCP", "addr", addr)
		go func() {
			if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				serverErr <- err
				return
			}
			serverErr <- nil
		}()
		logger.Info("Server started successfully",
			"url", fmt.Sprintf("http://%s", addr),
			"total_startup_duration", time.Since(startTime).String())
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(quit)

	select {
	case <-ctx.Done():
		logger.Info("Server context cancelled")
	case sig := <-quit:
		logger.Info("Server received shutdown signal", "signal", sig.String())
	case err := <-serverErr:
		if err != nil {
			return fmt.Errorf("server failed: %w", err)
		}
		return nil
	}

	logger.Info("Server is shutting down...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("server forced to shutdown: %w", err)
	}
	if cfg.SocketPath != "" {
		_ = os.Remove(cfg.SocketPath)
	}
	logger.Info("Server stopped gracefully")
	return nil
}

// ApplyInstanceOverrides derives per-instance config without mutating the
// global config source. Explicit PLETKA_DB_NAME still wins, matching the legacy
// viper mutation order.
func ApplyInstanceOverrides(cfg Config) Config {
	if cfg.Instance == "" {
		return cfg
	}
	cfg.Database.Name = fmt.Sprintf("pletka_%s", cfg.Instance)
	if envName := os.Getenv("PLETKA_DB_NAME"); envName != "" {
		cfg.Database.Name = envName
	}
	cfg.AllowedOrigins = mergeAllowedOrigins(fmt.Sprintf("https://%s.pletka.io", cfg.Instance), cfg.AllowedOrigins)
	return cfg
}

// mergeAllowedOrigins combines the derived per-instance origin with any
// config-supplied extra origins, de-duplicated, with the derived origin
// first. Config-supplied origins are never dropped — CORS.allowed_origins
// is additive on top of the auto-derived instance origin, not a replacement
// for it.
func mergeAllowedOrigins(derived string, extra []string) []string {
	seen := make(map[string]struct{}, len(extra)+1)
	merged := make([]string, 0, len(extra)+1)
	add := func(origin string) {
		if origin == "" {
			return
		}
		if _, ok := seen[origin]; ok {
			return
		}
		seen[origin] = struct{}{}
		merged = append(merged, origin)
	}
	add(derived)
	for _, origin := range extra {
		add(origin)
	}
	return merged
}

// Start opens runtime resources, runs migrations when configured, builds app
// options, and assembles the HTTP app.
func Start(ctx context.Context, cfg Config, logger *slog.Logger) (*Runtime, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if logger == nil {
		logger = slog.Default()
	}

	sqlDB, err := openSQLDB(ctx, cfg, logger)
	if err != nil {
		return nil, err
	}
	runtime := &Runtime{SQLDB: sqlDB}
	started := false
	defer func() {
		if !started {
			_ = runtime.Close(context.Background())
		}
	}()

	if err := runMigrations(sqlDB, cfg, logger); err != nil {
		return nil, err
	}

	pgxPool, err := cliruntime.OpenPGXPoolWithSettings(ctx, cfg.Database)
	if err != nil {
		return nil, fmt.Errorf("create pgx pool: %w", err)
	}
	runtime.Pool = pgxPool

	pletkaApp, err := buildApp(ctx, cfg, logger, pgxPool)
	if err != nil {
		return nil, err
	}
	runtime.App = pletkaApp

	started = true
	return runtime, nil
}

func (r *Runtime) Close(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	var err error
	if r.App != nil {
		closeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if closeErr := r.App.Close(closeCtx); closeErr != nil {
			err = closeErr
		}
	}
	if r.Pool != nil {
		r.Pool.Close()
	}
	if r.SQLDB != nil {
		if closeErr := r.SQLDB.Close(); err == nil && closeErr != nil {
			err = closeErr
		}
	}
	return err
}

func openSQLDB(ctx context.Context, cfg Config, logger *slog.Logger) (*sql.DB, error) {
	start := time.Now()
	logger.Info("Connecting to database",
		"host", cfg.Database.Host,
		"port", cfg.Database.Port,
		"name", cfg.Database.Name)

	sqlDB, err := cliruntime.OpenPingedSQLDBWithSettings(ctx, cfg.Database)
	if err != nil {
		return nil, fmt.Errorf("connect database: %w", err)
	}
	logger.Info("Database initialized", "duration", time.Since(start).String())
	return sqlDB, nil
}

func runMigrations(sqlDB *sql.DB, cfg Config, logger *slog.Logger) error {
	if cfg.SkipMigrations {
		logger.Warn("Skipping database migrations as requested")
		return nil
	}
	start := time.Now()
	logger.Info("Running goose database migrations")
	if err := database.Migrate(sqlDB); err != nil {
		return fmt.Errorf("database migrations: %w", err)
	}
	if cfg.MigrateExtra != nil {
		if err := cfg.MigrateExtra(sqlDB); err != nil {
			return fmt.Errorf("extra migrations: %w", err)
		}
	}
	logger.Info("Database migrations completed", "duration", time.Since(start).String())
	return nil
}

func buildApp(ctx context.Context, cfg Config, logger *slog.Logger, pool *pgxpool.Pool) (*app.App, error) {
	integrationCipher, err := buildIntegrationCipher(cfg, logger)
	if err != nil {
		return nil, err
	}

	appOptions := app.Options{
		Pool:     pool,
		Logger:   logger,
		DevMode:  cfg.Env == "development",
		Instance: cfg.Instance,
		Analytics: weavetemplates.AnalyticsConfig{
			Enabled:          cfg.AnalyticsEnabled,
			MatomoBaseURL:    cfg.AnalyticsMatomoBaseURL,
			SiteID:           cfg.AnalyticsMatomoSiteID,
			TrackQueryString: cfg.AnalyticsTrackQueryString,
		},
		LogStaticFiles:            cfg.LogStaticFiles,
		AllowedOrigins:            cfg.AllowedOrigins,
		RegistrationEnabled:       cfg.RegistrationEnabled,
		SSOLoginURL:               cfg.SSOLoginURL,
		GitDataDir:                gitDataDir(cfg),
		ModuleHost:                cfg.ModuleHost,
		OntologyHost:              cfg.OntologyHost,
		MetricsAddr:               cfg.MetricsAddr,
		ContentOverlayPath:        cfg.ContentOverlayPath,
		FrontendManifestPaths:     cfg.FrontendManifestPaths,
		CoreFrontendManifestPath:  cfg.CoreFrontendManifestPath,
		IntegrationCipher:         integrationCipher,
		AutocompleteMode:          cfg.AutocompleteMode,
		AutocompleteAllowOverride: cfg.AutocompleteAllowOverride,
	}
	appOptions.Contributions.StaticAssets = append(appOptions.Contributions.StaticAssets, cfg.StaticAssets...)
	appOptions.Contributions.FrontendManifests = append(appOptions.Contributions.FrontendManifests, cfg.FrontendManifests...)
	if cfg.ConfigureApp != nil {
		if err := cfg.ConfigureApp(&appOptions, pool, integrationCipher); err != nil {
			return nil, fmt.Errorf("configure app options: %w", err)
		}
	} else {
		coreRegistry, err := integrations.CoreRegistry()
		if err != nil {
			return nil, fmt.Errorf("build core integration registry: %w", err)
		}
		appOptions.IntegrationRegistry = coreRegistry
	}

	pletkaApp, err := app.New(ctx, appOptions)
	if err != nil {
		return nil, fmt.Errorf("build app: %w", err)
	}
	return pletkaApp, nil
}

func buildIntegrationCipher(cfg Config, logger *slog.Logger) (*integrations.Cipher, error) {
	if cfg.IntegrationSecretKey == "" {
		logger.Info("integrations.secret_key not set — integrations declaring secret fields will refuse to save")
		return nil, nil
	}
	cipher, err := integrations.NewCipher(cfg.IntegrationSecretKey)
	if err != nil {
		return nil, fmt.Errorf("build integration cipher: %w", err)
	}
	return cipher, nil
}

func gitDataDir(cfg Config) string {
	if cfg.GitDataDir != "" {
		return cfg.GitDataDir
	}
	return "./data/git-projects"
}
