package cliruntime

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
)

func TestParseLogLevel(t *testing.T) {
	tests := []struct {
		name  string
		level string
		want  slog.Level
	}{
		{name: "debug", level: "debug", want: slog.LevelDebug},
		{name: "info", level: "info", want: slog.LevelInfo},
		{name: "warn", level: "warn", want: slog.LevelWarn},
		{name: "error", level: "error", want: slog.LevelError},
		{name: "uppercase", level: "DEBUG", want: slog.LevelDebug},
		{name: "unknown", level: "trace", want: slog.LevelInfo},
		{name: "empty", level: "", want: slog.LevelInfo},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ParseLogLevel(tt.level); got != tt.want {
				t.Fatalf("ParseLogLevel(%q) = %s, want %s", tt.level, got, tt.want)
			}
		})
	}
}

func TestNewLoggerHonorsLevel(t *testing.T) {
	logger := NewLogger(LoggerConfig{Level: "warn"}, &bytes.Buffer{})

	if logger.Enabled(context.Background(), slog.LevelInfo) {
		t.Fatal("info logging is enabled, want disabled")
	}
	if !logger.Enabled(context.Background(), slog.LevelWarn) {
		t.Fatal("warn logging is disabled, want enabled")
	}
}

func TestNewLoggerUsesTextFormatInDevelopment(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(LoggerConfig{Level: "info", Format: "json", Env: "development"}, &buf)

	logger.Info("hello")

	out := strings.TrimSpace(buf.String())
	if strings.HasPrefix(out, "{") {
		t.Fatalf("development logger wrote JSON output: %s", out)
	}
	if !strings.Contains(out, "msg=hello") {
		t.Fatalf("development logger output = %q, want text output containing msg=hello", out)
	}
}

func TestNewLoggerUsesJSONFormatOutsideDevelopment(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(LoggerConfig{Level: "info", Format: "json", Env: "production"}, &buf)

	logger.Info("hello")

	out := strings.TrimSpace(buf.String())
	if !strings.HasPrefix(out, "{") {
		t.Fatalf("production logger output = %q, want JSON output", out)
	}
	if !strings.Contains(out, `"msg":"hello"`) {
		t.Fatalf("production logger output = %q, want JSON message", out)
	}
}
