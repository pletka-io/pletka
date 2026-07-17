package cmd

import (
	"testing"

	"github.com/spf13/cobra"

	"github.com/pletka-io/pletka/pkg/weave/generators"
	"github.com/pletka-io/pletka/pkg/weave/genwiring"
)

func TestParseGeneratorFormatExportGraph(t *testing.T) {
	available := availableFormats(genwiring.CoreCLIRenderers())
	for _, value := range []string{"exportgraph", "export-graph", "graph", "templategraph", "template-graph"} {
		got, err := parseGeneratorFormat(value, available)
		if err != nil {
			t.Fatalf("parseGeneratorFormat(%q) error: %v", value, err)
		}
		if got != generators.FormatExportGraph {
			t.Fatalf("parseGeneratorFormat(%q) = %q, want %q", value, got, generators.FormatExportGraph)
		}
	}
}

// TestAddOperationalCommandsRegistersExpectedCommands verifies that a
// hosted-platform wrapper calling AddOperationalCommands on its own fresh
// root gets exactly the operational subcommands core promises — and that
// serve and pages, which the doc comment says are deliberately excluded,
// are absent.
func TestAddOperationalCommandsRegistersExpectedCommands(t *testing.T) {
	root := &cobra.Command{Use: "pletkactl-test"}
	AddOperationalCommands(root, OperationalOptions{})

	for _, args := range [][]string{
		{"project", "init-git"},
		{"project", "load-git"},
		{"db", "migrate"},
		{"db", "migrate", "status"},
		{"db", "migrate", "down"},
		{"weave", "generate"},
		{"weave", "verify-paths"},
	} {
		if _, _, err := root.Find(args); err != nil {
			t.Fatalf("find %v: %v", args, err)
		}
	}

	for _, args := range [][]string{
		{"serve"},
		{"pages", "extract"},
		{"pages", "hydrate"},
		{"pages", "promote"},
	} {
		cmd, _, err := root.Find(args)
		if err == nil && cmd.Name() == args[len(args)-1] {
			t.Fatalf("expected %v to be absent from operational commands, but it was found", args)
		}
	}
}

func TestRootCommandRegistersExpectedCommands(t *testing.T) {
	root := newRootCommand()
	for _, args := range [][]string{
		{"serve"},
		{"weave", "generate"},
		{"weave", "verify-paths"},
		{"project", "init-git"},
		{"project", "load-git"},
		{"db", "migrate"},
		{"db", "migrate", "status"},
		{"pages", "extract"},
		{"pages", "hydrate"},
		{"pages", "promote"},
	} {
		if _, _, err := root.Find(args); err != nil {
			t.Fatalf("find %v: %v", args, err)
		}
	}
}
