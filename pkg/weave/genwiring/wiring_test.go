package genwiring

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/pletka-io/pletka/pkg/weave/generators"
)

func TestCoreRenderersFormats(t *testing.T) {
	got := rendererFormats(CoreRenderers())
	want := []generators.Format{
		generators.FormatTurtle,
		generators.FormatJSONLD,
		generators.FormatMermaid,
		generators.FormatExportGraph,
		generators.FormatSPARQL,
		generators.FormatX3ML,
		generators.FormatX3MLB,
		generators.FormatResearchSpace,
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("CoreRenderers formats mismatch (-want +got):\n%s", diff)
	}
}

func TestCoreCLIRenderersFormats(t *testing.T) {
	got := rendererFormats(CoreCLIRenderers())
	want := []generators.Format{
		generators.FormatCSV,
		generators.FormatTurtle,
		generators.FormatJSONLD,
		generators.FormatMermaid,
		generators.FormatExportGraph,
		generators.FormatSPARQL,
		generators.FormatX3ML,
		generators.FormatX3MLB,
		generators.FormatResearchSpace,
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("CoreCLIRenderers formats mismatch (-want +got):\n%s", diff)
	}
}

func TestSharedRendererSetsAreRegistryValid(t *testing.T) {
	for name, renderers := range map[string][]generators.Renderer{
		"core":     CoreRenderers(),
		"core-cli": CoreCLIRenderers(),
	} {
		if _, err := generators.NewRegistry(renderers...); err != nil {
			t.Fatalf("%s renderers rejected by registry: %v", name, err)
		}
	}
}

func TestRendererRegistryRejectsDuplicateSharedFormats(t *testing.T) {
	renderers := CoreRenderers()
	renderers = append(renderers, CoreRenderers()[0])
	if _, err := generators.NewRegistry(renderers...); err == nil {
		t.Fatal("NewRegistry accepted duplicate renderer format")
	}
}

func rendererFormats(renderers []generators.Renderer) []generators.Format {
	out := make([]generators.Format, 0, len(renderers))
	for _, renderer := range renderers {
		out = append(out, renderer.Spec().Format)
	}
	return out
}
