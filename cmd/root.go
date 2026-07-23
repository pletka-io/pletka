package cmd

import (
	"log/slog"
	"os"

	"github.com/pletka-io/pletka/pkg/app/cliruntime"
	"github.com/pletka-io/pletka/pkg/weave/generators"
	"github.com/pletka-io/pletka/pkg/weave/genwiring"
	"github.com/spf13/cobra"
)

var logger *slog.Logger

// Execute builds and runs the root command tree.
func Execute() error {
	return newRootCommand().Execute()
}

func newRootCommand() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "pletka",
		Short: "Semantic platform for collaborative research data modeling",
		Long: `Pletka is a GitHub-like platform for semantic models that enables researchers
to create, share, and collaborate on semantic data models based on RDF/OWL ontologies.

Features:
- Field reuse and adoption (inherit, clone, fork)
- Version control for semantic models
- Social collaboration (stars, issues, following)
- Automatic RDF/Turtle generation
- Institution-based access control`,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			// Initialize configuration and logging before any command runs.
			cliruntime.BootstrapConfig(cliruntime.DefaultConfigBootstrap())
			initLogger()
		},
	}

	if err := cliruntime.AddRootConfigFlags(rootCmd); err != nil {
		panic(err)
	}

	rootCmd.AddCommand(newServeCommand())
	AddOperationalCommands(rootCmd, OperationalOptions{})
	rootCmd.AddCommand(newPagesCommand())

	return rootCmd
}

// OperationalOptions configures the operational command surface for the
// binary embedding it. The zero value is the core-only surface.
type OperationalOptions struct {
	// GeneratorRenderers backs `weave generate`. Empty means
	// genwiring.CoreCLIRenderers().
	GeneratorRenderers []generators.Renderer
}

// AddOperationalCommands registers pletka's operational subcommands on root.
// It exists so hosted-platform wrappers (e.g. pletkactl) can reuse core's
// operational surface - the "ship a one-off core binary just to run
// project init-git on a server" problem - without duplicating command
// wiring or importing cmd internals piecemeal.
//
// Included:
//   - project (init-git, load-git)              - materialize/hydrate a
//     project's database state to/from a git working tree
//   - db (migrate, migrate status, migrate down) - goose schema migrations
//   - weave (generate, verify-paths)             - generator output and
//     ontology path verification
//
// Deliberately excluded:
//   - serve - a hosted wrapper owns its own serve command (different
//     static assets, CORS/instance config, etc.); see cmd/serve.go
//   - pages (extract, hydrate, promote) - a filesystem-only content
//     authoring tool that reads/writes this repo's own source tree
//     (pkg/weave/content/pages, pkg/assets/i18n). It never touches a
//     database or a deployed instance, so it has no meaning inside a
//     hosted-platform CLI - it belongs to core's own dev workflow only
//
// Callers are responsible for bootstrapping viper (cliruntime.BootstrapConfig)
// and, if desired, logging before these commands run - e.g. in their own
// root command's PersistentPreRun - since AddOperationalCommands does not
// add one of its own. The commands registered here do not depend on core's
// package-level `logger` var except indirectly through
// app.NewGeneratorRuntime, which already falls back to slog.Default() when
// no logger is supplied (see pkg/app/generator_runtime.go), so running
// under a foreign root's PersistentPreRun is safe even though core's own
// initLogger never runs in that case.
func AddOperationalCommands(root *cobra.Command, opts OperationalOptions) {
	renderers := opts.GeneratorRenderers
	if len(renderers) == 0 {
		renderers = genwiring.CoreCLIRenderers()
	}
	root.AddCommand(
		newWeaveCommand(renderers),
		newProjectCommand(),
		newDBCommand(),
		newAPIKeyCommand(),
		newReleaseCommand(),
	)
}

// initLogger initializes structured logging with slog
func initLogger() {
	cfg := cliruntime.LoggerConfigFromViper()
	level := cliruntime.ParseLogLevel(cfg.Level)
	logger = cliruntime.NewLogger(cfg, os.Stderr)
	slog.SetDefault(logger)

	logger.Info("Logger initialized",
		"level", level.String(),
		"format", cfg.Format,
		"env", cfg.Env)

	if cfg.ConfigFileUsed != "" {
		logger.Info("Using config file", "path", cfg.ConfigFileUsed)
	}
}
