package cmd

import (
	"context"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/pletka-io/pletka/pkg/app/serverruntime"
)

func newServeCommand() *cobra.Command {
	serveCmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the web server",
		Long: `Start the Pletka web server to serve the semantic platform application.

The server provides both a web interface and REST API for managing semantic models,
handling authentication, and enabling collaboration features.`,
		Run: func(cmd *cobra.Command, args []string) {
			runServer()
		},
	}

	// Server-specific flags
	serveCmd.Flags().StringP("port", "p", "8080", "port to run server on")
	serveCmd.Flags().String("host", "localhost", "host to bind server to")
	serveCmd.Flags().Bool("skip-migrations", false, "skip database migrations on startup")
	serveCmd.Flags().Bool("validate-only", false, "validate weave templates and exit (no database required)")
	serveCmd.Flags().String("instance", "", "instance name for multi-instance mode (derives database and CORS defaults)")
	serveCmd.Flags().String("socket", "", "Unix socket path (for nginx proxy, disables TCP listener)")

	// Bind flags to viper
	viper.BindPFlag("server.port", serveCmd.Flags().Lookup("port"))
	viper.BindPFlag("server.host", serveCmd.Flags().Lookup("host"))
	viper.BindPFlag("server.skip_migrations", serveCmd.Flags().Lookup("skip-migrations"))
	viper.BindPFlag("server.validate_only", serveCmd.Flags().Lookup("validate-only"))
	viper.BindPFlag("server.instance", serveCmd.Flags().Lookup("instance"))
	viper.BindPFlag("server.socket", serveCmd.Flags().Lookup("socket"))

	return serveCmd
}

func runServer() {
	cfg := serverruntime.ConfigFromViper()
	if err := serverruntime.Run(context.Background(), cfg, logger); err != nil {
		logger.Error("start server runtime", "err", err)
		os.Exit(1)
	}
}
