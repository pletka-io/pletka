package formschema

import "testing"

// TestExampleEntityListEditorEndpoints locks the endpoint set the
// example-workspace editor relies on (schema-driven URL rule: the widget
// never builds these itself).
func TestExampleEntityListEditorEndpoints(t *testing.T) {
	s := BuildExampleEntityListSchema("LA", "en", nil, "model", "LAM.10")
	if s.Editor == nil {
		t.Fatal("editor missing")
	}
	want := map[string]string{
		"list_url":            "/projects/LA/examples",
		"detail_url_template": "/projects/LA/examples/{id}",
		"form_schema_url":     "/projects/LA/examples/form-schema",
		"page_url_template":   "/projects/LA/examples/{id}",
	}
	for k, v := range want {
		if got := s.Editor.Endpoints[k]; got != v {
			t.Errorf("endpoints[%q] = %q, want %q", k, got, v)
		}
	}
	if len(s.Editor.Endpoints) != len(want) {
		t.Errorf("endpoint count = %d, want %d: %v", len(s.Editor.Endpoints), len(want), s.Editor.Endpoints)
	}
}
