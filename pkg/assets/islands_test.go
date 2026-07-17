package assets

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/google/go-cmp/cmp"
	"github.com/pletka-io/pletka/pkg/frontendmanifest"
)

func TestIslandResolverResolveLegacyManifestKey(t *testing.T) {
	r := &IslandResolver{}
	r.parseManifest([]byte(`{
		"src/islands/entity-list.ts": {
			"file": "entity-list-abc123.js",
			"name": "entity-list",
			"src": "src/islands/entity-list.ts",
			"isEntry": true,
			"css": ["assets/entity-list-abc123.css"]
		}
	}`))

	if got, want := r.Resolve("entity-list"), "/static/dist/entity-list-abc123.js"; got != want {
		t.Fatalf("Resolve() = %q, want %q", got, want)
	}
	gotCSS, wantCSS := r.ResolveCSS("entity-list"), []string{"/static/dist/assets/entity-list-abc123.css"}
	if diff := cmp.Diff(wantCSS, gotCSS); diff != "" {
		t.Fatalf("ResolveCSS() mismatch (-want +got):\n%s", diff)
	}
}

func TestIslandResolverResolveManifestEntryName(t *testing.T) {
	r := &IslandResolver{}
	r.parseManifest([]byte(`{
		"../platform/frontend/src/islands/arches-dashboard.ts": {
			"file": "arches-dashboard-def456.js",
			"name": "arches-dashboard",
			"src": "../platform/frontend/src/islands/arches-dashboard.ts",
			"isEntry": true
		},
		"_ArchesPanel.js": {
			"file": "chunks/ArchesPanel.js",
			"name": "ArchesPanel"
		}
	}`))

	if got, want := r.Resolve("arches-dashboard"), "/static/dist/arches-dashboard-def456.js"; got != want {
		t.Fatalf("Resolve() = %q, want %q", got, want)
	}
	if got := r.Resolve("ArchesPanel"); got != "" {
		t.Fatalf("Resolve() for non-entry chunk = %q, want empty", got)
	}
}

func TestIslandResolverComposesContributedViteManifests(t *testing.T) {
	r := NewIslandResolverWithStaticFS(false, fstest.MapFS{
		"dist/.vite/manifest.json": {Data: []byte(`{
		  "src/islands/entity-list.ts": {
		    "file": "entity-list-platform.js",
		    "name": "entity-list",
		    "src": "src/islands/entity-list.ts",
		    "isEntry": true
		  },
		  "src/islands/platform-dashboard.ts": {
		    "file": "platform-dashboard.js",
		    "name": "platform-dashboard",
		    "src": "src/islands/platform-dashboard.ts",
		    "isEntry": true
		  }
		}`)},
	})

	if got, want := r.Resolve("platform-dashboard"), "/static/dist/platform-dashboard.js"; got != want {
		t.Fatalf("Resolve(platform-dashboard) = %q, want %q", got, want)
	}
	if got, want := r.Resolve("entity-list"), "/static/dist/entity-list-platform.js"; got != want {
		t.Fatalf("Resolve(entity-list) = %q, want platform override %q", got, want)
	}
	if got := r.Resolve("admin-shell"); got == "" {
		t.Fatal("Resolve(admin-shell) did not fall back to core embedded manifest")
	}
}

