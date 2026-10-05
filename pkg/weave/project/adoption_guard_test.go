package project

import (
	"strings"
	"testing"
)

// The adoption picker reads an ancestor at that ancestor's own release. If
// its readers are not wired it would fall back to listing live entities,
// which is exactly how projects came to adopt content that was never
// published. Validate must refuse, so a forgotten wiring fails at startup
// rather than shipping a picker that quietly offers drafts.
func TestHostValidateRequiresTheAdoptionReaders(t *testing.T) {
	for _, missing := range []string{"Models", "Collections", "Releases"} {
		h := Host{
			Service:     &Service{},
			Weave:       nil,
			Models:      stubModelLister{},
			Collections: stubCollectionLister{},
			Releases:    stubReleaseLister{},
		}
		switch missing {
		case "Models":
			h.Models = nil
		case "Collections":
			h.Collections = nil
		case "Releases":
			h.Releases = nil
		}
		err := h.Validate()
		if err == nil {
			t.Errorf("Validate accepted a Host with no %s; a nil reader silently returns the picker to listing drafts", missing)
			continue
		}
		if !strings.Contains(err.Error(), missing) {
			t.Errorf("Validate rejected a Host with no %s but did not name it: %v", missing, err)
		}
	}
}

func TestContainsString(t *testing.T) {
	live := []string{"0.2.0", "0.1.0"}
	if !containsString(live, "0.1.0") {
		t.Error("an older live release must be selectable: a curator may deliberately want a stable one")
	}
	if containsString(live, "0.3.0") {
		t.Error("a version the source has not released must not be accepted")
	}
	if containsString(nil, "0.1.0") {
		t.Error("a project with no releases offers nothing")
	}
}
