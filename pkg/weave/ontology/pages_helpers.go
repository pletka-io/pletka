package ontology

import (
	"strings"

	"github.com/pletka-io/pletka/pkg/domain"
)

// localizedText returns the best available translation for lang: exact
// match, then English, then any non-empty value.
func localizedText(ts domain.Translations, lang string) string {
	if len(ts) == 0 {
		return ""
	}
	if value := strings.TrimSpace(ts[lang]); value != "" {
		return value
	}
	if value := strings.TrimSpace(ts["en"]); value != "" {
		return value
	}
	for _, value := range ts {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

// ontologyTypeLabel renders the human label for an ontology kind.
func ontologyTypeLabel(kind domain.OntologyType) string {
	if kind == domain.OntologyTypeExtension {
		return "Extension"
	}
	return "Base"
}

// summarizeText truncates text to roughly limit runes on a word boundary.
func summarizeText(text string, limit int) string {
	preview, _ := previewText(text, limit)
	return preview
}

// previewText returns the (possibly truncated) text and whether it was cut.
func previewText(text string, limit int) (string, bool) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" || limit <= 0 {
		return trimmed, false
	}
	runes := []rune(trimmed)
	if len(runes) <= limit {
		return trimmed, false
	}
	cut := limit
	for cut > limit/2 && cut < len(runes) && runes[cut] != ' ' {
		cut--
	}
	if cut <= limit/2 {
		cut = limit
	}
	return strings.TrimSpace(string(runes[:cut])) + "...", true
}

// firstNonEmpty returns the first non-blank value.
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

// familyName is a nil-safe accessor for a family's display name.
func familyName(family *domain.OntologyFamily) string {
	if family == nil {
		return ""
	}
	return family.Name
}

// familyURL is a nil-safe builder for a family's public browse URL.
func familyURL(family *domain.OntologyFamily) string {
	if family == nil {
		return ""
	}
	return "/ontologies/families/" + family.Slug
}
