package app_test

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/pkg/app"
)

func TestNewRequiresPool(t *testing.T) {
	if _, err := app.New(context.Background(), app.Options{}); err == nil {
		t.Fatal("New accepted options without a pgx pool")
	}
}

func TestNewGeneratorRuntimeRequiresPool(t *testing.T) {
	if _, err := app.NewGeneratorRuntime(app.Options{}); err == nil {
		t.Fatal("NewGeneratorRuntime accepted options without a pgx pool")
	}
}

func TestValidatePresentation(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := app.ValidatePresentation(context.Background(), app.PresentationOptions{
		Logger:                   logger,
		CoreFrontendManifestPath: filepath.Join(repoRoot(t), "frontend/core.frontend.json"),
	}); err != nil {
		t.Fatalf("ValidatePresentation() error = %v", err)
	}
}

func TestStaticAssetSetValidate(t *testing.T) {
	tests := []struct {
		name string
		in   app.StaticAssetSet
	}{
		{name: "missing id", in: app.StaticAssetSet{FS: fstest.MapFS{}}},
		{name: "missing filesystem", in: app.StaticAssetSet{ID: "x"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.in.Validate(); err == nil {
				t.Fatal("Validate returned nil error")
			}
		})
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(wd, "go.mod")); err == nil {
			return wd
		}
		parent := filepath.Dir(wd)
		if parent == wd {
			t.Fatal("could not find repo root")
		}
		wd = parent
	}
}

func TestRouteContributionValidate(t *testing.T) {
	tests := []struct {
		name string
		in   app.RouteContribution
	}{
		{name: "missing id", in: app.RouteContribution{Mount: func(chi.Router, app.Host) {}}},
		{name: "missing mount", in: app.RouteContribution{ID: "x"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.in.Validate(); err == nil {
				t.Fatal("Validate returned nil error")
			}
		})
	}
}
