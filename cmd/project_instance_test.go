package cmd

import "testing"

// TestProjectCommand_InstanceFlag pins that the project command group carries
// the same persistent --instance flag as weave, so load-git, init-git and
// reconcile can target an instance database by name.
func TestProjectCommand_InstanceFlag(t *testing.T) {
	root := newProjectCommand()
	if root.PersistentFlags().Lookup("instance") == nil {
		t.Fatalf("project command has no persistent --instance flag")
	}
	for _, sub := range []string{"load-git", "init-git", "reconcile"} {
		c, _, err := root.Find([]string{sub})
		if err != nil || c == nil {
			t.Fatalf("subcommand %s not found: %v", sub, err)
		}
		if c.InheritedFlags().Lookup("instance") == nil {
			t.Errorf("%s does not inherit --instance", sub)
		}
	}
}
