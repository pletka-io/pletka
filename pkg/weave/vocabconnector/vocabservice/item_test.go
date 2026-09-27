package vocabservice

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/pletka-io/pletka/pkg/domain"
)

// TestLabelForKeepsTheEnglishLabelAlongsideTheDisplayLanguage is fix round 1,
// item 1: the contract says an item's prefLabel holds "the key chosen as
// lang, plus en when the concept has an English prefLabel too" — v1 stored
// that pair deliberately, and the picker's tr() falls back to English, so
// dropping it means a curator working in a third language sees nothing
// where they previously saw the English label.
func TestLabelForKeepsTheEnglishLabelAlongsideTheDisplayLanguage(t *testing.T) {
	nl := "nl"
	it := item{
		PrefLabel: map[string]string{"nl": "brons", "en": "bronze (metal)"},
		Lang:      &nl,
	}
	got := labelFor(it)
	if diff := cmp.Diff(map[string]string{"nl": "brons", "en": "bronze (metal)"}, map[string]string(got)); diff != "" {
		t.Errorf("labelFor mismatch (-want +got):\n%s", diff)
	}
}

// TestLabelForOnAnEnglishHitStoresJustEnglish covers the "not already the
// display language" half of the ruling: an English hit must not duplicate
// the en key.
func TestLabelForOnAnEnglishHitStoresJustEnglish(t *testing.T) {
	en := "en"
	it := item{
		PrefLabel: map[string]string{"en": "bronze (metal)"},
		Lang:      &en,
	}
	got := labelFor(it)
	if diff := cmp.Diff(map[string]string{"en": "bronze (metal)"}, map[string]string(got)); diff != "" {
		t.Errorf("labelFor mismatch (-want +got):\n%s", diff)
	}
}

// TestLabelForOnAnItemWithNeitherLanguageStoresOnlyItsOwn covers an item
// whose one language is neither the display language's key nor English: it
// stores just that one, no English key present to add.
func TestLabelForOnAnItemWithNeitherLanguageStoresOnlyItsOwn(t *testing.T) {
	es := "es"
	it := item{
		PrefLabel: map[string]string{"es": "metal no ferroso"},
		Lang:      &es,
	}
	got := labelFor(it)
	if diff := cmp.Diff(map[string]string{"es": "metal no ferroso"}, map[string]string(got)); diff != "" {
		t.Errorf("labelFor mismatch (-want +got):\n%s", diff)
	}
}

// TestLabelForDoesNotScanCaseInsensitivelyForEnglish pins the ruling's other
// half: index "en" directly. A concept with "en-GB" but no "en" gets one
// language — that is acceptable and deterministic, not a bug to paper over
// with a case-insensitive or fallback scan (that is what v1 did, and what
// several review rounds went into removing).
func TestLabelForDoesNotScanCaseInsensitivelyForEnglish(t *testing.T) {
	nl := "nl"
	it := item{
		PrefLabel: map[string]string{"nl": "brons", "EN": "bronze (metal)", "en-GB": "bronze (metal)"},
		Lang:      &nl,
	}
	got := labelFor(it)
	if diff := cmp.Diff(map[string]string{"nl": "brons"}, map[string]string(got)); diff != "" {
		t.Errorf("labelFor mismatch (-want +got):\n%s", diff)
	}
}

// TestLabelForAliasesARegionalResolutionToItsPrimarySubtag covers the gap the
// service's BCP-47 matching opened. Asking for `en` can legitimately resolve
// to `en-US` — the service names the exact key it used — but the frontend's
// tr() and Translations.Get() look a label up by the plain language, so a
// label stored only under en-US renders blank to a curator working in en.
func TestLabelForAliasesARegionalResolutionToItsPrimarySubtag(t *testing.T) {
	lang := func(s string) *string { return &s }

	tests := []struct {
		name string
		it   item
		want domain.Translations
	}{
		{
			// The live shape: AAT concept 300056134 has en-US and en-GB and no
			// bare en, and `lang=en` resolves to en-US.
			name: "a regional resolution is also stored under its base language",
			it: item{
				Lang: lang("en-US"),
				PrefLabel: map[string]string{
					"en-US": "film color (color)",
					"en-GB": "film colour (color)", //nolint:misspell // the real en-GB label; the en-GB/en-US split is what this test is about
					"es":    "color en película",
				},
			},
			want: domain.Translations{"en-US": "film color (color)", "en": "film color (color)"},
		},
		{
			// The concept's own value for the language wins: it is the
			// language's label, not a region's.
			name: "an existing base-language value is never overwritten",
			it: item{
				Lang: lang("en-GB"),
				PrefLabel: map[string]string{
					"en-GB": "colour", //nolint:misspell // ditto: the British spelling is the point
					"en":    "color",
				},
			},
			want: domain.Translations{"en-GB": "colour", "en": "color"}, //nolint:misspell // ditto
		},
		{
			name: "a plain language resolution is unchanged",
			it: item{
				Lang:      lang("nl"),
				PrefLabel: map[string]string{"nl": "brons", "en": "bronze (metal)"},
			},
			want: domain.Translations{"nl": "brons", "en": "bronze (metal)"},
		},
		{
			// Aliasing a non-English regional tag must still pair English, so
			// the frontend's English fallback keeps working.
			name: "a regional non-English resolution aliases and still pairs English",
			it: item{
				Lang:      lang("zh-Hant"),
				PrefLabel: map[string]string{"zh-Hant": "青銅", "en": "bronze (metal)"},
			},
			want: domain.Translations{"zh-Hant": "青銅", "zh": "青銅", "en": "bronze (metal)"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := labelFor(tc.it)
			if len(got) != len(tc.want) {
				t.Fatalf("labelFor = %v, want %v", got, tc.want)
			}
			for k, v := range tc.want {
				if got[k] != v {
					t.Errorf("labelFor[%q] = %q, want %q (full: %v)", k, got[k], v, got)
				}
			}
		})
	}
}
