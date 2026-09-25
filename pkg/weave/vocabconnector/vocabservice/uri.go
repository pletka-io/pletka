// Package vocabservice speaks the Pletka vocabulary-service contract: a JSON
// API serving one or more vocabularies. See
// docs-oss/reference/vocabulary-service-contract.md. kakugo
// (https://vocab.pletka.io) is its first implementation.
package vocabservice

import (
	"net/url"
	"strings"

	"github.com/pletka-io/pletka/pkg/domain"
)

// NormalizeURI stores Getty concepts under a canonical https identifier and
// rewrites the human "page" form to the data form.
//
// Copied deliberately from pkg/weave/vocabconnector/aat/uri.go rather than
// shared: this package exists so external vocabularies can move off the aat
// connector, and that connector is explicitly out of scope for this work.
func NormalizeURI(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return value
	}
	if !strings.EqualFold(parsed.Host, "vocab.getty.edu") {
		return parsed.String()
	}
	switch {
	case strings.HasPrefix(parsed.Path, "/page/aat/"):
		parsed.Scheme = "https"
		parsed.Path = "/aat/" + strings.TrimPrefix(parsed.Path, "/page/aat/")
	case strings.HasPrefix(parsed.Path, "/aat/"):
		parsed.Scheme = "https"
	}
	return parsed.String()
}

// SplitParentString turns the service's comma-joined ancestor chain into its
// parts, nearest ancestor first. Copied from the aat connector, see above.
// Deliberately differs from the aat original on input of only separators
// (e.g., ",," returns nil, not a slice with the trimmed string): nil is
// better than rendering bare separators as ancestor labels in the picker.
func SplitParentString(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// ParentStringRefs is SplitParentString as labeled refs. The service gives no
// identifier for an ancestor beyond the immediate broader concept, so every
// ref carries a label only; Search fills item 0's URI and ID from the hit's
// broader concept. Copied from the aat connector, see above.
func ParentStringRefs(value, lang string) []domain.VocabularyEntryRef {
	parts := SplitParentString(value)
	if len(parts) == 0 {
		return nil
	}
	if strings.TrimSpace(lang) == "" {
		lang = "en"
	}
	out := make([]domain.VocabularyEntryRef, 0, len(parts))
	for _, part := range parts {
		out = append(out, domain.VocabularyEntryRef{Label: domain.Translations{lang: part}})
	}
	return out
}
