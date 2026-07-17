package weave

import (
	"encoding/json"

	"github.com/pletka-io/pletka/pkg/domain"
)

// Pointer + pgtype helpers used to live here (strPtr, derefStr,
// boolPtr, timestamptzToTimePtr, etc.) but they were duplicated in
// every slice store. They now live at pkg/database/dbutil and the
// callers import that package directly.
//
// The translation helpers below stay because they're domain-specific
// (the *Translations* JSON shape) and not generic pointer plumbing.

// ---------------------------------------------------------------------------
// Domain-level translation helpers (for weave_* stores using domain.Translations)
// ---------------------------------------------------------------------------

// marshalDomainTranslations converts a domain.Translations map to JSON bytes for storage.
func marshalDomainTranslations(t domain.Translations) []byte {
	if t == nil {
		return nil
	}
	b, err := json.Marshal(t)
	if err != nil {
		return nil
	}
	return b
}

// unmarshalDomainTranslations converts JSON bytes to a domain.Translations map.
func unmarshalDomainTranslations(b []byte) domain.Translations {
	if len(b) == 0 {
		return nil
	}
	var t domain.Translations
	if err := json.Unmarshal(b, &t); err != nil {
		return nil
	}
	return t
}

// marshalJSON marshals any value to JSON bytes. Returns nil on nil input or error.
func marshalJSON(v any) []byte {
	if v == nil {
		return nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return b
}

func marshalJSONArray(v any) []byte {
	if v == nil {
		return []byte("[]")
	}
	b, err := json.Marshal(v)
	if err != nil || len(b) == 0 || string(b) == "null" {
		return []byte("[]")
	}
	return b
}

func unmarshalVocabularyEntryRefs(b []byte) []domain.VocabularyEntryRef {
	if len(b) == 0 {
		return nil
	}
	var refs []domain.VocabularyEntryRef
	if err := json.Unmarshal(b, &refs); err != nil {
		return nil
	}
	return refs
}
