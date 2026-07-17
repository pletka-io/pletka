package formschema_test

import (
	"strings"
	"testing"

	"github.com/pletka-io/pletka/pkg/formschema"
)

func TestBuildProjectOntologyVersionListSchema(t *testing.T) {
	s := formschema.BuildProjectOntologyVersionListSchema("LA", "en", nil)
	if s == nil {
		t.Fatal("nil schema")
	}
	if s.EntityType != "project-ontology-version" {
		t.Errorf("EntityType=%q", s.EntityType)
	}
	if !strings.Contains(s.DataURL, "/projects/LA/project-ontology-versions") {
		t.Errorf("DataURL=%s", s.DataURL)
	}
	if s.Caps.Create == nil || s.Caps.Edit == nil || s.Caps.Delete == nil || s.Caps.Stats == nil {
		t.Errorf("missing expected capabilities: %+v", s.Caps)
	}
	if s.Caps.Reorder != nil {
		t.Errorf("Reorder should be nil — versions have no canonical order")
	}
	if s.Caps.InlineRename != nil {
		t.Errorf("InlineRename should be nil — ontology label comes from master table")
	}
	wantColumns := []string{"label", "version", "primary", "usage_count"}
	var gotColumns []string
	for _, c := range s.Columns {
		gotColumns = append(gotColumns, c.Key)
	}
	if len(gotColumns) != len(wantColumns) {
		t.Fatalf("column count: got %d want %d (%v)", len(gotColumns), len(wantColumns), gotColumns)
	}
	for i, want := range wantColumns {
		if gotColumns[i] != want {
			t.Errorf("Columns[%d].Key=%q want %q", i, gotColumns[i], want)
		}
	}
}
