package frontendmanifest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCatalogCombinesCoreAndPlatformEntries(t *testing.T) {
	core := Manifest{
		Path: "core.frontend.json",
		Core: true,
		Islands: []Entry{
			{Name: "entity-list", Source: "entity-list.ts"},
		},
		FormWidgets: []Entry{
			{Name: "select", Source: "Select.svelte"},
		},
	}
	platform := Manifest{
		Path: "platform.frontend.json",
		Islands: []Entry{
			{Name: "platform-dashboard", Source: "platform-dashboard.ts"},
		},
		EntityListRowWidgets: []Entry{
			{Name: "platform-card", Source: "PlatformCard.svelte"},
		},
	}

	catalog, err := NewCatalog(core, platform)
	if err != nil {
		t.Fatalf("NewCatalog() error = %v", err)
	}
	for _, tc := range []struct {
		kind Kind
		name string
	}{
		{KindIsland, "entity-list"},
		{KindIsland, "platform-dashboard"},
		{KindFormWidget, "select"},
		{KindEntityListRowWidget, "platform-card"},
	} {
		if !catalog.Has(tc.kind, tc.name) {
			t.Fatalf("catalog missing %s %q", tc.kind, tc.name)
		}
	}
}

func TestCatalogRejectsDuplicateWithoutOverride(t *testing.T) {
	core := Manifest{
		Path: "core.frontend.json",
		Core: true,
		FormWidgets: []Entry{
			{Name: "select", Source: "Select.svelte"},
		},
	}
	platform := Manifest{
		Path: "platform.frontend.json",
		FormWidgets: []Entry{
			{Name: "select", Source: "PlatformSelect.svelte"},
		},
	}

	_, err := NewCatalog(core, platform)
	if err == nil {
		t.Fatal("NewCatalog() error = nil, want duplicate error")
	}
	if !strings.Contains(err.Error(), `overrides": "core"`) {
		t.Fatalf("error = %q, want override hint", err)
	}
}

func TestCatalogRejectsDuplicatePlatformEntries(t *testing.T) {
	first := Manifest{
		Path: "platform-a.frontend.json",
		Islands: []Entry{
			{Name: "platform-dashboard", Source: "DashboardA.ts"},
		},
	}
	second := Manifest{
		Path: "platform-b.frontend.json",
		Islands: []Entry{
			{Name: "platform-dashboard", Source: "DashboardB.ts"},
		},
	}

	_, err := NewCatalog(first, second)
	if err == nil {
		t.Fatal("NewCatalog() error = nil, want duplicate platform error")
	}
	if !strings.Contains(err.Error(), `duplicate islands "platform-dashboard"`) {
		t.Fatalf("error = %q, want duplicate platform island", err)
	}
}

func TestCatalogAllowsExplicitCoreOverride(t *testing.T) {
	core := Manifest{
		Path: "core.frontend.json",
		Core: true,
		FormWidgets: []Entry{
			{Name: "select", Source: "Select.svelte"},
		},
	}
	platform := Manifest{
		Path: "platform.frontend.json",
		FormWidgets: []Entry{
			{Name: "select", Source: "PlatformSelect.svelte", Overrides: "core"},
		},
	}

	catalog, err := NewCatalog(core, platform)
	if err != nil {
		t.Fatalf("NewCatalog() error = %v", err)
	}
	if !catalog.Has(KindFormWidget, "select") {
		t.Fatal("catalog missing overridden select widget")
	}
}

func TestCatalogValidatesReferencesAcrossCoreAndPlatform(t *testing.T) {
	catalog, err := NewCatalog(
		Manifest{
			Path: "core.frontend.json",
			Islands: []Entry{
				{Name: "entity-list", Source: "entity-list.ts"},
			},
		},
		Manifest{
			Path: "platform.frontend.json",
			Islands: []Entry{
				{Name: "platform-dashboard", Source: "platform-dashboard.ts"},
			},
			FormWidgets: []Entry{
				{Name: "platform-lookup", Source: "PlatformLookup.svelte"},
			},
		},
	)
	if err != nil {
		t.Fatalf("NewCatalog() error = %v", err)
	}

	err = catalog.ValidateReferences([]Reference{
		{Kind: KindIsland, Name: "entity-list", Source: "core page"},
		{Kind: KindIsland, Name: "platform-dashboard", Source: "platform page"},
		{Kind: KindFormWidget, Name: "platform-lookup", Source: "platform form"},
	})
	if err != nil {
		t.Fatalf("ValidateReferences() error = %v", err)
	}
}

func TestCatalogValidateFilesUsesManifestRelativePlatformSources(t *testing.T) {
	root := t.TempDir()
	coreDir := filepath.Join(root, "core")
	platformDir := filepath.Join(root, "platform")
	if err := os.MkdirAll(filepath.Join(coreDir, "src", "islands"), 0o755); err != nil {
		t.Fatalf("mkdir core: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(platformDir, "widgets"), 0o755); err != nil {
		t.Fatalf("mkdir platform: %v", err)
	}
	if err := os.WriteFile(filepath.Join(coreDir, "src", "islands", "entity-list.ts"), []byte("export {};"), 0o644); err != nil {
		t.Fatalf("write core source: %v", err)
	}
	if err := os.WriteFile(filepath.Join(platformDir, "widgets", "PlatformLookup.svelte"), []byte("<script></script>"), 0o644); err != nil {
		t.Fatalf("write platform source: %v", err)
	}

	catalog, err := NewCatalog(
		Manifest{
			Path: filepath.Join(coreDir, "core.frontend.json"),
			Core: true,
			Islands: []Entry{
				{Name: "entity-list", Source: "./src/islands/entity-list.ts"},
			},
		},
		Manifest{
			Path: filepath.Join(platformDir, "platform.frontend.json"),
			FormWidgets: []Entry{
				{Name: "platform-lookup", Source: "./widgets/PlatformLookup.svelte"},
			},
		},
	)
	if err != nil {
		t.Fatalf("NewCatalog() error = %v", err)
	}
	if err := catalog.ValidateFiles(); err != nil {
		t.Fatalf("ValidateFiles() error = %v", err)
	}
}

func TestCatalogValidateFilesFailsForMissingPlatformSource(t *testing.T) {
	platformDir := t.TempDir()
	catalog, err := NewCatalog(Manifest{
		Path: filepath.Join(platformDir, "platform.frontend.json"),
		FormWidgets: []Entry{
			{Name: "platform-lookup", Source: "./widgets/Missing.svelte"},
		},
	})
	if err != nil {
		t.Fatalf("NewCatalog() error = %v", err)
	}

	err = catalog.ValidateFiles()
	if err == nil {
		t.Fatal("ValidateFiles() error = nil, want missing source error")
	}
	if !strings.Contains(err.Error(), `formWidgets "platform-lookup" source`) {
		t.Fatalf("error = %q, want missing platform source", err)
	}
}

func TestCatalogValidatesReferences(t *testing.T) {
	catalog, err := NewCatalog(Manifest{
		Path: "core.frontend.json",
		Islands: []Entry{
			{Name: "entity-list", Source: "entity-list.ts"},
		},
	})
	if err != nil {
		t.Fatalf("NewCatalog() error = %v", err)
	}

	err = catalog.ValidateReferences([]Reference{
		{Kind: KindIsland, Name: "entity-list", Source: "test"},
		{Kind: KindIsland, Name: "missing-island", Source: "test"},
	})
	if err == nil {
		t.Fatal("ValidateReferences() error = nil, want missing reference")
	}
	if !strings.Contains(err.Error(), `islands "missing-island"`) {
		t.Fatalf("error = %q, want missing island", err)
	}
}
