package cliruntime

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func TestAddRootConfigFlagsBindsViperKeys(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	cmd := &cobra.Command{Use: "test"}
	if err := AddRootConfigFlags(cmd); err != nil {
		t.Fatalf("AddRootConfigFlags() error = %v", err)
	}
	if err := cmd.ParseFlags([]string{
		"--log-level", "debug",
		"--log-format", "text",
		"--env", "production",
		"--config", "/tmp/config.yaml",
	}); err != nil {
		t.Fatalf("ParseFlags() error = %v", err)
	}

	if got, want := viper.GetString("log.level"), "debug"; got != want {
		t.Fatalf("log.level = %q, want %q", got, want)
	}
	if got, want := viper.GetString("log.format"), "text"; got != want {
		t.Fatalf("log.format = %q, want %q", got, want)
	}
	if got, want := viper.GetString("env"), "production"; got != want {
		t.Fatalf("env = %q, want %q", got, want)
	}
	if got, want := viper.GetString("config"), "/tmp/config.yaml"; got != want {
		t.Fatalf("config = %q, want %q", got, want)
	}
}

func TestBootstrapConfigAppliesDefaults(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	BootstrapConfig(DefaultConfigBootstrap())

	if got, want := viper.GetString("database.name"), "pletka_weave"; got != want {
		t.Fatalf("database.name = %q, want %q", got, want)
	}
	if got, want := viper.GetString("autocomplete.mode"), "indexed"; got != want {
		t.Fatalf("autocomplete.mode = %q, want %q", got, want)
	}
	if got := viper.GetBool("autocomplete.allow_per_request_override"); !got {
		t.Fatal("autocomplete.allow_per_request_override = false, want true")
	}
}

func TestBootstrapConfigBindsLegacyEnvFallback(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	t.Setenv("ZELLIJ_LOG_LEVEL", "debug")

	BootstrapConfig(DefaultConfigBootstrap())

	if got, want := viper.GetString("log.level"), "debug"; got != want {
		t.Fatalf("log.level = %q, want legacy env fallback %q", got, want)
	}
}

func TestBootstrapConfigPrefersPletkaEnvOverLegacyFallback(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	t.Setenv("PLETKA_LOG_LEVEL", "warn")
	t.Setenv("ZELLIJ_LOG_LEVEL", "debug")

	BootstrapConfig(DefaultConfigBootstrap())

	if got, want := viper.GetString("log.level"), "warn"; got != want {
		t.Fatalf("log.level = %q, want PLETKA env %q", got, want)
	}
}

func TestBootstrapConfigUsesExplicitConfigFile(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	path := filepath.Join(t.TempDir(), "custom.yaml")
	if err := os.WriteFile(path, []byte("database:\n  name: pletka_from_file\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	viper.Set("config", path)

	BootstrapConfig(DefaultConfigBootstrap())

	if got := viper.ConfigFileUsed(); got != path {
		t.Fatalf("ConfigFileUsed() = %q, want %q", got, path)
	}
	if got, want := viper.GetString("database.name"), "pletka_from_file"; got != want {
		t.Fatalf("database.name = %q, want %q", got, want)
	}
}
