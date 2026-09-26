package settings

import (
	"context"
	"errors"
	"sort"
	"strings"
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/formschema"
	"github.com/pletka-io/pletka/pkg/i18n"
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

// TestVocabularySchemaOffersOnlyFieldsItsEndpointSaves is the regression
// guard for the settings screen's worst failure mode: this form's single
// endpoint is PUT /projects/{id}/settings/vocabularies, whose handler
// (Handler.UpdateVocabularies) decodes enforce_concept_lists and
// concept_namespace and nothing else. Any other *editable* field in this
// schema is a control a curator can change, submit, and be told
// {"success":true} about while the change is discarded.
//
// vocabulary_ids and add_vocabulary_id stay in the schema as display —
// what the project owns, and what the service could serve — but must be
// readonly, because their real targets are the POST and DELETE endpoints
// on the same path and nothing is wired to those yet.
func TestVocabularySchemaOffersOnlyFieldsItsEndpointSaves(t *testing.T) {
	h := &Handler{serviceVocabularies: stubLister{out: []ServiceVocabulary{
		{Name: "fish-monument-type", Label: "FISH Monument Types", Concepts: 5009, Languages: []string{"en"}},
	}}}

	schema, err := h.vocabularySchema(context.Background(), "proj-1", VocabularySettingsState{
		Options: []VocabularySettingsOption{
			{ID: "voc-1", SystemName: "aat", Label: domain.Translations{"en": "AAT"}},
		},
	}, "en")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var editable []string
	for _, section := range schema.Sections {
		for _, field := range section.Fields {
			if !field.Readonly {
				editable = append(editable, field.Name)
			}
		}
	}
	sort.Strings(editable)

	want := []string{"concept_namespace", "enforce_concept_lists"}
	if len(editable) != len(want) {
		t.Fatalf("editable fields = %v, want exactly %v — every editable field must be one the PUT decodes", editable, want)
	}
	for i := range want {
		if editable[i] != want[i] {
			t.Fatalf("editable fields = %v, want exactly %v", editable, want)
		}
	}
}

// TestVocabularySchemaHelpDoesNotPromiseAGlobalTier pins the copy: this
// branch's whole point is that there is no global vocabulary tier, so the
// owned-vocabulary field must not tell a curator to choose among global
// sources.
func TestVocabularySchemaHelpDoesNotPromiseAGlobalTier(t *testing.T) {
	h := &Handler{serviceVocabularies: stubLister{}}

	schema, err := h.vocabularySchema(context.Background(), "proj-1", VocabularySettingsState{}, "en")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	help := helpText(t, findVocabularyField(t, schema, "vocabulary_ids"))
	if strings.Contains(strings.ToLower(help), "global") {
		t.Fatalf("vocabulary_ids help still describes a global tier: %q", help)
	}
}

// helpText reads a field's English help. Schema builders emit help as an
// i18n.LocalizedText (key + English fallback); the response edge enriches it
// from the bundle, but the fallback is the copy this test is about.
func helpText(t *testing.T, field formschema.FieldDef) string {
	t.Helper()
	lt, ok := field.Help.(i18n.LocalizedText)
	if !ok {
		t.Fatalf("help = %#v, want i18n.LocalizedText", field.Help)
	}
	return lt.Translations["en"]
}
