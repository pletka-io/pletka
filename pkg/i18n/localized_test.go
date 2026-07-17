package i18n_test

import (
	"encoding/json"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/pletka-io/pletka/pkg/domain"

	"github.com/pletka-io/pletka/pkg/i18n"
)

// newTestManager builds a memory-backed Manager with English + Dutch
// as the enabled languages so resolveLocalized has something to walk.
func newTestManager(t *testing.T) i18n.Manager {
	t.Helper()
	m, err := newMemoryManager(i18n.Config{
		Languages: []i18n.Language{
			{Code: "en", Name: "English", Enabled: true},
			{Code: "nl", Name: "Nederlands", Enabled: true},
		},
	})
	if err != nil {
		t.Fatalf("build manager: %v", err)
	}
	return m
}

func TestL_BakesEnglishFallback(t *testing.T) {
	lt := i18n.L("forms.cancel", "Cancel")
	if got, want := lt.Key, "forms.cancel"; got != want {
		t.Errorf("Key = %q, want %q", got, want)
	}
	if got, want := lt.Translations["en"], "Cancel"; got != want {
		t.Errorf("Translations[en] = %q, want %q", got, want)
	}
}

func TestLocalizedText_MarshalJSON_DropsKey(t *testing.T) {
	lt := i18n.L("forms.cancel", "Cancel")
	out, err := json.Marshal(lt)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(out)
	want := `{"en":"Cancel"}`
	if got != want {
		t.Errorf("MarshalJSON = %s, want %s", got, want)
	}
}

func TestLF_BakesArgsAndFallback(t *testing.T) {
	lt := i18n.LF("release.viewing", "Viewing release {version}",
		map[string]string{"version": "1.0.0"})
	if got, want := lt.Key, "release.viewing"; got != want {
		t.Errorf("Key = %q, want %q", got, want)
	}
	if got, want := lt.Args["version"], "1.0.0"; got != want {
		t.Errorf("Args[version] = %q, want %q", got, want)
	}
	if got, want := lt.Translations["en"], "Viewing release {version}"; got != want {
		t.Errorf("Translations[en] = %q, want %q", got, want)
	}
}

func TestResolve_SubstitutesArgsAcrossLanguages(t *testing.T) {
	m := newTestManager(t)
	if err := m.Set("release.viewing", "nl", &i18n.Translation{Value: "Release {version} bekijken", Status: i18n.StatusApproved}); err != nil {
		t.Fatalf("seed nl: %v", err)
	}
	if err := m.Set("release.viewing", "en", &i18n.Translation{Value: "Viewing release {version}", Status: i18n.StatusApproved}); err != nil {
		t.Fatalf("seed en: %v", err)
	}

	type form struct {
		Label i18n.LocalizedText
	}
	f := &form{Label: i18n.LF("release.viewing", "Viewing release {version}",
		map[string]string{"version": "1.0.0"})}

	m.Resolve(f, "nl")

	want := domain.Translations{"en": "Viewing release 1.0.0", "nl": "Release 1.0.0 bekijken"}
	if diff := cmp.Diff(want, f.Label.Translations); diff != "" {
		t.Errorf("Translations mismatch (-want +got):\n%s", diff)
	}
	if f.Label.Args != nil {
		t.Errorf("Args should be nil after Resolve, got %v", f.Label.Args)
	}
}

func TestLF_MarshalJSON_SubstitutesArgsWithoutResolve(t *testing.T) {
	lt := i18n.LF("release.viewing", "Viewing release {version}",
		map[string]string{"version": "1.0.0"})
	out, err := json.Marshal(lt)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if got, want := string(out), `{"en":"Viewing release 1.0.0"}`; got != want {
		t.Errorf("MarshalJSON = %s, want %s", got, want)
	}
}

func TestLF_UnknownPlaceholderPassesThrough(t *testing.T) {
	lt := i18n.LF("release.viewing", "Viewing release {version} on {date}",
		map[string]string{"version": "1.0.0"})
	out, err := json.Marshal(lt)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if got, want := string(out), `{"en":"Viewing release 1.0.0 on {date}"}`; got != want {
		t.Errorf("MarshalJSON = %s, want %s", got, want)
	}
}

func TestLocalizedText_MarshalJSON_NilTranslations(t *testing.T) {
	lt := i18n.LocalizedText{Key: "x"}
	out, err := json.Marshal(lt)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if got, want := string(out), "null"; got != want {
		t.Errorf("MarshalJSON = %s, want %s", got, want)
	}
}

func TestResolve_FillsTranslationsFromBundle(t *testing.T) {
	m := newTestManager(t)
	if err := m.Set("forms.cancel", "nl", &i18n.Translation{Value: "Annuleren", Status: i18n.StatusApproved}); err != nil {
		t.Fatalf("seed nl: %v", err)
	}
	if err := m.Set("forms.cancel", "en", &i18n.Translation{Value: "Cancel", Status: i18n.StatusApproved}); err != nil {
		t.Fatalf("seed en: %v", err)
	}

	// Schema with one LocalizedText buried in a struct off a pointer —
	// the typical case for formschema FieldDef.Label.
	type field struct {
		Name  string
		Label i18n.LocalizedText
	}
	type form struct {
		Fields []field
	}
	f := &form{Fields: []field{{Name: "x", Label: i18n.L("forms.cancel", "Cancel")}}}

	m.Resolve(f, "nl")

	want := domain.Translations{"en": "Cancel", "nl": "Annuleren"}
	if diff := cmp.Diff(want, f.Fields[0].Label.Translations); diff != "" {
		t.Errorf("Translations mismatch (-want +got):\n%s", diff)
	}
}

