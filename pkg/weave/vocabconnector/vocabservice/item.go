package vocabservice

import (
	"strings"

	"github.com/pletka-io/pletka/pkg/domain"
)

// item is v2's one concept reference shape: a suggest hit, an entry of
// parents, of narrower, of children, and the concept fetch itself all decode
// into this. See docs/vocab-service.md "The one item shape" in the kakugo
// repo, the contract's authority.
type item struct {
	URI string `json:"uri"`
	// NarrowerTotal is the count of direct children — not of the whole
	// subtree. Contract v2.4+ sends it on every item; a pointer because zero
	// is a meaningful answer ("this is a leaf") that omitempty would erase on
	// the way back out.
	NarrowerTotal *int `json:"narrowerTotal,omitempty"`
	// DescendantsTotal is the whole-subtree size, sent only when a request
	// passes descendants=1 (contract v2.13); nil otherwise. A pointer for the
	// same nil-vs-zero reason as NarrowerTotal.
	DescendantsTotal *int              `json:"descendantsTotal,omitempty"`
	ID               string            `json:"id"`
	Kind             string            `json:"kind"`
	Class            string            `json:"class"`
	PrefLabel        map[string]string `json:"prefLabel"`
	Lang             *string           `json:"lang"`
}

// labelFor returns the language it.Lang names, plus "en" when the item has
// one and it is not already the display language — matching the contract's
// "the key chosen as lang, plus en when the concept has an English
// prefLabel too". On suggest the service has already narrowed prefLabel to
// (at most) those two keys; on concept, where prefLabel carries every
// language the concept has, this same rule narrows it down to at most two,
// which is why it lives here rather than at a call site. The pairing with
// English exists for the reader, not the wire: the frontend's tr() resolves
// a Translations map as requested language, then English, then whatever is
// there, so storing English alongside a non-English display value is what
// lets a curator working in a third language still see something familiar
// instead of falling through to a language they may not read at all.
//
// The display-language half of this is an exact lookup, deliberately with
// no fallback: the contract guarantees lang names a key of prefLabel
// specifically (nothing else), so a miss here would mean the service broke
// its own promise, not that a fallback is owed. Returns nil when prefLabel
// is empty or lang is null — a subject with no skos:prefLabel at all. It
// does not iterate prefLabel to pick lang, and it does not scan for "en"
// case-insensitively or fall back to a near-miss (e.g. "en-GB"): lang is the
// contract's answer for the display language, "en" is indexed directly, and
// an item with no exact "en" key simply gets one language, deterministically.
//
// scopeNoteFor below shares this exact same "display plus English" shape on
// its hit path, for the identical tr()-fallback reason. It is still a
// separate function, not a shared helper this one calls into, because the
// two differ on what happens on a miss: labelFor has none (lang is
// guaranteed to name a prefLabel key), scopeNoteFor falls back through
// English-alone then und (lang is only a preference for scopeNote, never a
// guarantee — see scopeNoteFor). Collapsing the two back into one function
// is exactly the mistake that shipped the bug scopeNoteFor's doc comment
// describes: a display-language miss on prefLabel is a broken contract
// promise, but a display-language miss on scopeNote is the ordinary case
// for anything that isn't the concept's most-translated field, and treating
// it as impossible silently drops (and, on Fetch, overwrites) the note.
func labelFor(it item) domain.Translations {
	if it.Lang == nil {
		return nil
	}
	value, ok := it.PrefLabel[*it.Lang]
	if !ok {
		return nil
	}
	labels := domain.Translations{*it.Lang: value}
	// A regional resolution is also stored under its primary subtag, so a
	// label the service answered as en-US is findable as en.
	//
	// The service matches by primary subtag (v2.5), so asking for en can
	// legitimately resolve to en-US — the key it then names is the exact one
	// it used, which is honest but not what anything looks it up by: the
	// frontend's tr() and Translations.Get() ask for the plain language. A
	// label stored only as en-US renders blank to a curator working in en.
	//
	// Only an alias, never an overwrite: a concept that has its own value for
	// the primary subtag keeps it, because that value is the language's own
	// label rather than a region's. And only when the subtags match, which
	// they do by construction here — a resolution whose primary subtag
	// differs from the request is a fallback, and aliasing that onto the
	// requested language is exactly the mislabelling to avoid.
	if base, _, regional := strings.Cut(*it.Lang, "-"); regional && base != "" {
		if _, ok := it.PrefLabel[base]; !ok {
			labels[base] = value
		}
	}
	if *it.Lang != "en" {
		if enValue, ok := it.PrefLabel["en"]; ok {
			labels["en"] = enValue
		}
	}
	return labels
}

// scopeNoteFor resolves a concept's scopeNote — a map the contract makes no
// promise about beyond "language-keyed". Unlike prefLabel, lang is not
// guaranteed to name a key of scopeNote: the service's only guarantee about
// lang is that it names a prefLabel key. Treating lang as an exact-only
// lookup here, with no further fallback, silently dropped a scope note in
// every language that happened to have none of its own — which on a Fetch
// does not just omit a field, it overwrites a scope note the row already
// had, since the persisted upsert has no guard against replacing a value
// with nothing.
//
// So on a miss this is a preference, not a lookup, and it falls back: the
// display language if scopeNote has it (see below for what "has it" stores),
// else English alone, else whatever untagged value the concept has, else
// nothing. und is deliberate, not an afterthought — the contract calls
// scopeNote out as the one predicate whose values can still arrive with no
// language tag at all, so skipping it would drop an untagged note exactly
// the way the display-language miss used to drop a tagged one.
//
// On a hit — the display language does have a scopeNote — the result is not
// just that one key. It is paired with English the same way labelFor pairs
// prefLabel, and for the identical reason: the frontend's tr() falls back
// to English before anything else, so a German scope note stored alone
// renders in German to a curator working in French, where storing the
// English pair alongside it renders in English instead. Do not read the
// shared shape here as an invitation to merge this back into labelFor: the
// two functions still differ on what happens when the display language is
// absent, which is the entire fix this function exists to make.
func scopeNoteFor(lang *string, values map[string]string) domain.Translations {
	if lang != nil {
		if value, ok := values[*lang]; ok {
			note := domain.Translations{*lang: value}
			if *lang != "en" {
				if enValue, ok := values["en"]; ok {
					note["en"] = enValue
				}
			}
			return note
		}
	}
	if value, ok := values["en"]; ok {
		return domain.Translations{"en": value}
	}
	if value, ok := values["und"]; ok {
		return domain.Translations{"und": value}
	}
	return nil
}

// labelText is labelFor's plain-string counterpart, used where a caller
// wants only the display text (a breadcrumb entry), not a language-keyed
// map. It returns "" for an item with no label, same as labelFor's nil.
func labelText(it item) string {
	if it.Lang == nil {
		return ""
	}
	return it.PrefLabel[*it.Lang]
}

// itemToRef fills a VocabularyEntryRef's URI, ExternalID and Label from it.
// Nothing else: a ref's other fields are the storing side's business.
func itemToRef(it item) domain.VocabularyEntryRef {
	return domain.VocabularyEntryRef{
		URI:        it.URI,
		ExternalID: it.ID,
		Label:      labelFor(it),
	}
}
