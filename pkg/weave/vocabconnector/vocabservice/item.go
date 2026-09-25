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
// language the concept has, this same rule is the narrowing to at most two
// that Task 2 needs, which is why it lives here rather than at a call site.
//
// Returns nil when prefLabel is empty or lang is null — a subject with no
// skos:prefLabel at all. It does not iterate prefLabel to pick lang, and it
// does not scan for "en" case-insensitively or fall back to a near-miss
// (e.g. "en-GB"): lang is the contract's answer for the display language,
// "en" is indexed directly, and an item with no exact "en" key simply gets
// one language, deterministically.
func labelFor(it item) domain.Translations {
	return narrowByLang(it.Lang, it.PrefLabel)
}

// narrowByLang applies labelFor's rule to any language-keyed map, not just an
// item's own prefLabel. concept/{id} needs the same narrowing a second time,
// for scopeNote: that field carries every language the concept has, exactly
// like concept's prefLabel does, and for the same reason a stored row must
// not carry a concept's whole language set. One rule, shared, rather than a
// second one invented for scopeNote.
func narrowByLang(lang *string, values map[string]string) domain.Translations {
	if lang == nil {
		return nil
	}
	value, ok := values[*lang]
	if !ok {
		return nil
	}
	labels := domain.Translations{*lang: value}
	if *lang != "en" {
		if enValue, ok := values["en"]; ok {
			labels["en"] = enValue
		}
	}
	return labels
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
