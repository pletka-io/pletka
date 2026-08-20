package gitmaterializer

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestDeletePathsFor(t *testing.T) {
	got := deletePathsFor("model", "GRPM.2")
	want := []string{"models/GRPM.2"} // whole dir: model.yaml + overrides/
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("model delete paths:\n%s", diff)
	}
	if diff := cmp.Diff([]string{"fields/GRPF.1"}, deletePathsFor("field", "GRPF.1")); diff != "" {
		t.Fatalf("field delete paths:\n%s", diff)
	}
}
