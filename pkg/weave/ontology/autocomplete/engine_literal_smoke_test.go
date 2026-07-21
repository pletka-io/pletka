//go:build integration

package autocomplete_test

import (
	"context"
	"testing"

	"github.com/pletka-io/pletka/pkg/weave"
	weaveontology "github.com/pletka-io/pletka/pkg/weave/ontology"
	"github.com/pletka-io/pletka/pkg/weave/ontology/autocomplete"
)

// TestEngine_LiteralSuggestions_Smoke verifies that GetSuggestions emits
// a synthetic "literal" suggestion when the path tip is a
// DatatypeProperty. Prior to this, the engine
// returned an empty slice and the Path Writer dropdown showed nothing
// when the user navigated into a DatatypeProperty's range slot.
//
// Live-DB smoke test, mirroring resolver_smoke_test.go conventions.
// Skipped when no DB is available.
func TestEngine_LiteralSuggestions_Smoke(t *testing.T) {
	pool := testPool(t)
	defer pool.Close()

	store := weaveontology.NewPostgresStore(pool)
	ws := weave.NewPostgresStore(pool)
	svc := weaveontology.NewService(store, ws.Projects(), nil)

	// crm:P3_has_note is a DatatypeProperty in CRM. Its range relation
	// targets rdfs:Literal in production data. The engine should emit a
	// synthetic literal suggestion derived from that range.
	cases := []struct {
		name        string
		currentPath []string
		projectID   string
		wantLiteral bool
		wantPrefix  string // expected prefix on the literal suggestion's datatype
	}{
		{
			name:        "datatype property emits literal",
			currentPath: []string{"crm:E1_CRM_Entity", "crm:P3_has_note"},
			projectID:   "LA",
			wantLiteral: true,
			// Either xsd:* or rdfs:* depending on which W3C namespace
			// the source ontology used for this property's range.
			wantPrefix: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := autocomplete.Request{
				ProjectID:   tc.projectID,
				CurrentPath: tc.currentPath,
				MaxResults:  20,
			}
			suggestions, err := svc.GetSuggestions(context.Background(), req)
			if err != nil {
				t.Fatalf("GetSuggestions: %v", err)
			}

			var literals []autocomplete.Suggestion
			for _, s := range suggestions {
				if s.Type == "literal" {
					literals = append(literals, s)
				}
			}

			if tc.wantLiteral && len(literals) == 0 {
				t.Errorf("expected at least one literal suggestion, got %d total suggestions, none of type=literal", len(suggestions))
				for _, s := range suggestions {
					t.Logf("got suggestion type=%q qname=%q", s.Type, s.Qname)
				}
				return
			}

			for _, lit := range literals {
				if lit.Datatype == "" {
					t.Errorf("literal suggestion %q has empty Datatype field", lit.Qname)
				}
				if lit.Qname != lit.Datatype {
					t.Errorf("literal suggestion qname=%q != datatype=%q (expected synthetic literal to round-trip)", lit.Qname, lit.Datatype)
				}
				t.Logf("literal suggestion: qname=%q datatype=%q label=%v", lit.Qname, lit.Datatype, lit.Label)
			}
		})
	}
}
