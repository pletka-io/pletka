// Command pletka runs the standalone Pletka server.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"github.com/pletka-io/pletka/server"
	"github.com/pletka-io/pletka/store/postgres"
)

// Build-time variables populated by goreleaser via -ldflags.
var (
	version = "0.1.0-dev"
	commit  = "unknown"
	date    = "unknown"
	cfgFile string
	logger  *slog.Logger
)

func main() {
	if err := newRootCommand().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:           "pletka",
		Short:         "Schema-driven web platform for semantic models",
		Version:       fmt.Sprintf("%s (commit %s, built %s)", version, commit, date),
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			if cmd.Name() == "version" {
				return nil
			}
			if err := initConfig(cmd); err != nil {
				return err
			}
			logger = newLogger()
			slog.SetDefault(logger)
			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}

	root.SetVersionTemplate("pletka {{.Version}}\n")
	root.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "config file")
	root.PersistentFlags().String("log-level", "info", "log level: debug, info, warn, error")
	root.PersistentFlags().String("log-format", "text", "log format: text or json")
	root.PersistentFlags().String("database-url", "", "PostgreSQL connection URL; overrides individual database fields")
	root.PersistentFlags().String("database-host", "localhost", "PostgreSQL host")
	root.PersistentFlags().String("database-port", "5432", "PostgreSQL port")
	root.PersistentFlags().String("database-name", "pletka", "PostgreSQL database name")
	root.PersistentFlags().String("database-user", "postgres", "PostgreSQL user")
	root.PersistentFlags().String("database-password", "", "PostgreSQL password")
	root.PersistentFlags().String("database-sslmode", "disable", "PostgreSQL sslmode")
	root.PersistentFlags().Int32("database-max-conns", 10, "PostgreSQL maximum pool connections")
	root.PersistentFlags().Int32("database-min-conns", 0, "PostgreSQL minimum pool connections")
	root.PersistentFlags().Duration("database-connect-timeout", 10*time.Second, "PostgreSQL connection timeout")

	mustBindFlag("log.level", root.PersistentFlags().Lookup("log-level"))
	mustBindFlag("log.format", root.PersistentFlags().Lookup("log-format"))
	mustBindFlag("database.url", root.PersistentFlags().Lookup("database-url"))
	mustBindFlag("database.host", root.PersistentFlags().Lookup("database-host"))
	mustBindFlag("database.port", root.PersistentFlags().Lookup("database-port"))
	mustBindFlag("database.name", root.PersistentFlags().Lookup("database-name"))
	mustBindFlag("database.user", root.PersistentFlags().Lookup("database-user"))
	mustBindFlag("database.password", root.PersistentFlags().Lookup("database-password"))
	mustBindFlag("database.sslmode", root.PersistentFlags().Lookup("database-sslmode"))
	mustBindFlag("database.max_conns", root.PersistentFlags().Lookup("database-max-conns"))
	mustBindFlag("database.min_conns", root.PersistentFlags().Lookup("database-min-conns"))
	mustBindFlag("database.connect_timeout", root.PersistentFlags().Lookup("database-connect-timeout"))

	root.AddCommand(newServeCommand())
	root.AddCommand(newMigrateCommand())
	root.AddCommand(newVersionCommand())
	return root
}

func newServeCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the Pletka HTTP server",
		RunE: func(_ *cobra.Command, _ []string) error {
			return serve()
		},
	}

	cmd.Flags().String("host", "localhost", "host to bind")
	cmd.Flags().StringP("port", "p", "8080", "port to listen on")
	cmd.Flags().Duration("read-header-timeout", 10*time.Second, "maximum time to read request headers")
	cmd.Flags().Duration("read-timeout", 30*time.Second, "maximum time to read the full request")
	cmd.Flags().Duration("write-timeout", 30*time.Second, "maximum time to write the response")
	cmd.Flags().Duration("idle-timeout", 120*time.Second, "maximum time to wait for the next request")
	cmd.Flags().Duration("shutdown-timeout", 10*time.Second, "maximum graceful shutdown wait")
	cmd.Flags().Bool("pprof", false, "enable pprof debug server")
	cmd.Flags().String("pprof-host", "localhost", "pprof host to bind")
	cmd.Flags().String("pprof-port", "6060", "pprof port to listen on")

	mustBindFlag("server.host", cmd.Flags().Lookup("host"))
	mustBindFlag("server.port", cmd.Flags().Lookup("port"))
	mustBindFlag("server.read_header_timeout", cmd.Flags().Lookup("read-header-timeout"))
	mustBindFlag("server.read_timeout", cmd.Flags().Lookup("read-timeout"))
	mustBindFlag("server.write_timeout", cmd.Flags().Lookup("write-timeout"))
	mustBindFlag("server.idle_timeout", cmd.Flags().Lookup("idle-timeout"))
	mustBindFlag("server.shutdown_timeout", cmd.Flags().Lookup("shutdown-timeout"))
	mustBindFlag("debug.pprof.enabled", cmd.Flags().Lookup("pprof"))
	mustBindFlag("debug.pprof.host", cmd.Flags().Lookup("pprof-host"))
	mustBindFlag("debug.pprof.port", cmd.Flags().Lookup("pprof-port"))

	return cmd
}

func newMigrateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Manage PostgreSQL schema migrations",
	}
	cmd.PersistentFlags().Duration("timeout", 2*time.Minute, "migration command timeout")
	mustBindFlag("migrate.timeout", cmd.PersistentFlags().Lookup("timeout"))

	cmd.AddCommand(&cobra.Command{
		Use:   "up",
		Short: "Apply all pending migrations",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), viper.GetDuration("migrate.timeout"))
			defer cancel()
			return postgres.Up(ctx, postgresConfigFromViper(), cmd.OutOrStdout())
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "Print migration status",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), viper.GetDuration("migrate.timeout"))
			defer cancel()
			return postgres.Status(ctx, postgresConfigFromViper(), cmd.OutOrStdout())
		},
	})
	return cmd
}

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print build version",
		Run: func(cmd *cobra.Command, _ []string) {
			fmt.Fprintf(cmd.OutOrStdout(), "pletka %s (commit %s, built %s)\n", version, commit, date)
		},
	}
}

func initConfig(cmd *cobra.Command) error {
	viper.SetConfigType("yaml")
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		viper.SetConfigName("config")
		viper.AddConfigPath(".")
		viper.AddConfigPath("./configs")
		viper.AddConfigPath("$HOME/.pletka")
	}

	viper.SetEnvPrefix("PLETKA")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	viper.AutomaticEnv()
	setDefaults()

	if err := viper.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if !errors.As(err, &notFound) {
			return fmt.Errorf("read config: %w", err)
		}
	}

	if logger != nil {
		logger.Debug("config initialized", "config_file", viper.ConfigFileUsed(), "command", cmd.CommandPath())
	}
	return nil
}

func setDefaults() {
	viper.SetDefault("log.level", "info")
	viper.SetDefault("log.format", "text")
	viper.SetDefault("database.url", "")
	viper.SetDefault("database.host", "localhost")
	viper.SetDefault("database.port", "5432")
	viper.SetDefault("database.name", "pletka")
	viper.SetDefault("database.user", "postgres")
	viper.SetDefault("database.password", "")
	viper.SetDefault("database.sslmode", "disable")
	viper.SetDefault("database.max_conns", 10)
	viper.SetDefault("database.min_conns", 0)
	viper.SetDefault("database.connect_timeout", 10*time.Second)
	viper.SetDefault("migrate.timeout", 2*time.Minute)
	viper.SetDefault("server.host", "localhost")
	viper.SetDefault("server.port", "8080")
	viper.SetDefault("server.read_header_timeout", 10*time.Second)
	viper.SetDefault("server.read_timeout", 30*time.Second)
	viper.SetDefault("server.write_timeout", 30*time.Second)
	viper.SetDefault("server.idle_timeout", 120*time.Second)
	viper.SetDefault("server.shutdown_timeout", 10*time.Second)
	viper.SetDefault("debug.pprof.enabled", false)
	viper.SetDefault("debug.pprof.host", "localhost")
	viper.SetDefault("debug.pprof.port", "6060")
}