func TestIslandResolverValidatesEmbeddedPlatformManifest(t *testing.T) {
	coreManifest := filepath.Join(t.TempDir(), "core.frontend.json")
	mustWrite(t, coreManifest, `{}`)
	platformManifest, err := frontendmanifest.LoadBytes("embedded:platform:frontend/platform.frontend.json", []byte(`{
  "islands": [
    {"name": "platform-dashboard", "source": "./src/islands/platform-dashboard.ts"}
  ],
  "formWidgets": [
    {"name": "platform-lookup", "source": "./src/widgets/PlatformLookup.svelte"}
  ]
}`), false)
	if err != nil {
		t.Fatalf("LoadBytes() error = %v", err)
	}

	r := NewIslandResolverWithStaticFS(false, fstest.MapFS{
		"dist/.vite/manifest.json": {Data: []byte(`{
		  "src/islands/platform-dashboard.ts": {
		    "file": "platform-dashboard.js",
		    "name": "platform-dashboard",
		    "src": "src/islands/platform-dashboard.ts",
		    "isEntry": true
		  }
		}`)},
	})

	err = r.ValidateFrontendManifests(FrontendManifestValidation{
		CoreManifestPath:    coreManifest,
		PlatformManifests:   []frontendmanifest.Manifest{platformManifest},
		ValidateSourceFiles: true,
	})
	if err != nil {
		t.Fatalf("ValidateFrontendManifests() error = %v", err)
	}

	catalog, err := FrontendManifestCatalog(FrontendManifestValidation{
		CoreManifestPath:    coreManifest,
		PlatformManifests:   []frontendmanifest.Manifest{platformManifest},
		ValidateSourceFiles: true,
	})
	if err != nil {
		t.Fatalf("FrontendManifestCatalog() error = %v", err)
	}
	if !catalog.Has(frontendmanifest.KindIsland, "platform-dashboard") {
		t.Fatal("embedded platform manifest missing platform-dashboard island")
	}
	if !catalog.Has(frontendmanifest.KindFormWidget, "platform-lookup") {
		t.Fatal("embedded platform manifest missing platform-lookup form widget")
	}
}

func TestFrontendManifestPathsFromEnv(t *testing.T) {
	t.Setenv("PLETKA_FRONTEND_MANIFESTS", "examples/platform.frontend.json")
	t.Setenv("PLETKA_FRONTEND_CONTRIBUTIONS", filepath.Join(t.TempDir(), "extra.frontend.json"))

	paths := FrontendManifestPathsFromEnv()
	if len(paths) != 2 {
		t.Fatalf("FrontendManifestPathsFromEnv() len = %d, want 2: %#v", len(paths), paths)
	}
	if got, want := filepath.ToSlash(paths[0]), "frontend/examples/platform.frontend.json"; got != want {
		t.Fatalf("relative env path = %q, want %q", got, want)
	}
	if runtime.GOOS != "windows" && !filepath.IsAbs(paths[1]) {
		t.Fatalf("absolute env path became relative: %q", paths[1])
	}
}

func TestEmbeddedCoreFrontendManifestFallback(t *testing.T) {
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(originalDir); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	})
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}

	catalog, err := FrontendManifestCatalog(FrontendManifestValidation{
		ValidateSourceFiles: true,
	})
	if err != nil {
		t.Fatalf("FrontendManifestCatalog() error = %v", err)
	}
	for _, name := range []string{"hero", "prose", "feature_grid", "cta_strip", "quote", "toc", "actions"} {
		if !catalog.Has(frontendmanifest.KindContentWidget, name) {
			t.Fatalf("embedded catalog missing content widget %q", name)
		}
	}
}

func TestEmbeddedCoreFrontendManifestMatchesSourceFile(t *testing.T) {
	root := repoRoot(t)
	source, err := os.ReadFile(filepath.Join(root, "frontend", "core.frontend.json"))
	if err != nil {
		t.Fatalf("read source manifest: %v", err)
	}
	embedded, err := staticFiles.ReadFile("frontend/core.frontend.json")
	if err != nil {
		t.Fatalf("read embedded manifest: %v", err)
	}
	if string(embedded) != string(source) {
		t.Fatal("pkg/assets/frontend/core.frontend.json must match frontend/core.frontend.json")
	}
}

