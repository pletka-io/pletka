package formschema

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
)

func TestFilterForRole(t *testing.T) {
	schema := &FormSchema{
		EntityType: "category",
		Mode:       ModeCreate,
		Endpoint:   &SchemaEndpoint{Method: "POST", URL: "/test"},
		Sections: []Section{
			{
				ID:    "identity",
				Label: domain.Translations{"en": "Identity"},
				Fields: []FieldDef{
					{Name: "ui_name", Widget: WidgetMultilingualText, MinRole: ""},
					{Name: "status", Widget: WidgetSelect, MinRole: "maintainer"},
				},
			},
			{
				ID:    "admin",
				Label: domain.Translations{"en": "Admin"},
				Fields: []FieldDef{
					{Name: "danger", Widget: WidgetText, MinRole: "owner"},
				},
			},
		},
	}

	t.Run("contributor sees identity but not admin section", func(t *testing.T) {
		filtered := schema.FilterForRole("contributor")
		if len(filtered.Sections) != 1 {
			t.Fatalf("expected 1 section, got %d", len(filtered.Sections))
		}
		if len(filtered.Sections[0].Fields) != 1 {
			t.Fatalf("expected 1 field, got %d", len(filtered.Sections[0].Fields))
		}
		if filtered.Sections[0].Fields[0].Name != "ui_name" {
			t.Errorf("expected ui_name field, got %s", filtered.Sections[0].Fields[0].Name)
		}
		if filtered.Endpoint == nil {
			t.Error("contributor should have an endpoint")
		}
	})

	t.Run("viewer gets no endpoint", func(t *testing.T) {
		filtered := schema.FilterForRole("viewer")
		if filtered.Endpoint != nil {
			t.Error("viewer should not have an endpoint")
		}
	})

	t.Run("owner sees everything", func(t *testing.T) {
		filtered := schema.FilterForRole("owner")
		if len(filtered.Sections) != 2 {
			t.Fatalf("expected 2 sections, got %d", len(filtered.Sections))
		}
	})
}

func TestFieldDef_MarshalsDependentFieldKeys(t *testing.T) {
	f := FieldDef{
		Name:              "version_id",
		Widget:            "select",
		DependsOn:         []string{"ontology_id"},
		OptionsURL:        "/projects/P/ontology-versions?base={ontology_id}",
		SearchURL:         "/projects/P/ontology-versions/search?base={ontology_id}",
		HiddenUntilFilled: []string{"ontology_id"},
	}
	b, err := json.Marshal(f)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(b)
	for _, want := range []string{
		`"depends_on":["ontology_id"]`,
		`"options_url":"/projects/P/ontology-versions?base={ontology_id}"`,
		`"search_url":"/projects/P/ontology-versions/search?base={ontology_id}"`,
		`"hidden_until_filled":["ontology_id"]`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
}

func TestCompositePaneSchema_MarshalShape(t *testing.T) {
	s := CompositePaneSchema{
		Kind:  "composite-pane",
		Title: domain.Translations{"en": "Ontology"},
		Panels: []CompositePanel{
			{ID: "parent-inheritance", SchemaURL: "/x"},
			{ID: "linked-ontologies", Label: domain.Translations{"en": "Linked"}, SchemaURL: "/y"},
		},
	}
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(b)
	want := `{"kind":"composite-pane","title":{"en":"Ontology"},"panels":[{"id":"parent-inheritance","schema_url":"/x"},{"id":"linked-ontologies","label":{"en":"Linked"},"schema_url":"/y"}]}`
	if got != want {
		t.Errorf("marshal mismatch:\n got: %s\nwant: %s", got, want)
	}
}
