package cliruntime

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// EnvBinding maps a viper key to one or more environment variable names.
type EnvBinding struct {
	Key   string
	Names []string
}

// ConfigBootstrap describes the command-runtime configuration bootstrap.
// Core and platform CLIs can share this shape while supplying different
// defaults or env bindings.
type ConfigBootstrap struct {
	ConfigKey  string
	ConfigName string
	ConfigType string
	Paths      []string
	EnvPrefix  string
	Env        []EnvBinding
	Defaults   map[string]any
}

// DefaultConfigBootstrap returns the root CLI configuration behavior used by
// the Pletka server and command surfaces.
func DefaultConfigBootstrap() ConfigBootstrap {
	return ConfigBootstrap{
		ConfigKey:  "config",
		ConfigName: "config",
		ConfigType: "yaml",
		Paths: []string{
			".",
			"./configs",
			"$HOME/.pletka",
		},
		EnvPrefix: "PLETKA",
		Env: []EnvBinding{
			// Legacy ZELLIJ_* names are kept as fallbacks during the rename.
			{Key: "log.level", Names: []string{"PLETKA_LOG_LEVEL", "ZELLIJ_LOG_LEVEL"}},
			{Key: "env", Names: []string{"PLETKA_ENV", "ZELLIJ_ENV"}},
			{Key: "content.overlay_path", Names: []string{"PLETKA_CONTENT_OVERLAY"}},
			{Key: "cors.allowed_origins", Names: []string{"PLETKA_CORS_ALLOWED_ORIGINS"}},
			{Key: "integrations.secret_key", Names: []string{"PLETKA_INTEGRATIONS_SECRET_KEY"}},
			{Key: "observability.metrics_addr", Names: []string{"PLETKA_OBSERVABILITY_METRICS_ADDR"}},
			{Key: "analytics.enabled", Names: []string{"PLETKA_ANALYTICS_ENABLED"}},
			{Key: "analytics.matomo.site_id", Names: []string{"PLETKA_ANALYTICS_MATOMO_SITE_ID"}},
			{Key: "analytics.matomo.base_url", Names: []string{"PLETKA_ANALYTICS_MATOMO_BASE_URL"}},
			{Key: "analytics.track_query_string", Names: []string{"PLETKA_ANALYTICS_TRACK_QUERY_STRING"}},
		},
		Defaults: map[string]any{
			"log.level":                 "warn",
			"log.format":                "json",
			"log.static_files":          false,
			"env":                       "development",
			"server.port":               "8080",
			"server.host":               "localhost",
			"database.host":             "localhost",
			"database.port":             5433,
			"database.name":             "pletka_weave",
			"database.user":             "postgres",
			"database.password":         "pw123",
			"database.sslmode":          "disable",
			"autocomplete.mode":         "indexed",
			"analytics.matomo.base_url": "https://analytics.pletka.io",
			"autocomplete.allow_per_request_override": true,
		},
	}
}

// AddRootConfigFlags registers the root flags consumed by BootstrapConfig.
func AddRootConfigFlags(cmd *cobra.Command) error {
	flags := cmd.PersistentFlags()
	flags.StringP("config", "c", "", "config file (default is ./config.yaml)")
	flags.StringP("log-level", "l", "info", "log level (debug, info, warn, error)")
	flags.String("log-format", "json", "log format (json, text)")
	flags.StringP("env", "e", "development", "environment (development, production)")

	if err := viper.BindPFlag("config", flags.Lookup("config")); err != nil {
		return err
	}
	if err := viper.BindPFlag("log.level", flags.Lookup("log-level")); err != nil {
		return err
	}
	if err := viper.BindPFlag("log.format", flags.Lookup("log-format")); err != nil {
		return err
	}
	return viper.BindPFlag("env", flags.Lookup("env"))
}

// BootstrapConfig applies config-file search paths, environment bindings, and
// defaults. It preserves the previous command behavior by ignoring missing or
// unreadable config files.
func BootstrapConfig(cfg ConfigBootstrap) {
	if cfg.ConfigKey == "" {
		cfg.ConfigKey = "config"
	}
	if cfgFile := viper.GetString(cfg.ConfigKey); cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		for _, path := range cfg.Paths {
			viper.AddConfigPath(path)
		}
		if cfg.ConfigName != "" {
			viper.SetConfigName(cfg.ConfigName)
		}
		if cfg.ConfigType != "" {
			viper.SetConfigType(cfg.ConfigType)
		}
	}

	viper.AutomaticEnv()
	if cfg.EnvPrefix != "" {
		viper.SetEnvPrefix(cfg.EnvPrefix)
	}
	for _, binding := range cfg.Env {
		args := append([]string{binding.Key}, binding.Names...)
		_ = viper.BindEnv(args...)
	}
	for key, value := range cfg.Defaults {
		viper.SetDefault(key, value)
	}

	_ = viper.ReadInConfig()
}
