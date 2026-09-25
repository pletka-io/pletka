package vocabservice

import (
	"testing"

	"github.com/google/go-cmp/cmp"
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
