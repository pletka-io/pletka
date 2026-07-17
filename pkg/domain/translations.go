package domain

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"maps"
)

// Localizable is the marker interface satisfied by any value representing
// localizable copy. Translations satisfies it directly; i18n.LocalizedText
// satisfies it via embedding. Schema fields use this type to enforce
// "localizable copy here" at compile time without committing to a concrete
// representation.
type Localizable interface {
	isLocalizable()
}

// Translations is a map of language code to text, stored as JSONB. It is the
// shared multilingual map type used across the domain.
type Translations map[string]string

// isLocalizable marks Translations as a Localizable. Empty body — the method
// exists purely so the type satisfies the interface.
func (Translations) isLocalizable() {}

// Value implements the driver.Valuer interface for database storage.
func (t Translations) Value() (driver.Value, error) {
	if t == nil {
		return nil, nil
	}
	return json.Marshal(t)
}

// Scan implements the sql.Scanner interface for database retrieval.
func (t *Translations) Scan(value any) error {
	if value == nil {
		*t = nil
		return nil
	}

	bytes, ok := value.([]byte)
	if !ok {
		return errors.New("type assertion to []byte failed")
	}

	return json.Unmarshal(bytes, t)
}

// Get returns the translation for a given language code with fallback.
// Tries the requested language, then each fallback as a language key, then
// English, then the first available translation, then fallbacks as literals.
func (t Translations) Get(lang string, fallback ...string) string {
	if t == nil {
		for _, fb := range fallback {
			if fb != "" {
				return fb
			}
		}
		return ""
	}

	if val, ok := t[lang]; ok && val != "" {
		return val
	}

	for _, fb := range fallback {
		if val, ok := t[fb]; ok && val != "" {
			return val
		}
	}

	if val, ok := t["en"]; ok && val != "" {
		return val
	}

	for _, val := range t {
		if val != "" {
			return val
		}
	}

	for _, fb := range fallback {
		if fb != "" {
			return fb
		}
	}

	return ""
}

// Set adds or updates a translation for the given language.
func (t *Translations) Set(lang, value string) {
	if *t == nil {
		*t = make(Translations)
	}
	(*t)[lang] = value
}

// Has reports whether a non-empty translation exists for the given language.
func (t Translations) Has(lang string) bool {
	val, ok := t[lang]
	return ok && val != ""
}

// Languages returns all language codes present in the map.
func (t Translations) Languages() []string {
	langs := make([]string, 0, len(t))
	for lang := range t {
		langs = append(langs, lang)
	}
	return langs
}

// IsEmpty reports whether the translations map contains no entries.
func (t Translations) IsEmpty() bool {
	return len(t) == 0
}

// Clone creates a deep copy of the translations.
func (t Translations) Clone() Translations {
	if t == nil {
		return nil
	}
	clone := make(Translations, len(t))
	maps.Copy(clone, t)
	return clone
}
