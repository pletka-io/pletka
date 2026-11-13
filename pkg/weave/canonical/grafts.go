package canonical

import "github.com/pletka-io/pletka/pkg/domain"

// Grafts serializes one model's graft overrides as grafts.yaml: a mapping
// keyed by address, with only the keys each override sets.
func Grafts(overrides []domain.GraftOverride) ([]byte, error) {
	doc := make(map[string]any, len(overrides))
	for _, g := range overrides {
		entry := map[string]any{}
		if g.IsHidden {
			entry["is_hidden"] = true
		}
		if g.MaxOccursSet {
			entry["max_occurs"] = nilableInt(g.MaxOccurs)
		}
		if len(g.DisplayName) > 0 {
			entry["display_name"] = translationsToMap(g.DisplayName)
		}
		doc[g.Address] = entry
	}
	return Encode(doc)
}
