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
// concept_namespace and nothing else. A field a curator can change and
// submit, and be told {"success":true} about while the change is discarded,
// is the bug this pins.
//
// "Editable" therefore means editable AND carried by the form's submit.
// vocabulary_ids and add_vocabulary_id are now both live controls, but they
// are self-managing: their controls call the POST and DELETE endpoints on
// the same path directly, and FormRenderer excludes any field carrying
// item_add_url or item_remove_url_template from the submit payload. They are
// exempt because the PUT never sees them, not because they are inert — the
// property under test is that nothing reaches that PUT which it would throw
// away.
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

	var submitted []string
	var selfManaged []string
	for _, section := range schema.Sections {
		for _, field := range section.Fields {
			if field.Readonly {
				continue
			}
			if field.ItemAddURL != "" || field.ItemRemoveURLTemplate != "" {
				selfManaged = append(selfManaged, field.Name)
				continue
			}
			submitted = append(submitted, field.Name)
		}
	}
	sort.Strings(submitted)
	sort.Strings(selfManaged)

	want := []string{"concept_namespace", "enforce_concept_lists"}
	if len(submitted) != len(want) {
		t.Fatalf("fields carried by the PUT = %v, want exactly %v — every one must be a field the PUT decodes", submitted, want)
	}
	for i := range want {
		if submitted[i] != want[i] {
			t.Fatalf("fields carried by the PUT = %v, want exactly %v", submitted, want)
		}
	}

	// The exemption has to be earned: a field is only allowed out of the
	// payload because it has somewhere else to go. Without this, marking any
	// field self-managing and wiring it to nothing would pass the check
	// above while silently discarding input again.
	wantSelfManaged := []string{"add_vocabulary_id", "vocabulary_ids"}
	if len(selfManaged) != len(wantSelfManaged) {
		t.Fatalf("self-managing fields = %v, want exactly %v", selfManaged, wantSelfManaged)
	}
	for i := range wantSelfManaged {
		if selfManaged[i] != wantSelfManaged[i] {
			t.Fatalf("self-managing fields = %v, want exactly %v", selfManaged, wantSelfManaged)
		}
	}
	if url := findVocabularyField(t, schema, "add_vocabulary_id").ItemAddURL; url != "/projects/proj-1/settings/vocabularies" {
		t.Errorf("add_vocabulary_id item_add_url = %q, want the project's vocabularies endpoint", url)
	}
	if url := findVocabularyField(t, schema, "vocabulary_ids").ItemRemoveURLTemplate; url != "/projects/proj-1/settings/vocabularies/{id}" {
		t.Errorf("vocabulary_ids item_remove_url_template = %q, want the per-vocabulary endpoint", url)
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

// TestCollidingServiceLabelsCarryTheirMountName pins the disambiguation. The
// service labels four GeoNames mounts and two Iconclass mounts identically,
// differing only by mount name and size, and a native select shows only the
// label — so without this a curator picking between them is choosing blind.
//
// Unique labels must stay clean: annotating every option would make the
// common case noisier to fix the rare one.
func TestCollidingServiceLabelsCarryTheirMountName(t *testing.T) {
	h := &Handler{serviceVocabularies: stubLister{out: []ServiceVocabulary{
		{Name: "geonames", Label: "GeoNames", Concepts: 5775923},
		{Name: "geonames-water", Label: "GeoNames", Concepts: 3155630},
		{Name: "aat", Label: "Art & Architecture Thesaurus", Concepts: 59300},
		{Name: "unlabelled-mount"},
	}}}

	schema, err := h.vocabularySchema(context.Background(), "proj-1", VocabularySettingsState{}, "en")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	labels := map[string]string{}
	for _, opt := range findVocabularyField(t, schema, "add_vocabulary_id").Options {
		labels[opt.Value] = tr(t, opt.Label)
	}

	if got := labels["geonames"]; got != "GeoNames (geonames)" {
		t.Errorf("geonames label = %q, want the mount name appended", got)
	}
	if got := labels["geonames-water"]; got != "GeoNames (geonames-water)" {
		t.Errorf("geonames-water label = %q, want the mount name appended", got)
	}
	if got := labels["aat"]; got != "Art & Architecture Thesaurus" {
		t.Errorf("aat label = %q, want it left clean — its label does not collide", got)
	}
	if got := labels["unlabelled-mount"]; got != "unlabelled-mount" {
		t.Errorf("unlabelled mount label = %q, want the mount name", got)
	}
}

// tr reads an option's English label, whichever localizable shape it carries.
func tr(t *testing.T, label domain.Localizable) string {
	t.Helper()
	switch v := label.(type) {
	case domain.Translations:
		return v.Get("en")
	case i18n.LocalizedText:
		return v.Translations["en"]
	default:
		t.Fatalf("label = %#v, want a known localizable shape", label)
		return ""
	}
}
