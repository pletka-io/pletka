//go:build integration

package autocomplete_test

import (
	"context"
	"testing"

	"github.com/pletka-io/pletka/pkg/weave"
	weaveontology "github.com/pletka-io/pletka/pkg/weave/ontology"
	"github.com/pletka-io/pletka/pkg/weave/ontology/autocomplete"
)

// TestIndexedEngine_CrossVersionSubclasses_Smoke verifies that IndexedEngine
// surfaces cross-version subclasses when a range class is defined in one
// linked ontology version and a subclass is declared in another.
//
// Fixture: AME project, path ending in crm:P140i_was_attributed_by.
// crm:E13_Attribute_Assignment is the direct range. AAAo declares
// aaao:ZE19_Naming as a subclass of E13 via a cross-version edge. Both must
// appear in the indexed engine's output.
func TestIndexedEngine_CrossVersionSubclasses_Smoke(t *testing.T) {
	pool := testPool(t)
	defer pool.Close()
	store := weaveontology.NewPostgresStore(pool)
	ws := weave.NewPostgresStore(pool)
	cache := autocomplete.NewIndexCache(store, ws.Projects(), nil)
	eng := autocomplete.NewIndexed(cache)

	req := autocomplete.Request{
		ProjectID:   "AME",
		CurrentPath: []string{"crm:E1_CRM_Entity", "crm:P140i_was_attributed_by"},
		MaxResults:  200,
	}
	got, err := eng.GetSuggestions(context.Background(), req)
	if err != nil {
		t.Fatalf("GetSuggestions: %v", err)
	}
	want := map[string]bool{
		"crm:E13_Attribute_Assignment": false,
		"aaao:ZE19_Naming":             false,
	}
	for _, s := range got {
		if _, ok := want[s.Qname]; ok {
			want[s.Qname] = true
		}
	}
	for q, f := range want {
		if !f {
			t.Errorf("missing %q from indexed engine", q)
		}
	}
	if t.Failed() {
		t.Logf("project=%s path=%v got %d suggestions", req.ProjectID, req.CurrentPath, len(got))
		for _, s := range got {
			t.Logf("  type=%q qname=%q", s.Type, s.Qname)
		}
	}
}

// TestIndexedEngine_PropertiesForScope_CrossVersionLineage_Smoke mirrors the
// DirectEngine test for property suggestions when the scope class spans
// ontology versions. The indexed engine must walk the full ancestor lineage
// across version boundaries via the pre-wired pointer graph.
//
// Fixture: AME project, scope aaao:ZE19_Naming.
// Expected: crm:P140i_was_attributed_by (domain E13, one hop up) and
// crm:P129i_is_subject_of (domain E1, full chain up).
func TestIndexedEngine_PropertiesForScope_CrossVersionLineage_Smoke(t *testing.T) {
	pool := testPool(t)
	defer pool.Close()
	store := weaveontology.NewPostgresStore(pool)
	ws := weave.NewPostgresStore(pool)
	cache := autocomplete.NewIndexCache(store, ws.Projects(), nil)
	eng := autocomplete.NewIndexed(cache)

	req := autocomplete.Request{
		ProjectID:   "AME",
		CurrentPath: []string{"aaao:ZE19_Naming"},
		MaxResults:  500,
	}
	got, err := eng.GetSuggestions(context.Background(), req)
	if err != nil {
		t.Fatalf("GetSuggestions: %v", err)
	}
	want := map[string]bool{
		"crm:P140i_was_attributed_by": false, // one hop up (E13)
		"crm:P129i_is_subject_of":     false, // full chain up (E1)
	}
	for _, s := range got {
		if _, ok := want[s.Qname]; ok {
			want[s.Qname] = true
		}
	}
	for q, f := range want {
		if !f {
			t.Errorf("missing cross-version ancestor property %q from indexed engine", q)
		}
	}
	if t.Failed() {
		t.Logf("project=%s path=%v got %d suggestions", req.ProjectID, req.CurrentPath, len(got))
		for _, s := range got {
			t.Logf("  type=%q qname=%q", s.Type, s.Qname)
		}
	}
}

// TestIndexedEngine_LiteralSuggestion_Smoke verifies that IndexedEngine emits
// a synthetic "literal" suggestion when the path tip is a DatatypeProperty,
// mirroring DirectEngine's behaviour.
//
// Fixture: LA project, path ending in crm:P3_has_note (a DatatypeProperty).
func TestIndexedEngine_LiteralSuggestion_Smoke(t *testing.T) {
	pool := testPool(t)
	defer pool.Close()
	store := weaveontology.NewPostgresStore(pool)
	ws := weave.NewPostgresStore(pool)
	cache := autocomplete.NewIndexCache(store, ws.Projects(), nil)
	eng := autocomplete.NewIndexed(cache)

	req := autocomplete.Request{
		ProjectID:   "LA",
		CurrentPath: []string{"crm:E1_CRM_Entity", "crm:P3_has_note"},
		MaxResults:  20,
	}
	got, err := eng.GetSuggestions(context.Background(), req)
	if err != nil {
		t.Fatalf("GetSuggestions: %v", err)
	}
	var literals []autocomplete.Suggestion
	for _, s := range got {
		if s.Type == "literal" {
			literals = append(literals, s)
		}
	}
	if len(literals) == 0 {
		t.Errorf("expected at least one literal suggestion, got %d total suggestions, none of type=literal", len(got))
		for _, s := range got {
			t.Logf("  type=%q qname=%q", s.Type, s.Qname)
		}
		return
	}
	for _, lit := range literals {
		if lit.Datatype == "" {
			t.Errorf("literal suggestion %q has empty Datatype field", lit.Qname)
		}
		if lit.Qname != lit.Datatype {
			t.Errorf("literal qname=%q != datatype=%q", lit.Qname, lit.Datatype)
		}
		t.Logf("literal: qname=%q datatype=%q label=%v", lit.Qname, lit.Datatype, lit.Label)
	}
}
