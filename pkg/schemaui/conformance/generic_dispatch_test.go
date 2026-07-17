package conformance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEntitySchemaHandlerUsesProviderRegistry(t *testing.T) {
	source := readRepoFile(t, "pkg/weave/entityschema/handler.go")
	for _, forbidden := range []string{
		`case "model"`,
		`case "collection"`,
		`case "field"`,
		`case "concept-list"`,
		`case "example"`,
	} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("entityschema handler must dispatch through schema providers, found %s", forbidden)
		}
	}
}

func TestWidgetDispatcherUsesWidgetRegistry(t *testing.T) {
	source := readRepoFile(t, "frontend/src/lib/components/form/WidgetDispatcher.svelte")
	for _, forbidden := range []string{
		"field.widget ===",
		"field.widget ==",
	} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("WidgetDispatcher must resolve widgets through the registry, found %q", forbidden)
		}
	}
}

func TestFormRendererUsesWidgetRegistryMetadata(t *testing.T) {
	source := readRepoFile(t, "frontend/src/lib/components/form/FormRenderer.svelte")
	if !strings.Contains(source, "initialValueForWidget(field.widget)") {
		t.Fatal("FormRenderer must get widget-specific initial values from widget registry metadata")
	}
	for _, forbidden := range []string{
		"field.widget ===",
		"field.widget ==",
		"field.widget !==",
		"field.widget !=",
		"switch (field.widget",
	} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("FormRenderer must not branch on concrete widget names, found %q", forbidden)
		}
	}
}

func TestSchemaSectionRenderersUseRegistries(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		required string
		forbid   []string
	}{
		{
			name:     "ontology page",
			path:     "frontend/src/lib/components/ontology/OntologyPage.svelte",
			required: "getOntologySectionWidget(section.widget)",
			forbid: []string{
				"section.widget ===",
				"section.widget ==",
				"{#if section.widget",
				"{:else if section.widget",
				"switch (section.widget",
			},
		},
		{
			name:     "project overview",
			path:     "frontend/src/lib/components/project/ProjectOverview.svelte",
			required: "getProjectOverviewWidget(section.widget)",
			forbid: []string{
				"section.widget ===",
				"section.widget ==",
				"{#if section.widget",
				"{:else if section.widget",
				"switch (section.widget",
			},
		},
		{
			name:     "entity-list filters",
			path:     "frontend/src/lib/components/entity-list/FilterDrawer.svelte",
			required: "getFilterWidget(widget)",
			forbid: []string{
				"widget ===",
				"widget ==",
				"{#if widget",
				"{:else if widget",
				"switch (widget",
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			source := readRepoFile(t, tc.path)
			if !strings.Contains(source, tc.required) {
				t.Fatalf("%s must dispatch through registry resolver %q", tc.path, tc.required)
			}
			for _, forbidden := range tc.forbid {
				if strings.Contains(source, forbidden) {
					t.Fatalf("%s must dispatch through a registry, found %q", tc.path, forbidden)
				}
			}
		})
	}
}

func TestDetailViewGroupSemanticsUseHelper(t *testing.T) {
	for _, rel := range []string{
		"frontend/src/lib/detailview/components/CategorySection.svelte",
		"frontend/src/lib/detailview/components/CollectionGroup.svelte",
		"frontend/src/lib/detailview/components/EditableGroup.svelte",
		"frontend/src/lib/detailview/components/OverrideEditor.svelte",
		"frontend/src/lib/detailview/override-editor-state.svelte.ts",
	} {
		t.Run(rel, func(t *testing.T) {
			source := readRepoFile(t, rel)
			for _, forbidden := range []string{
				"widget === 'collection-group'",
				"widget === \"collection-group\"",
				"widget === 'field-group'",
				"widget === \"field-group\"",
				"widget !== 'collection-group'",
				"widget !== \"collection-group\"",
				"widget !== 'field-group'",
				"widget !== \"field-group\"",
				"widget == 'collection-group'",
				"widget == \"collection-group\"",
				"widget == 'field-group'",
				"widget == \"field-group\"",
				"widget != 'collection-group'",
				"widget != \"collection-group\"",
				"widget != 'field-group'",
				"widget != \"field-group\"",
			} {
				if strings.Contains(source, forbidden) {
					t.Fatalf("%s must use detailview group-kind helpers, found %q", rel, forbidden)
				}
			}
		})
	}
}

func TestEntityListViewDoesNotSpecialCaseEntityTypes(t *testing.T) {
	source := readRepoFile(t, "frontend/src/lib/components/entity-list/EntityListView.svelte")
	for _, forbidden := range []string{
		`schema.entity_type === 'example'`,
		`schema.entity_type === "example"`,
	} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("EntityListView must use schema capabilities or registries for entity-specific behavior, found %q", forbidden)
		}
	}
}

func readRepoFile(t *testing.T, rel string) string {
	t.Helper()
	path := filepath.Join(repoRoot(t), filepath.FromSlash(rel))
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(data)
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
