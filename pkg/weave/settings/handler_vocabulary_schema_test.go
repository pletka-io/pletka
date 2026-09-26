package settings

import (
	"context"
	"errors"
	"testing"

	"github.com/pletka-io/pletka/pkg/formschema"
)

// TestHandlerVocabularySchemaSurfacesServiceListerError is the seam's whole
// point made concrete: when the configured vocabulary service fails to
// answer, the handler's schema-building path must hand the caller an error
// rather than quietly render a form with an empty "add from service" list.
// An empty list here would read to a curator as "the service serves
// nothing", which is a different (false) claim from "we couldn't ask it".
func TestHandlerVocabularySchemaSurfacesServiceListerError(t *testing.T) {
	h := &Handler{serviceVocabularies: stubLister{err: errors.New("dial tcp: connection refused")}}

	schema, err := h.vocabularySchema(context.Background(), "proj-1", VocabularySettingsState{}, "en")
	if err == nil {
		t.Fatal("a service-lister error must reach the caller, not collapse into an empty option list")
	}
	if schema != nil {
		t.Fatalf("schema = %#v, want nil on error", schema)
	}
}

// TestHandlerVocabularySchemaOffersServiceOptionsWithNoError is the
// companion positive case: a successful listing must actually reach the
// rendered form as the "add from service" field's options.
func TestHandlerVocabularySchemaOffersServiceOptionsWithNoError(t *testing.T) {
	h := &Handler{serviceVocabularies: stubLister{out: []ServiceVocabulary{
		{Name: "fish-monument-type", Label: "FISH Monument Types", Concepts: 5009, Languages: []string{"en"}},
	}}}

	schema, err := h.vocabularySchema(context.Background(), "proj-1", VocabularySettingsState{}, "en")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	field := findVocabularyField(t, schema, "add_vocabulary_id")
	if len(field.Options) != 1 || field.Options[0].Value != "fish-monument-type" {
		t.Fatalf("add_vocabulary_id options = %#v, want the one service mount", field.Options)
	}
}

func findVocabularyField(t *testing.T, schema *formschema.FormSchema, name string) formschema.FieldDef {
	t.Helper()
	for _, section := range schema.Sections {
		for _, field := range section.Fields {
			if field.Name == name {
				return field
			}
		}
	}
	t.Fatalf("field %q not found in schema", name)
	return formschema.FieldDef{}
}
