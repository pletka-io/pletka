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

// labelFor returns the one language it.Lang names, or nil when prefLabel is
// empty or lang is null — a subject with no skos:prefLabel at all. It does
// not iterate prefLabel to pick a language: lang is the contract's answer,
// and map iteration order is not deterministic.
func labelFor(it item) domain.Translations {
	if it.Lang == nil {
		return nil
	}
	value, ok := it.PrefLabel[*it.Lang]
	if !ok {
		return nil
	}
	return domain.Translations{*it.Lang: value}
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
