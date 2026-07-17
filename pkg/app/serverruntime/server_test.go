package serverruntime

import (
	"testing"
	"testing/fstest"

	"github.com/spf13/viper"

	"github.com/pletka-io/pletka/pkg/app"
	"github.com/pletka-io/pletka/pkg/database"
)

func TestApplyInstanceOverrides(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	cfg := ApplyInstanceOverrides(Config{
		Instance:       "demo",
		AllowedOrigins: []string{"https://configured.example.test"},
		Database: database.Settings{
			Name: "pletka_config",
		},
	})

	if got, want := cfg.Database.Name, "pletka_demo"; got != want {
		t.Fatalf("Database.Name = %q, want %q", got, want)
	}
	if got, want := cfg.AllowedOrigins, []string{"https://demo.pletka.io", "https://configured.example.test"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("AllowedOrigins = %#v, want %#v", got, want)
	}
	if got := viper.GetString("database.name"); got != "" {
		t.Fatalf("viper database.name was mutated to %q", got)
	}
	if got := viper.GetStringSlice("cors.allowed_origins"); len(got) != 0 {
		t.Fatalf("viper cors.allowed_origins was mutated to %#v", got)
	}
}

func TestApplyInstanceOverridesKeepsDatabaseEnvOverride(t *testing.T) {
	t.Setenv("PLETKA_DB_NAME", "pletka_env")

	cfg := ApplyInstanceOverrides(Config{
		Instance: "demo",
		Database: database.Settings{
			Name: "pletka_config",
		},
	})

	if got, want := cfg.Database.Name, "pletka_env"; got != want {
		t.Fatalf("Database.Name = %q, want env override %q", got, want)
	}
}

func TestConfigFromViperIncludesAppMiddlewareConfig(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	viper.Set("env", "production")
	viper.Set("server.validate_only", true)
	viper.Set("log.static_files", true)
	viper.Set("cors.allowed_origins", []string{"https://example.test"})
	viper.Set("features.registration", true)
	viper.Set("content.overlay_path", "/tmp/content")

	cfg := ConfigFromViper()

	if cfg.Env != "production" {
		t.Fatalf("Env = %q, want production", cfg.Env)
	}
	if !cfg.ValidateOnly {
		t.Fatal("ValidateOnly = false, want true")
	}
	if !cfg.LogStaticFiles {
		t.Fatal("LogStaticFiles = false, want true")
	}
	if got, want := cfg.AllowedOrigins, []string{"https://example.test"}; len(got) != 1 || got[0] != want[0] {
		t.Fatalf("AllowedOrigins = %#v, want %#v", got, want)
	}
	if !cfg.RegistrationEnabled {
		t.Fatal("RegistrationEnabled = false, want true")
	}
	if got, want := cfg.ContentOverlayPath, "/tmp/content"; got != want {
		t.Fatalf("ContentOverlayPath = %q, want %q", got, want)
	}
}

func TestConfigPresentationOptions(t *testing.T) {
	tests := []struct {
		name string
		env  string
		want bool
	}{
		{name: "development", env: "development", want: true},
		{name: "production", env: "production", want: false},
		{name: "empty", env: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Config{Env: tt.env}.PresentationOptions(nil)
			if got.DevMode != tt.want {
				t.Fatalf("DevMode = %v, want %v", got.DevMode, tt.want)
			}
		})
	}
}

func TestConfigPresentationOptionsIncludesContentOverlay(t *testing.T) {
	got := Config{ContentOverlayPath: "/tmp/content"}.PresentationOptions(nil)
	if got.ContentOverlayPath != "/tmp/content" {
		t.Fatalf("ContentOverlayPath = %q, want /tmp/content", got.ContentOverlayPath)
	}
}

func TestConfigPresentationOptionsIncludesFrontendAssets(t *testing.T) {
	cfg := Config{
		FrontendManifestPaths:    []string{"/tmp/platform.frontend.json"},
		CoreFrontendManifestPath: "/tmp/core.frontend.json",
		StaticAssets: []app.StaticAssetSet{
			{ID: "platform", FS: fstest.MapFS{}},
		},
		FrontendManifests: []app.FrontendManifestSet{
			{ID: "platform", FS: fstest.MapFS{}, Path: "frontend/platform.frontend.json"},
		},
	}

	got := cfg.PresentationOptions(nil)
	if len(got.FrontendManifestPaths) != 1 || got.FrontendManifestPaths[0] != "/tmp/platform.frontend.json" {
		t.Fatalf("FrontendManifestPaths = %#v", got.FrontendManifestPaths)
	}
	if got.CoreFrontendManifestPath != "/tmp/core.frontend.json" {
		t.Fatalf("CoreFrontendManifestPath = %q", got.CoreFrontendManifestPath)
	}
	if len(got.StaticAssets) != 1 || got.StaticAssets[0].ID != "platform" {
		t.Fatalf("StaticAssets = %#v", got.StaticAssets)
	}
	if len(got.FrontendManifests) != 1 || got.FrontendManifests[0].ID != "platform" {
		t.Fatalf("FrontendManifests = %#v", got.FrontendManifests)
	}
}

func TestConfigFromViperIncludesDatabaseSettings(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	t.Setenv("PLETKA_DB_NAME", "pletka_env")

	viper.Set("database.host", "db.example.test")
	viper.Set("database.port", 15432)
	viper.Set("database.name", "pletka_config")
	viper.Set("database.user", "pletka_user")
	viper.Set("database.password", "secret")
	viper.Set("database.sslmode", "require")

	cfg := ConfigFromViper()

	if got, want := cfg.Database.Host, "db.example.test"; got != want {
		t.Fatalf("Database.Host = %q, want %q", got, want)
	}
	if got, want := cfg.Database.Port, 15432; got != want {
		t.Fatalf("Database.Port = %d, want %d", got, want)
	}
	if got, want := cfg.Database.Name, "pletka_env"; got != want {
		t.Fatalf("Database.Name = %q, want env override %q", got, want)
	}
	if got, want := cfg.Database.User, "pletka_user"; got != want {
		t.Fatalf("Database.User = %q, want %q", got, want)
	}
	if got, want := cfg.Database.Password, "secret"; got != want {
		t.Fatalf("Database.Password = %q, want %q", got, want)
	}
	if got, want := cfg.Database.SSLMode, "require"; got != want {
		t.Fatalf("Database.SSLMode = %q, want %q", got, want)
	}
}

func TestGitDataDirDefault(t *testing.T) {
	if got, want := gitDataDir(Config{}), "./data/git-projects"; got != want {
		t.Fatalf("gitDataDir(empty) = %q, want %q", got, want)
	}
	if got, want := gitDataDir(Config{GitDataDir: "/tmp/projects"}), "/tmp/projects"; got != want {
		t.Fatalf("gitDataDir(custom) = %q, want %q", got, want)
	}
}
