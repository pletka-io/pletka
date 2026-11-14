package canonical

import (
	"slices"
	"strings"

	"github.com/pletka-io/pletka/pkg/domain"
)

// TreeConcept is one concepts.yaml item.
type TreeConcept struct {
	URI        string
	Label      domain.Translations
	ScopeNote  domain.Translations
	Broader    []string
	TopConcept bool
	ExternalID string
	ExactMatch []string
}

// Vocabulary serializes a project vocabulary to vocabularies/<V>/vocabulary.yaml.
// Excluded: id, project_id, connector_type, config, timestamps (database-local).
func Vocabulary(v *domain.Vocabulary) ([]byte, error) {
	return Encode(map[string]any{
		keySemanticID:  v.SemanticID,
		keySystemName:  v.SystemName,
		keyUIName:      translationsToMap(v.UIName),
		keyDescription: translationsToMap(v.Description),
		keyStatus:      v.Status,
		"base_uri":     v.BaseURI,
	})
}

// VocabularyConcepts serializes concepts.yaml, sorted by URI. Empty optional
// keys are left out so a concept without a hierarchy stays one short block.
func VocabularyConcepts(cs []TreeConcept) ([]byte, error) {
	sorted := slices.Clone(cs)
	slices.SortFunc(sorted, func(a, b TreeConcept) int { return strings.Compare(a.URI, b.URI) })
	items := make([]map[string]any, 0, len(sorted))
	for _, c := range sorted {
		m := map[string]any{"uri": c.URI, "label": translationsToMap(c.Label)}
		if len(c.ScopeNote) > 0 {
			m["scope_note"] = translationsToMap(c.ScopeNote)
		}
		if len(c.Broader) > 0 {
			m["broader"] = c.Broader
		}
		if c.TopConcept {
			m["top_concept"] = true
		}
		if c.ExternalID != "" {
			m["external_id"] = c.ExternalID
		}
		if len(c.ExactMatch) > 0 {
			m["exact_match"] = c.ExactMatch
		}
		items = append(items, m)
	}
	return Encode(items)
}

// ConceptListEntries serializes concept-lists/<CL>/entries.yaml in list order.
func ConceptListEntries(uris []string) ([]byte, error) { return Encode(uris) }
