package vocabservice

import "github.com/pletka-io/pletka/pkg/domain"

// item is v2's one concept reference shape: a suggest hit, an entry of
// parents, of narrower, of children, and the concept fetch itself all decode
// into this. See docs/vocab-service.md "The one item shape" in the kakugo
// repo, the contract's authority.
type item struct {
	URI       string            `json:"uri"`
	ID        string            `json:"id"`
	Kind      string            `json:"kind"`
	Class     string            `json:"class"`
	PrefLabel map[string]string `json:"prefLabel"`
	Lang      *string           `json:"lang"`
}

// labelFor returns the language it.Lang names, plus "en" when the item has
// one and it is not already the display language — matching the contract's
// "the key chosen as lang, plus en when the concept has an English
// prefLabel too". On suggest the service has already narrowed prefLabel to
// (at most) those two keys; on concept, where prefLabel carries every
// language the concept has, this same rule narrows it down to at most two,
// which is why it lives here rather than at a call site.
//
// This is an exact lookup, deliberately with no fallback: the contract
// guarantees lang names a key of prefLabel specifically (nothing else), so a
// miss here would mean the service broke its own promise, not that a
// fallback is owed. Returns nil when prefLabel is empty or lang is null — a
// subject with no skos:prefLabel at all. It does not iterate prefLabel to
// pick lang, and it does not scan for "en" case-insensitively or fall back
// to a near-miss (e.g. "en-GB"): lang is the contract's answer for the
// display language, "en" is indexed directly, and an item with no exact
// "en" key simply gets one language, deterministically.
func labelFor(it item) domain.Translations {
	if it.Lang == nil {
		return nil
	}
	value, ok := it.PrefLabel[*it.Lang]
	if !ok {
		return nil
	}
	labels := domain.Translations{*it.Lang: value}
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
// lang is that it names a prefLabel key. Treating lang as an exact key here
// too (labelFor's rule) would silently drop a scope note in every language
// that happens to have none of its own — which on a Fetch does not just omit
// a field, it overwrites a scope note the row already had, since the
// persisted upsert has no guard against replacing a value with nothing.
//
// So this is a preference, not a lookup, and it falls back: the display
// language if scopeNote has it, else English, else whatever untagged value
// the concept has, else nothing. und is deliberate, not an afterthought —
// the contract calls scopeNote out as the one predicate whose values can
// still arrive with no language tag at all, so skipping it would drop an
// untagged note exactly the way the display-language miss used to drop a
// tagged one. The result is narrowed to the one language actually found,
// never the whole map.
func scopeNoteFor(lang *string, values map[string]string) domain.Translations {
	if lang != nil {
		if value, ok := values[*lang]; ok {
			return domain.Translations{*lang: value}
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
