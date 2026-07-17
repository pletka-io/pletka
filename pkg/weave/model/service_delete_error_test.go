package model

import (
	"strings"
	"testing"
)

func TestErrEntityInUse_MentionsProjectsAndDeprecate(t *testing.T) {
	err := &ErrEntityInUse{
		ModelID: "LAM.1",
		Usage:   UsageReport{FieldCount: 3, ProjectIDs: []string{"LA", "SRD"}},
	}
	msg := err.Error()
	for _, want := range []string{"LAM.1", "3", "2 project", "deprecate"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q missing %q", msg, want)
		}
	}
}
