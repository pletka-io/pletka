//go:build integration

package autocomplete_test

import (
	"context"
	"testing"

	"github.com/pletka-io/pletka/pkg/weave"
	weaveontology "github.com/pletka-io/pletka/pkg/weave/ontology"
	"github.com/pletka-io/pletka/pkg/weave/ontology/autocomplete"
)

// TestEngine_RangeSubclasses_Smoke verifies that GetSuggestions emits
// not only the direct range class of a property but also its subclasses,
// so curators can pick a more-specific node when the ontology defines
// one.
//
// Concrete fixture: crm:P1_is_identified_by has range crm:E41_Appellation.
// The CRM also defines crm:E33_E41_Linguistic_Appellation as a subclass
// of E41 (a multi-typed convenience class). George reported the dropdown
// not offering E33_E41 when typing p1->; this test verifies it now does.
//
// Skipped when no DB is available.
func TestEngine_RangeSubclasses_Smoke(t *testing.T) {
	pool := testPool(t)
	defer pool.Close()

	store := weaveontology.NewPostgresStore(pool)
	ws := weave.NewPostgresStore(pool)
	svc := weaveontology.NewService(store, ws.Projects(), nil)

	req := autocomplete.Request{
		ProjectID:   "LA",
		CurrentPath: []string{"crm:E1_CRM_Entity", "crm:P1_is_identified_by"},
		MaxResults:  50,
	}
	suggestions, err := svc.GetSuggestions(context.Background(), req)
	if err != nil {
		t.Fatalf("GetSuggestions: %v", err)
	}

	wantQnames := map[string]bool{
		"crm:E41_Appellation":                false,
		"crm:E33_E41_Linguistic_Appellation": false,
	}
	for _, s := range suggestions {
		if _, ok := wantQnames[s.Qname]; ok {
			wantQnames[s.Qname] = true
		}
	}
	for qname, found := range wantQnames {
		if !found {
			t.Errorf("expected suggestion qname=%q to be present", qname)
		}
	}

	if t.Failed() {
		for _, s := range suggestions {
			t.Logf("got: type=%q qname=%q", s.Type, s.Qname)
		}
	}
}

// TestEngine_RangeSubclasses_CrossVersion_Smoke pins the project-union
// semantic for subclass walking: when the range class of a property is
// defined in one linked ontology version (CRM) and a sibling linked
// version (AAAo) declares a subclass of that range class, the dropdown
// must surface the cross-ontology subclass — not just the same-version
// ones.
//
// Fixture: AME project links CRM + AAAo (+ others). crm:P140i has range
// crm:E13_Attribute_Assignment. AAAo defines aaao:ZE19_Naming as a
// subclass of crm:E13_Attribute_Assignment via the cross-ontology
// subclass_of edge in weave_ontology_relations. The post-fix engine
// must walk subclass relations across every linked version, not only
// the version where the range qname was first found.
//
// Pre-fix this test FAILS — aaao:ZE19_Naming is absent because
// collectSubclasses only queried CRM's version. Post-fix it PASSES.
//
// Skipped when no DB is available.
func TestEngine_RangeSubclasses_CrossVersion_Smoke(t *testing.T) {
	pool := testPool(t)
	defer pool.Close()

	store := weaveontology.NewPostgresStore(pool)
	ws := weave.NewPostgresStore(pool)
	svc := weaveontology.NewService(store, ws.Projects(), nil)

	req := autocomplete.Request{
		ProjectID:   "AME",
		CurrentPath: []string{"crm:E1_CRM_Entity", "crm:P140i_was_attributed_by"},
		MaxResults:  200,
	}
	suggestions, err := svc.GetSuggestions(context.Background(), req)
	if err != nil {
		t.Fatalf("GetSuggestions: %v", err)
	}

	wantQnames := map[string]bool{
		"crm:E13_Attribute_Assignment": false,
		"aaao:ZE19_Naming":             false,
	}
	for _, s := range suggestions {
		if _, ok := wantQnames[s.Qname]; ok {
			wantQnames[s.Qname] = true
		}
	}
	for qname, found := range wantQnames {
		if !found {
			t.Errorf("expected cross-version suggestion qname=%q to be present", qname)
		}
	}

	if t.Failed() {
		t.Logf("project=%s path=%v got %d suggestions", req.ProjectID, req.CurrentPath, len(suggestions))
		for _, s := range suggestions {
			t.Logf("got: type=%q qname=%q", s.Type, s.Qname)
		}
	}
}

