package cmd

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// TestInstanceFlag_SharedByWeaveAndProject pins that the weave and project
// command groups expose one and the same --instance flag object. viper keeps
// a single pflag per key, so two separately defined flags would leave one
// group reading the other's unchanged value and silently targeting the
// wrong database.
func TestInstanceFlag_SharedByWeaveAndProject(t *testing.T) {
	root := &cobra.Command{Use: "test"}
	AddOperationalCommands(root, OperationalOptions{})
	weave, _, err := root.Find([]string{"weave", "verify-paths"})
	if err != nil {
		t.Fatal(err)
	}
	project, _, err := root.Find([]string{"project", "load-git"})
	if err != nil {
		t.Fatal(err)
	}
	wf, pf := weave.InheritedFlags().Lookup("instance"), project.InheritedFlags().Lookup("instance")
	if wf == nil || pf == nil {
		t.Fatalf("both groups must carry --instance (weave=%v project=%v)", wf != nil, pf != nil)
	}
	if wf != pf {
		t.Fatalf("weave and project must share one --instance flag object")
	}
	if err := project.ParseFlags([]string{"--instance", "beta"}); err != nil {
		t.Fatal(err)
	}
	if got := viper.GetString("instance"); got != "beta" {
		t.Fatalf("project --instance beta: viper instance = %q", got)
	}
	if err := weave.ParseFlags([]string{"--instance", "gamma"}); err != nil {
		t.Fatal(err)
	}
	if got := viper.GetString("instance"); got != "gamma" {
		t.Fatalf("weave --instance gamma: viper instance = %q", got)
	}
}
