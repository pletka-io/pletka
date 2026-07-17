package i18n

import (
	"fmt"
	"strings"
)

// TN returns a pluralized translation
func (m *manager) TN(key string, count int, lang string, args ...any) string {
	trans, err := m.getTranslation(key, "", lang)
	if err != nil {
		if m.config.DebugMode {
			return fmt.Sprintf("missing_plural_%s", key)
		}
		return key
	}
	
	// Get the appropriate plural form
	form := getPluralForm(lang, count)
	
	// Check if we have plural forms
	if trans.PluralForms != nil && len(trans.PluralForms) > 0 {
		if pluralValue, ok := trans.PluralForms[form]; ok {
			// Replace {count} or {0} with the actual count
			result := strings.ReplaceAll(pluralValue, "{count}", fmt.Sprintf("%d", count))
			result = strings.ReplaceAll(result, "{0}", fmt.Sprintf("%d", count))
			
			// Apply any additional arguments
			if len(args) > 0 {
				return fmt.Sprintf(result, args...)
			}
			return result
		}
	}
	
	// Fallback to simple singular/plural from pipe-separated format
	// e.g., "1 item|{0} items"
	if strings.Contains(trans.Value, "|") {
		parts := strings.Split(trans.Value, "|")
		var result string
		
		if count == 1 && len(parts) > 0 {
			result = parts[0]
		} else if len(parts) > 1 {
			result = parts[1]
		} else {
			result = trans.Value
		}
		
		// Replace placeholders
		result = strings.ReplaceAll(result, "{count}", fmt.Sprintf("%d", count))
		result = strings.ReplaceAll(result, "{0}", fmt.Sprintf("%d", count))
		
		if len(args) > 0 {
			return fmt.Sprintf(result, args...)
		}
		return result
	}
	
	// No plural form found, use singular with count
	return fmt.Sprintf("%d %s", count, trans.Value)
}

// getPluralForm returns the appropriate plural form for a language and count
// Based on CLDR plural rules: http://cldr.unicode.org/index/cldr-spec/plural-rules
func getPluralForm(lang string, count int) PluralForm {
	// Handle special cases first
	if count == 0 {
		// Some languages have special zero form
		switch lang {
		case "ar", "cy", "he":
			return PluralZero
		}
	}
	
	// Get language family (handle variants like en-US)
	baseLang := strings.Split(lang, "-")[0]
	
	switch baseLang {
	// Languages with only singular/plural (Germanic, Romance, etc.)
	case "en", "de", "nl", "sv", "da", "no", "nb", "nn", // Germanic
		"es", "it", "pt", "ca", // Romance
		"fi", "et", "hu", // Finno-Ugric
		"tr", "az": // Turkic
		if count == 1 {
			return PluralOne
		}
		return PluralOther
		
	// French and Brazilian Portuguese (0,1 = one; else other)
	case "fr", "pt-BR":
		if count == 0 || count == 1 {
			return PluralOne
		}
		return PluralOther
		
	// Slavic languages with complex rules
	case "pl": // Polish
		if count == 1 {
			return PluralOne
		}
		if count%10 >= 2 && count%10 <= 4 && (count%100 < 12 || count%100 > 14) {
			return PluralFew
		}
		return PluralMany
		
	case "cs", "sk": // Czech, Slovak
		if count == 1 {
			return PluralOne
		}
		if count >= 2 && count <= 4 {
			return PluralFew
		}
		return PluralOther
		
	case "ru", "uk", "be": // Russian, Ukrainian, Belarusian
		if count%10 == 1 && count%100 != 11 {
			return PluralOne
		}
		if count%10 >= 2 && count%10 <= 4 && (count%100 < 12 || count%100 > 14) {
			return PluralFew
		}
		return PluralMany
		
	// Celtic languages
	case "ga": // Irish
		if count == 1 {
			return PluralOne
		}
		if count == 2 {
			return PluralTwo
		}
		if count >= 3 && count <= 6 {
			return PluralFew
		}
		if count >= 7 && count <= 10 {
			return PluralMany
		}
		return PluralOther
		
	// Arabic
	case "ar":
		if count == 0 {
			return PluralZero
		}
		if count == 1 {
			return PluralOne
		}
		if count == 2 {
			return PluralTwo
		}
		if count%100 >= 3 && count%100 <= 10 {
			return PluralFew
		}
		if count%100 >= 11 && count%100 <= 99 {
			return PluralMany
		}
		return PluralOther
		
	// Japanese, Chinese, Korean, Thai, Vietnamese (no plurals)
	case "ja", "zh", "ko", "th", "vi", "id", "ms":
		return PluralOther
		
	// Default: simple singular/plural
	default:
		if count == 1 {
			return PluralOne
		}
		return PluralOther
	}
}

// PluralRule represents a plural rule for a language
type PluralRule struct {
	Language string
	Rules    map[PluralForm]func(n int) bool
}

// GetPluralRules returns plural rules for common languages
func GetPluralRules() map[string]PluralRule {
	return map[string]PluralRule{
		"en": {
			Language: "en",
			Rules: map[PluralForm]func(n int) bool{
				PluralOne:   func(n int) bool { return n == 1 },
				PluralOther: func(n int) bool { return n != 1 },
			},
		},
		"pl": {
			Language: "pl",
			Rules: map[PluralForm]func(n int) bool{
				PluralOne: func(n int) bool { return n == 1 },
				PluralFew: func(n int) bool {
					return n%10 >= 2 && n%10 <= 4 && (n%100 < 12 || n%100 > 14)
				},
				PluralMany: func(n int) bool {
					return n != 1 && (n%10 < 2 || n%10 > 4) || (n%100 >= 12 && n%100 <= 14)
				},
			},
		},
		"ar": {
			Language: "ar",
			Rules: map[PluralForm]func(n int) bool{
				PluralZero: func(n int) bool { return n == 0 },
				PluralOne:  func(n int) bool { return n == 1 },
				PluralTwo:  func(n int) bool { return n == 2 },
				PluralFew:  func(n int) bool { return n%100 >= 3 && n%100 <= 10 },
				PluralMany: func(n int) bool { return n%100 >= 11 && n%100 <= 99 },
				PluralOther: func(n int) bool { return true },
			},
		},
	}
}