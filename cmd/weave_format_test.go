package cmd

import (
	"context"
	"io"
	"testing"

	"github.com/pletka-io/pletka/pkg/weave/generators"
	"github.com/pletka-io/pletka/pkg/weave/genwiring"
)

type fakeRenderer struct{ f generators.Format }

func (r fakeRenderer) Spec() generators.FormatSpec {
	return generators.FormatSpec{Format: r.f, ContentType: "text/plain", FileExtension: "txt"}
}
func (r fakeRenderer) Render(_ context.Context, _ *generators.Snapshot, _ io.Writer) error {
	return nil
}

func TestParseGeneratorFormatCoreSet(t *testing.T) {
	available := availableFormats(genwiring.CoreCLIRenderers())
	if _, err := parseGeneratorFormat("turtle", available); err != nil {
		t.Fatalf("turtle should be available in core set: %v", err)
	}
	if _, err := parseGeneratorFormat("shacl", available); err == nil {
		t.Fatal("shacl must be unsupported in the core-only set")
	}
}

// TestParseGeneratorFormatDirectNameFallback pins the alias-drift fix:
// sparql and researchspace are in CoreCLIRenderers() but have no entry in
// parseGeneratorFormat's alias switch, so they must resolve via the
// direct-name fallback against the available set.
func TestParseGeneratorFormatDirectNameFallback(t *testing.T) {
	available := availableFormats(genwiring.CoreCLIRenderers())
	for _, tc := range []struct {
		value string
		want  generators.Format
	}{
		{"sparql", generators.FormatSPARQL},
		{"researchspace", generators.FormatResearchSpace},
	} {
		got, err := parseGeneratorFormat(tc.value, available)
		if err != nil {
			t.Fatalf("parseGeneratorFormat(%q) error: %v", tc.value, err)
		}
		if got != tc.want {
			t.Fatalf("parseGeneratorFormat(%q) = %q, want %q", tc.value, got, tc.want)
		}
	}
}

func TestParseGeneratorFormatInjected(t *testing.T) {
	available := availableFormats(append(genwiring.CoreCLIRenderers(),
		fakeRenderer{f: generators.FormatSHACL}))
	got, err := parseGeneratorFormat("shacl", available)
	if err != nil {
		t.Fatalf("injected shacl renderer must make the format parseable: %v", err)
	}
	if got != generators.FormatSHACL {
		t.Fatalf("want FormatSHACL, got %q", got)
	}
}