// TestEngine_PropertiesForScope_CrossVersionMultiRowLineage_Smoke pins the
// case where a qname (e.g. crm:E4_Period) has DISTINCT rows in different
// linked ontology versions: the CRM 7.1.3 row declares parents E2 and E92,
// while the crmgeo 1.2.1 row declares parent crmgeo:SP1_Phenomenal_Spacetime_Volume.
// classLineageQnames must walk subclass_of relations from EVERY linked version-row
// of each qname, not just the first row, so the crmgeo branch is not missed.
//
// Fixture: AME project, scope aaao:ZE19_Naming. Its lineage via CRM includes
// crm:E4_Period (which has a crmgeo cross-version row declaring SP1 as parent).
// Properties whose domain is crmgeo:SP1 (e.g. crmgeo:Q1i_is_occupied_by,
// crmgeo:Q3_has_temporal_projection) must appear in the suggestions.
//
// Pre-fix this FAILS — DirectEngine calls firstClassByQname for crm:E4_Period
// and only walks that one row's relations (CRM row, no crmgeo SP1 edge).
// Post-fix it PASSES.
//
// Skipped when no DB is available.
func TestEngine_PropertiesForScope_CrossVersionMultiRowLineage_Smoke(t *testing.T) {
	pool := testPool(t)
	defer pool.Close()

	store := weaveontology.NewPostgresStore(pool)
	ws := weave.NewPostgresStore(pool)
	svc := weaveontology.NewService(store, ws.Projects(), nil)

	req := autocomplete.Request{
		ProjectID:   "AME",
		CurrentPath: []string{"aaao:ZE19_Naming"},
		MaxResults:  500,
	}
	suggestions, err := svc.GetSuggestions(context.Background(), req)
	if err != nil {
		t.Fatalf("GetSuggestions: %v", err)
	}

	// crmgeo properties that are only reachable via the SP1 branch of
	// crm:E4_Period's lineage (crmgeo version row, not CRM version row).
	wantQnames := map[string]bool{
		"crmgeo:Q1i_is_occupied_by":      false,
		"crmgeo:Q3_has_temporal_projection": false,
	}
	for _, s := range suggestions {
		if _, ok := wantQnames[s.Qname]; ok {
			wantQnames[s.Qname] = true
		}
	}
	for qname, found := range wantQnames {
		if !found {
			t.Errorf("expected crmgeo cross-version property qname=%q to be present (missed SP1 lineage branch)", qname)
		}
	}

	if t.Failed() {
		t.Logf("project=%s path=%v got %d suggestions", req.ProjectID, req.CurrentPath, len(suggestions))
		for _, s := range suggestions {
			t.Logf("got: type=%q qname=%q", s.Type, s.Qname)
		}
	}
}

// TestEngine_PropertiesForScope_CrossVersionLineage_Smoke pins the
// symmetric upward case: when the scope class is defined in
// one linked ontology version (AAAo) and inherits from a class defined
// in a different linked version (CRM), the property dropdown must
// surface properties whose domain is **any ancestor across the full
// cross-version lineage** — not just direct-domain properties in the
// scope class's home version.
//
// Fixture: AME project. aaao:ZE19_Naming is declared as a subclass of
// crm:E13_Attribute_Assignment. The full chain is
// ZE19 ⊂ E13 ⊂ E11 ⊂ E7 ⊂ E5 ⊂ E2 ⊂ E1_CRM_Entity, spanning the AAAo
// and CRM versions. With a scope of ZE19 the dropdown must offer:
//   - crm:P140i_was_attributed_by  (domain crm:E13 — one hop up)
//   - crm:P129i_is_subject_of      (domain crm:E1 — full chain up)
//
// Pre-fix this FAILS because classLineageQnames walks superclasses in
// a single version only, so the chain breaks at the first cross-
// ontology subclass_of edge.
//
// Skipped when no DB is available.
func TestEngine_PropertiesForScope_CrossVersionLineage_Smoke(t *testing.T) {
	pool := testPool(t)
	defer pool.Close()

	store := weaveontology.NewPostgresStore(pool)
	ws := weave.NewPostgresStore(pool)
	svc := weaveontology.NewService(store, ws.Projects(), nil)

	req := autocomplete.Request{
		ProjectID:   "AME",
		CurrentPath: []string{"aaao:ZE19_Naming"},
		MaxResults:  500,
	}
	suggestions, err := svc.GetSuggestions(context.Background(), req)
	if err != nil {
		t.Fatalf("GetSuggestions: %v", err)
	}

	wantQnames := map[string]bool{
		"crm:P140i_was_attributed_by": false, // one hop up (E13)
		"crm:P129i_is_subject_of":     false, // full chain up (E1)
	}
	for _, s := range suggestions {
		if _, ok := wantQnames[s.Qname]; ok {
			wantQnames[s.Qname] = true
		}
	}
	for qname, found := range wantQnames {
		if !found {
			t.Errorf("expected cross-version ancestor property qname=%q to be present", qname)
		}
	}

	if t.Failed() {
		t.Logf("project=%s path=%v got %d suggestions", req.ProjectID, req.CurrentPath, len(suggestions))
		for _, s := range suggestions {
			t.Logf("got: type=%q qname=%q", s.Type, s.Qname)
		}
	}
}
