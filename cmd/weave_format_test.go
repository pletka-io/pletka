package cmd

import (
	"context"
	"io"
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
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

type needsRenderer struct{}

func (needsRenderer) Spec() generators.FormatSpec {
	return generators.FormatSpec{Format: generators.FormatTurtle, IncludeReferenced: true, IncludeClassAncestors: true}
}
func (needsRenderer) Render(_ context.Context, _ *generators.Snapshot, _ io.Writer) error { return nil }

// --snapshot-for builds the snapshot dump with what that format's renderer
// asks for, so a dump shows exactly what the renderer would get.
func TestSnapshotForAppliesTheRenderersOptions(t *testing.T) {
	renderers := []generators.Renderer{needsRenderer{}}
	got, err := snapshotOptions(generators.Options{BasePrefix: "ex"}, "turtle", true, renderers)
	if err != nil {
		t.Fatal(err)
	}
	if !got.IncludeReferenced || !got.IncludeClassAncestors || got.BasePrefix != "ex" {
		t.Fatalf("options = %+v", got)
	}
	if got, _ := snapshotOptions(generators.Options{}, "", true, renderers); got.IncludeReferenced {
		t.Fatal("no --snapshot-for must leave the options alone")
	}
	if _, err := snapshotOptions(generators.Options{}, "nope", true, renderers); err == nil {
		t.Fatal("want an error for an unknown format")
	}
	if _, err := snapshotOptions(generators.Options{}, "turtle", false, renderers); err == nil {
		t.Fatal("want an error for --snapshot-for without a snapshot format")
	}
}

// two models with the same system name get distinct files: the colliding
// ones carry their semantic id, unique names stay as they were
func TestModelFilenamesAreUnique(t *testing.T) {
	m := func(id, sem, sys string) *domain.Model {
		return &domain.Model{Entity: domain.Entity{ID: id, SemanticID: sem, SystemName: sys}}
	}
	got := modelFilenames([]*domain.Model{m("1", "PM.1", "person"), m("2", "PM.2", "event"), m("3", "PM.3", "event"), m("4", "", "")}, ".ttl")
	want := []string{"person.ttl", "event_PM.2.ttl", "event_PM.3.ttl", "4.ttl"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}
