package cmd

import (
	"strings"
	"testing"
)

func TestCheckAuditVersioningFlags(t *testing.T) {
	if err := checkAuditVersioningFlags("text", nil); err != nil {
		t.Errorf("text, no rules: %v", err)
	}
	if err := checkAuditVersioningFlags("json", []string{"L5", "A2"}); err != nil {
		t.Errorf("json, known rules: %v", err)
	}

	err := checkAuditVersioningFlags("csv", nil)
	if err == nil || !strings.Contains(err.Error(), "text or json") {
		t.Errorf("csv: got %v, want an error naming the formats", err)
	}

	err = checkAuditVersioningFlags("text", []string{"L5", "Z9"})
	if err == nil || !strings.Contains(err.Error(), "Z9") || !strings.Contains(err.Error(), "L5") {
		t.Errorf("unknown rule: got %v, want an error naming Z9 and the known rules", err)
	}
}

func TestWeaveCommandHasAuditVersioning(t *testing.T) {
	for _, c := range newWeaveCommand(nil).Commands() {
		if c.Name() == "audit-versioning" {
			return
		}
	}
	t.Error("weave has no audit-versioning subcommand")
}