func TestIslandResolverValidateFrontendManifests(t *testing.T) {
	root := t.TempDir()
	coreDir := filepath.Join(root, "frontend")
	platformDir := filepath.Join(root, "platform")
	mustWrite(t, filepath.Join(coreDir, "src", "islands", "entity-list.ts"), "export {};")
	mustWrite(t, filepath.Join(coreDir, "src", "global", "global-js.ts"), "export {};")
	mustWrite(t, filepath.Join(coreDir, "src", "lib", "components", "form", "widgets", "SelectWidget.svelte"), "<select></select>")
	mustWrite(t, filepath.Join(platformDir, "src", "islands", "platform-dashboard.ts"), "export {};")
	mustWrite(t, filepath.Join(platformDir, "src", "widgets", "PlatformLookup.svelte"), "<input />")

	coreManifest := filepath.Join(coreDir, "core.frontend.json")
	platformManifest := filepath.Join(platformDir, "platform.frontend.json")
	mustWrite(t, coreManifest, `{
  "globalEntries": [
    {"name": "global-js", "source": "./src/global/global-js.ts"}
  ],
  "islands": [
    {"name": "entity-list", "source": "./src/islands/entity-list.ts"}
  ],
  "formWidgets": [
    {"name": "select", "source": "./src/lib/components/form/widgets/SelectWidget.svelte"}
  ]
}`)
	mustWrite(t, platformManifest, `{
  "islands": [
    {"name": "platform-dashboard", "source": "./src/islands/platform-dashboard.ts"}
  ],
  "formWidgets": [
    {"name": "platform-lookup", "source": "./src/widgets/PlatformLookup.svelte"}
  ]
}`)

	r := &IslandResolver{}
	r.parseManifest([]byte(`{
  "global-js": {"file": "global-js.js", "name": "global-js", "isEntry": true},
  "entity-list": {"file": "entity-list.js", "name": "entity-list", "isEntry": true},
  "platform-dashboard": {"file": "platform-dashboard.js", "name": "platform-dashboard", "isEntry": true}
}`))
	if err := r.ValidateFrontendManifests(FrontendManifestValidation{
		CoreManifestPath:      coreManifest,
		PlatformManifestPaths: []string{platformManifest},
		ValidateSourceFiles:   true,
	}); err != nil {
		t.Fatalf("ValidateFrontendManifests() error = %v", err)
	}
}

func TestIslandResolverValidateFrontendManifestsRejectsMissingViteEntry(t *testing.T) {
	root := t.TempDir()
	coreManifest := filepath.Join(root, "core.frontend.json")
	mustWrite(t, coreManifest, `{}`)
	mustWrite(t, filepath.Join(root, "src", "islands", "platform-dashboard.ts"), "export {};")
	platformManifest := filepath.Join(root, "platform.frontend.json")
	mustWrite(t, platformManifest, `{
  "islands": [
    {"name": "platform-dashboard", "source": "./src/islands/platform-dashboard.ts"}
  ]
}`)

	r := &IslandResolver{}
	r.parseManifest([]byte(`{}`))
	err := r.ValidateFrontendManifests(FrontendManifestValidation{
		CoreManifestPath:      coreManifest,
		PlatformManifestPaths: []string{platformManifest},
		ValidateSourceFiles:   true,
	})
	if err == nil {
		t.Fatal("ValidateFrontendManifests() error = nil, want missing Vite entry")
	}
	if !strings.Contains(err.Error(), `islands "platform-dashboard"`) {
		t.Fatalf("error = %q, want missing platform dashboard", err)
	}
}

func TestIslandResolverValidateFrontendManifestsRejectsMissingPlatformSource(t *testing.T) {
	root := t.TempDir()
	coreManifest := filepath.Join(root, "core.frontend.json")
	mustWrite(t, coreManifest, `{}`)
	platformManifest := filepath.Join(root, "platform.frontend.json")
	mustWrite(t, platformManifest, `{
  "formWidgets": [
    {"name": "platform-lookup", "source": "./src/widgets/Missing.svelte"}
  ]
}`)

	r := &IslandResolver{}
	r.parseManifest([]byte(`{}`))
	err := r.ValidateFrontendManifests(FrontendManifestValidation{
		CoreManifestPath:      coreManifest,
		PlatformManifestPaths: []string{platformManifest},
		ValidateSourceFiles:   true,
	})
	if err == nil {
		t.Fatal("ValidateFrontendManifests() error = nil, want missing source")
	}
	if !strings.Contains(err.Error(), `formWidgets "platform-lookup" source`) {
		t.Fatalf("error = %q, want missing platform source", err)
	}
}

func mustWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
}