func newLogger() *slog.Logger {
	opts := &slog.HandlerOptions{Level: parseLogLevel(viper.GetString("log.level"))}
	if strings.EqualFold(viper.GetString("log.format"), "json") {
		return slog.New(slog.NewJSONHandler(os.Stderr, opts))
	}
	return slog.New(slog.NewTextHandler(os.Stderr, opts))
}

func parseLogLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func postgresConfigFromViper() postgres.Config {
	return postgres.Config{
		URL:            viper.GetString("database.url"),
		Host:           viper.GetString("database.host"),
		Port:           viper.GetString("database.port"),
		Database:       viper.GetString("database.name"),
		User:           viper.GetString("database.user"),
		Password:       viper.GetString("database.password"),
		SSLMode:        viper.GetString("database.sslmode"),
		MaxConnections: viper.GetInt32("database.max_conns"),
		MinConnections: viper.GetInt32("database.min_conns"),
		ConnectTimeout: viper.GetDuration("database.connect_timeout"),
	}
}

func serve() error {
	if logger == nil {
		logger = newLogger()
	}

	srv, err := server.New(server.Config{
		Addr:              net.JoinHostPort(viper.GetString("server.host"), viper.GetString("server.port")),
		Logger:            logger,
		ReadHeaderTimeout: viper.GetDuration("server.read_header_timeout"),
		ReadTimeout:       viper.GetDuration("server.read_timeout"),
		WriteTimeout:      viper.GetDuration("server.write_timeout"),
		IdleTimeout:       viper.GetDuration("server.idle_timeout"),
		BuildInfo: server.BuildInfo{
			Version: version,
			Commit:  commit,
			Date:    date,
		},
	})
	if err != nil {
		return err
	}

	servers := []*http.Server{srv.HTTPServer()}
	if viper.GetBool("debug.pprof.enabled") {
		servers = append(servers, newPprofServer())
	}

	errc := make(chan error, len(servers))
	for _, httpServer := range servers {
		go serveHTTP(logger, httpServer, errc)
	}

	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigc)

	select {
	case sig := <-sigc:
		logger.Info("stopping pletka server", "signal", sig.String())
		ctx, cancel := context.WithTimeout(context.Background(), viper.GetDuration("server.shutdown_timeout"))
		defer cancel()
		return shutdownServers(ctx, servers)
	case err := <-errc:
		return err
	}
}

func serveHTTP(logger *slog.Logger, srv *http.Server, errc chan<- error) {
	logger.Info("starting http server", "addr", srv.Addr)
	err := srv.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		errc <- err
		return
	}
	errc <- nil
}

func newPprofServer() *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	mux.Handle("/debug/pprof/allocs", pprof.Handler("allocs"))
	mux.Handle("/debug/pprof/block", pprof.Handler("block"))
	mux.Handle("/debug/pprof/goroutine", pprof.Handler("goroutine"))
	mux.Handle("/debug/pprof/heap", pprof.Handler("heap"))
	mux.Handle("/debug/pprof/mutex", pprof.Handler("mutex"))
	mux.Handle("/debug/pprof/threadcreate", pprof.Handler("threadcreate"))

	return &http.Server{
		Addr:              net.JoinHostPort(viper.GetString("debug.pprof.host"), viper.GetString("debug.pprof.port")),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
}

func shutdownServers(ctx context.Context, servers []*http.Server) error {
	var firstErr error
	for _, srv := range servers {
		if err := srv.Shutdown(ctx); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func mustBindFlag(key string, flag *pflag.Flag) {
	if flag == nil {
		panic(fmt.Sprintf("invalid flag binding for %s", key))
	}
	if err := viper.BindPFlag(key, flag); err != nil {
		panic(err)
	}
}
