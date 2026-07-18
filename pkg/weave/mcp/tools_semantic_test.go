package mcp

import (
	"context"
	"testing"

	"github.com/pletka-io/pletka/pkg/weave/ontology/autocomplete"
)

type fakeAutocomplete struct{ got autocomplete.Request }

func (f *fakeAutocomplete) GetSuggestions(_ context.Context, req autocomplete.Request) ([]autocomplete.Suggestion, error) {
	f.got = req
	return []autocomplete.Suggestion{{Qname: "crm:P1_is_identified_by", Type: "property"}}, nil
}

func TestAutocompleteToolPlumbing(t *testing.T) {
	h := testHostProjects()
	fake := &fakeAutocomplete{}
	h.Ontology = fake
	out, err := ontologyAutocomplete(context.Background(), h, autocompleteInput{
		ProjectID:   "LA",
		CurrentPath: []string{"crm:E21_Person"},
		Query:       "identified",
	})
	if err != nil {
		t.Fatalf("autocomplete: %v", err)
	}
	if len(out.Suggestions) != 1 {
		t.Fatalf("want 1 suggestion, got %d", len(out.Suggestions))
	}
	if fake.got.ProjectID != "LA" || len(fake.got.CurrentPath) != 1 {
		t.Fatalf("request not forwarded: %+v", fake.got)
	}
	if fake.got.MaxResults == 0 {
		t.Fatal("MaxResults default not applied")
	}
}

func TestFormSchemaUnknownType(t *testing.T) {
	h := testHostProjects()
	if _, err := getFormSchema(context.Background(), h, formSchemaInput{ProjectID: "LA", EntityType: "widget"}); err == nil {
		t.Fatal("unknown entity_type must error")
	}
}
