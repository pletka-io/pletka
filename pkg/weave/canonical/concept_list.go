package canonical

import "github.com/pletka-io/pletka/pkg/domain"

// Shared entity keys of the canonical files. The older encoders spell them
// inline; new encoders use these so the spelling has one home.
const (
	keySemanticID  = "semantic_id"
	keySystemName  = "system_name"
	keyUIName      = "ui_name"
	keyDescription = "description"
	keyStatus      = "status"
	keyDeprecated  = "deprecated"
)

// ConceptList serializes a domain.ConceptList to canonical YAML for the
// project tree. Entries and broader edges are not part of the tree yet; the
// file carries the list itself. Excluded: id, project_id, timestamps,
// list_type and vocabulary_id (database-local references).
func ConceptList(l *domain.ConceptList) ([]byte, error) {
	data := map[string]any{
		keySemanticID:  l.SemanticID,
		keySystemName:  l.SystemName,
		keyUIName:      translationsToMap(l.UIName),
		keyDescription: translationsToMap(l.Description),
		keyStatus:      l.Status,
		keyDeprecated:  l.Deprecated,
		"is_closed":    l.IsClosed,
	}
	return Encode(data)
}
