package content

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/pletka-io/pletka/pkg/frontendmanifest"
)

func TestLoaderRendersHeroSubtitleInlineMarkdown(t *testing.T) {
	raw := []byte(`---
slug: about
template: article
blocks:
  - type: hero
    title: About Pletka
    subtitle: "LOUDER semantic data — **L**inked and **O**pen"
---
`)

	schema, err := NewLoader(nil).parse("about", "en", raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(schema.Blocks) != 1 {
		t.Fatalf("blocks len = %d, want 1", len(schema.Blocks))
	}

	got, _ := schema.Blocks[0].Data["subtitle_html"].(string)
	if !strings.Contains(got, "<strong>L</strong>inked") {
		t.Fatalf("subtitle_html = %q, want rendered strong markup", got)
	}
	if strings.Contains(got, "<p>") || strings.Contains(got, "**L**") {
		t.Fatalf("subtitle_html = %q, want inline HTML without markdown markers or paragraph wrapper", got)
	}
}

func TestValidateWidgetReferencesIncludesImplicitProse(t *testing.T) {
	catalog := contentWidgetCatalog(t, "hero", "prose")
	source := NewOverlaySource(fstest.MapFS{
		"pages/home.en.md": &fstest.MapFile{Data: []byte(`---
slug: home
blocks:
  - type: hero
    title: Home
---
Body content.
`)},
	})

	if err := ValidateWidgetReferences(context.Background(), []ContentSource{source}, catalog); err != nil {
		t.Fatalf("ValidateWidgetReferences() error = %v", err)
	}
}

func TestValidateWidgetReferencesRejectsMissingContentWidget(t *testing.T) {
	catalog := contentWidgetCatalog(t, "prose")
	source := NewOverlaySource(fstest.MapFS{
		"pages/home.en.md": &fstest.MapFile{Data: []byte(`---
slug: home
blocks:
  - type: platform-hero
    title: Home
---
`)},
	})

	err := ValidateWidgetReferences(context.Background(), []ContentSource{source}, catalog)
	if err == nil {
		t.Fatal("ValidateWidgetReferences() error = nil, want missing widget")
	}
	if !strings.Contains(err.Error(), `contentWidgets "platform-hero"`) {
		t.Fatalf("error = %q, want missing platform hero", err)
	}
}

func TestConfiguredSourcesIncludesExplicitOverlayPath(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "pages"), 0o755); err != nil {
		t.Fatalf("mkdir pages: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pages", "extra.en.md"), []byte(`---
slug: extra
blocks: []
---
`), 0o644); err != nil {
		t.Fatalf("write overlay page: %v", err)
	}

	sources, err := ConfiguredSources(nil, dir)
	if err != nil {
		t.Fatalf("ConfiguredSources() error = %v", err)
	}
	if len(sources) != 2 {
		t.Fatalf("sources len = %d, want 2", len(sources))
	}

	entries, err := sources[1].List(context.Background())
	if err != nil {
		t.Fatalf("overlay List() error = %v", err)
	}
	if len(entries) != 1 || entries[0].Slug != "extra" {
		t.Fatalf("overlay entries = %#v, want extra", entries)
	}
}

func contentWidgetCatalog(t *testing.T, names ...string) *frontendmanifest.Catalog {
	t.Helper()
	entries := make([]frontendmanifest.Entry, 0, len(names))
	for _, name := range names {
		entries = append(entries, frontendmanifest.Entry{Name: name, Source: name + ".svelte"})
	}
	catalog, err := frontendmanifest.NewCatalog(frontendmanifest.Manifest{ContentWidgets: entries})
	if err != nil {
		t.Fatalf("NewCatalog() error = %v", err)
	}
	return catalog
}
