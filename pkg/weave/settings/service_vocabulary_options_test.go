package settings

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/i18n"
)

// stubLister is a test double for ServiceVocabularyLister. It carries no
// production wiring on its own — see pkg/app's real adapter tests for that —
// it exists only so buildServiceVocabularyOptions can be driven through its
// three documented outcomes without a real vocabulary service.
type stubLister struct {
	out []ServiceVocabulary
	err error
}

func (s stubLister) ServiceVocabularies(_ context.Context) ([]ServiceVocabulary, error) {
	return s.out, s.err
}

// localizableEnglish extracts the English string out of a domain.Localizable
// value. domain.Localizable is a marker interface (Get isn't part of its
// method set), so callers that need the text must go through the concrete
// type — this mirrors the helper already duplicated in
// pkg/formschema/composition_sidebar_test.go and
// pkg/formschema/namespace_binding_list_test.go.
func localizableEnglish(v domain.Localizable) string {
	switch t := any(v).(type) {
	case domain.Translations:
		return t.Get("en")
	case i18n.LocalizedText:
		// LocalizedText embeds domain.Translations, so Get promotes — naming
		// the embedded field here is what staticcheck QF1008 flags.
		return t.Get("en")
	default:
		return ""
	}
}

// Review Focus 3: zero mounts is a documented, legal response.
func TestBuildServiceVocabularyOptionsWithNoMounts(t *testing.T) {
	opts, err := buildServiceVocabularyOptions(context.Background(), stubLister{out: nil})
	if err != nil {
		t.Fatalf("zero mounts must not be an error: %v", err)
	}
	if len(opts) != 0 {
		t.Errorf("options = %v, want none", opts)
	}
}

// Review Focus 2: the one case the spec says must not look like "none exist".
func TestBuildServiceVocabularyOptionsSurfacesAnUnreachableService(t *testing.T) {
	_, err := buildServiceVocabularyOptions(context.Background(), stubLister{err: errors.New("dial tcp: connection refused")})
	if err == nil {
		t.Fatal("an unreachable service must surface as an error, not an empty list")
	}
}

func TestBuildServiceVocabularyOptionsWithNoServiceConfigured(t *testing.T) {
	opts, err := buildServiceVocabularyOptions(context.Background(), nil)
	if err != nil {
		t.Fatalf("no configured service is not an error: %v", err)
	}
	if len(opts) != 0 {
		t.Errorf("options = %v, want none", opts)
	}
}

func TestServiceVocabularyOptionCarriesItsLanguages(t *testing.T) {
	opts, err := buildServiceVocabularyOptions(context.Background(), stubLister{out: []ServiceVocabulary{
		{Name: "fish-monument-type", Label: "FISH Monument Types", Concepts: 5009, Languages: []string{"en"}},
	}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(opts) != 1 {
		t.Fatalf("options = %d, want 1", len(opts))
	}
	// Thirteen of twenty live mounts serve English only. A curator choosing one
	// for a Dutch project needs to see that here, not discover it from a picker
	// that silently answers in English.
	if desc := localizableEnglish(opts[0].Description); !strings.Contains(desc, "en") {
		t.Errorf("option description = %q, want it to name the languages the mount serves", desc)
	}
}
