package cliruntime

import (
	"fmt"
	"io"
	"log/slog"
	"strings"

	"github.com/spf13/viper"
)

// LoggerConfig contains the CLI-facing logger settings read from flags,
// environment, or config files before command execution starts.
type LoggerConfig struct {
	Level          string
	Format         string
	Env            string
	ConfigFileUsed string
}

// LoggerConfigFromViper reads logger settings from the viper keys used by the
// command surfaces.
func LoggerConfigFromViper() LoggerConfig {
	return LoggerConfig{
		Level:          viper.GetString("log.level"),
		Format:         viper.GetString("log.format"),
		Env:            viper.GetString("env"),
		ConfigFileUsed: viper.ConfigFileUsed(),
	}
}

// ParseLogLevel converts CLI/config log level strings to slog levels.
func ParseLogLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// NewLogger builds the structured logger used by command runtimes.
func NewLogger(cfg LoggerConfig, output io.Writer) *slog.Logger {
	if output == nil {
		output = io.Discard
	}

	opts := &slog.HandlerOptions{
		Level:     ParseLogLevel(cfg.Level),
		AddSource: true,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.SourceKey {
				if src, ok := a.Value.Any().(*slog.Source); ok && src.Function != "" {
					a.Value = slog.StringValue(fmt.Sprintf("%s:%d", src.Function, src.Line))
				}
			}
			return a
		},
	}

	if cfg.Format == "text" || cfg.Env == "development" {
		return slog.New(slog.NewTextHandler(output, opts))
	}
	return slog.New(slog.NewJSONHandler(output, opts))
}
