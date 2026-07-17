package autocomplete_test

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/pletka-io/pletka/pkg/weave"
	weaveontology "github.com/pletka-io/pletka/pkg/weave/ontology"
	"github.com/pletka-io/pletka/pkg/weave/ontology/autocomplete"
)

// equivalenceProbes is the canonical smoke-probe set shared by all
// modes-equivalence tests. Keep in sync with the behavioural smoke tests.
func equivalenceProbes() []autocomplete.Request {
	return []autocomplete.Request{
		// Property suggestions after a class (LA project)
		{ProjectID: "LA", CurrentPath: []string{"crm:E1_CRM_Entity", "crm:P1_is_identified_by"}, MaxResults: 50},
		// Literal (DatatypeProperty) suggestions (LA project)
		{ProjectID: "LA", CurrentPath: []string{"crm:E1_CRM_Entity", "crm:P3_has_note"}, MaxResults: 20},
		// Cross-version range subclasses (AME project)
		{ProjectID: "AME", CurrentPath: []string{"crm:E1_CRM_Entity", "crm:P140i_was_attributed_by"}, MaxResults: 200},
		// Properties via cross-version class lineage (AME project, aaao class)
		{ProjectID: "AME", CurrentPath: []string{"aaao:ZE19_Naming"}, MaxResults: 500},
		// Root-class search matching on local_name / label.
		{ProjectID: "AME", CurrentPath: []string{}, Query: "Naming", MaxResults: 50},
		// Root-class search matching on qname prefix only (not local_name or label).
		// Verifies that IndexedEngine.suggestRootClasses also checks qname, mirroring
		// DirectEngine's SearchClasses SQL (local_name OR qname OR label).
		// psql: SELECT count(DISTINCT qname) FROM weave_ontology_classes WHERE ... AND qname ILIKE '%aaao:%' → 52 rows.
		{ProjectID: "AME", CurrentPath: []string{}, Query: "aaao:", MaxResults: 500},
	}
}

// suggestionKey is the semantic identity tuple for a suggestion.
// We compare on Qname+Type+Datatype and deliberately exclude OntologyPrefix
// and OntologyVersion (Meta fields): for a qname defined in multiple linked
// versions, DirectEngine emits the first-seen version's Meta while
// IndexedEngine emits the PRIMARY version's Meta. This is a deliberate v3
// design decision ("primary wins for Meta") and produces legitimate
// differences that must not be treated as failures.
type suggestionKey struct {
	Qname    string
	Type     string
	Datatype string
}

func toKeys(ss []autocomplete.Suggestion) []suggestionKey {
	out := make([]suggestionKey, len(ss))
	for i, s := range ss {
		out[i] = suggestionKey{Qname: s.Qname, Type: s.Type, Datatype: s.Datatype}
	}
	return out
}

// lastStep returns a short label for the last element of req.CurrentPath,
// or "_root_" when the path is empty.
func lastStep(req autocomplete.Request) string {
	if len(req.CurrentPath) == 0 {
		if req.Query != "" {
			return "query=" + req.Query
		}
		return "_root_"
	}
	return req.CurrentPath[len(req.CurrentPath)-1]
}

// TestModesEquivalence_DirectVsIndexed_Smoke is the primary correctness gate
// for the autocomplete engines. It runs every smoke probe through both
// DirectEngine and IndexedEngine and asserts that the semantic suggestion
// sets are identical (sorted by Type + LocalName, compared on Qname+Type+Datatype).
//
// Requires a live DB at localhost:5433 (or TEST_DATABASE_URL). Projects AME
// and LA must have ontology coverage.
func TestModesEquivalence_DirectVsIndexed_Smoke(t *testing.T) {
	pool := testPool(t)
	defer pool.Close()

	store := weaveontology.NewPostgresStore(pool)
	ws := weave.NewPostgresStore(pool)

	direct := autocomplete.NewDirect(store, ws.Projects())
	cache := autocomplete.NewIndexCache(store, ws.Projects(), nil)
	indexed := autocomplete.NewIndexed(cache)

	for _, req := range equivalenceProbes() {
		req := req
		name := req.ProjectID + "|" + lastStep(req)
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()

			d, err := direct.GetSuggestions(ctx, req)
			if err != nil {
				t.Fatalf("direct.GetSuggestions: %v", err)
			}

			i, err := indexed.GetSuggestions(ctx, req)
			if err != nil {
				t.Fatalf("indexed.GetSuggestions: %v", err)
			}

			autocomplete.SortSuggestions(d)
			autocomplete.SortSuggestions(i)

			dKeys := toKeys(d)
			iKeys := toKeys(i)

			if diff := cmp.Diff(dKeys, iKeys); diff != "" {
				t.Errorf("suggestion-set mismatch (-direct +indexed):\n%s", diff)
				t.Logf("direct:  %d suggestions", len(d))
				t.Logf("indexed: %d suggestions", len(i))
			}
		})
	}
}
