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
		"models_url":          "/projects/LA/models",
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

func TestReleasedExampleSchemaPinsReadEndpointsAndRemovesWrites(t *testing.T) {
	s := BuildExampleEntityListSchema("LA", "en", nil, "model", "LAM.10")
	s.BindReadVersion("2.0.0")
	s.StripEditActions()
	if s.DataURL != "/projects/LA/examples?entity_id=LAM.10&entity_type=model&version=2.0.0" {
		t.Fatalf("data URL: %s", s.DataURL)
	}
	if s.Editor.Endpoints["form_schema_url"] != "/projects/LA/examples/form-schema?version=2.0.0" {
		t.Fatalf("viewer URL: %s", s.Editor.Endpoints["form_schema_url"])
	}
	if s.Capabilities != nil {
		t.Fatalf("release exposes write capabilities: %+v", s.Capabilities)
	}
	if len(s.RowActions) != 1 || s.RowActions[0].ID != "view" || s.RowActions[0].Method != "GET" {
		t.Fatalf("release actions: %+v", s.RowActions)
	}
}