func TestResolve_KeepsFallbackWhenBundleMisses(t *testing.T) {
	m := newTestManager(t)
	// No translations seeded — bundle completely empty.

	type form struct {
		Label i18n.LocalizedText
	}
	f := &form{Label: i18n.L("forms.cancel", "Cancel")}
	m.Resolve(f, "nl")

	if got, want := f.Label.Translations["en"], "Cancel"; got != want {
		t.Errorf("English fallback lost: got %q, want %q", got, want)
	}
}

func TestResolve_PassThroughRawTranslations(t *testing.T) {
	m := newTestManager(t)
	if err := m.Set("forms.cancel", "en", &i18n.Translation{Value: "Cancel-bundle", Status: i18n.StatusApproved}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	type form struct {
		Raw     domain.Translations
		Localed i18n.LocalizedText
	}
	raw := domain.Translations{"en": "Untouched"}
	f := &form{
		Raw:     raw,
		Localed: i18n.L("forms.cancel", "Cancel"),
	}
	m.Resolve(f, "en")

	// Raw map must not be touched even though it sits next to a
	// LocalizedText in the same parent struct.
	if got, want := f.Raw["en"], "Untouched"; got != want {
		t.Errorf("Raw was modified: got %q, want %q", got, want)
	}
	// LocalizedText was resolved.
	if got, want := f.Localed.Translations["en"], "Cancel-bundle"; got != want {
		t.Errorf("Localed not resolved: got %q, want %q", got, want)
	}
}

func TestResolve_NestedSliceAndMap(t *testing.T) {
	m := newTestManager(t)
	if err := m.Set("a", "en", &i18n.Translation{Value: "A", Status: i18n.StatusApproved}); err != nil {
		t.Fatalf("seed a: %v", err)
	}
	if err := m.Set("b", "en", &i18n.Translation{Value: "B", Status: i18n.StatusApproved}); err != nil {
		t.Fatalf("seed b: %v", err)
	}

	// Map values aren't addressable in Go reflect — the walker must
	// clone, walk, and set back. Test that path explicitly.
	type form struct {
		Slice   []i18n.LocalizedText
		Pointer *i18n.LocalizedText
		Map     map[string]i18n.LocalizedText
	}
	pointed := i18n.L("a", "Aen")
	f := &form{
		Slice:   []i18n.LocalizedText{i18n.L("a", "Aen"), i18n.L("b", "Ben")},
		Pointer: &pointed,
		Map:     map[string]i18n.LocalizedText{"a": i18n.L("a", "Aen")},
	}
	m.Resolve(f, "en")

	if got, want := f.Slice[0].Translations["en"], "A"; got != want {
		t.Errorf("Slice[0] not resolved: %q", got)
	}
	if got, want := f.Slice[1].Translations["en"], "B"; got != want {
		t.Errorf("Slice[1] not resolved: %q", got)
	}
	if got, want := f.Pointer.Translations["en"], "A"; got != want {
		t.Errorf("Pointer not resolved: %q", got)
	}
	if got, want := f.Map["a"].Translations["en"], "A"; got != want {
		t.Errorf("Map[a] not resolved: %q", got)
	}
}

// FormSchema field types like `FieldDef.Label any` carry the
// LocalizedText through an interface. The Elem() of an interface value
// isn't addressable, so the walker has to read-enrich-writeback rather
// than mutate in place.
func TestResolve_LocalizedTextInsideAnyField(t *testing.T) {
	m := newTestManager(t)
	if err := m.Set("forms.cancel", "nl", &i18n.Translation{Value: "Annuleren", Status: i18n.StatusApproved}); err != nil {
		t.Fatalf("seed nl: %v", err)
	}

	type schemaUI struct {
		CancelLabel any
		Other       any // raw Translations, must pass through
	}
	type schema struct {
		UI schemaUI
	}

	s := &schema{
		UI: schemaUI{
			CancelLabel: i18n.L("forms.cancel", "Cancel"),
			Other:       domain.Translations{"en": "Untouched"},
		},
	}
	m.Resolve(s, "nl")

	lt, ok := s.UI.CancelLabel.(i18n.LocalizedText)
	if !ok {
		t.Fatalf("CancelLabel type changed: %T", s.UI.CancelLabel)
	}
	if got, want := lt.Translations["nl"], "Annuleren"; got != want {
		t.Errorf("nl not resolved through interface: got %q, want %q", got, want)
	}
	if got, want := lt.Translations["en"], "Cancel"; got != want {
		t.Errorf("en fallback lost: got %q, want %q", got, want)
	}

	other, ok := s.UI.Other.(domain.Translations)
	if !ok {
		t.Fatalf("Other type changed: %T", s.UI.Other)
	}
	if got, want := other["en"], "Untouched"; got != want {
		t.Errorf("raw Translations modified: got %q, want %q", got, want)
	}
}

func TestResolve_NilSafe(t *testing.T) {
	m := newTestManager(t)
	// None of these should panic.
	m.Resolve(nil, "en")
	var f *struct{ Label i18n.LocalizedText }
	m.Resolve(f, "en")
}
