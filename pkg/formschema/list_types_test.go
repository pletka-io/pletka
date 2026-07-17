package formschema_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/formschema"
)

func TestListSchema_GroupsAndReadonly(t *testing.T) {
	s := formschema.ListSchema{
		EntityType:          "project-ontology-version",
		PerRowReadonlyField: "_readonly",
	}
	b, _ := json.Marshal(s)
	got := string(b)
	if !strings.Contains(got, `"per_row_readonly_field":"_readonly"`) {
		t.Errorf("missing per_row_readonly_field: %s", got)
	}
}

func TestGroup_MarshalShape(t *testing.T) {
	g := formschema.Group{
		ID:          "inherited-LA",
		Label:       domain.Translations{"en": "Inherited from LA"},
		Collapsible: true,
		Collapsed:   false,
		ReadOnly:    true,
		Badges:      []formschema.Badge{{Label: "Read only", Tone: "neutral"}},
		Items: []map[string]any{
			{"id": "v1", "label": "CIDOC-CRM 7.1.3"},
		},
	}
	b, err := json.Marshal(g)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(b)
	for _, want := range []string{
		`"id":"inherited-LA"`,
		`"read_only":true`,
		`"badges":[{"label":"Read only","tone":"neutral"}]`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
}
