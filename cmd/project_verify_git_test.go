package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestVerifyGitCommand_JSONOnBrokenTree runs the command on a tree whose
// project.yaml is missing and checks the JSON report and the non-nil result.
func TestVerifyGitCommand_JSONOnBrokenTree(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "pletka.mod"), []byte("schema_version: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	c := newProjectVerifyGitCommand()
	c.SetOut(&out)
	c.SetArgs([]string{dir, "--format", "json"})
	err := c.Execute()
	if err == nil {
		t.Fatal("expected a non-nil error for a tree with findings")
	}
	var rep struct {
		Findings []struct{ Code string } `json:"findings"`
	}
	if jerr := json.Unmarshal(out.Bytes(), &rep); jerr != nil {
		t.Fatalf("report: %s (%v)", out.String(), jerr)
	}
	hasLoad := false
	for _, f := range rep.Findings {
		hasLoad = hasLoad || f.Code == "load"
	}
	if !hasLoad {
		t.Fatalf("want a load finding: %s", out.String())
	}
}
