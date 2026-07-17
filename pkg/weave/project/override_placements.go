package project

import (
	"fmt"

	"github.com/pletka-io/pletka/pkg/domain"
)

// placementKey identifies one collection placement within a model.
type placementKey struct {
	CategoryID   string
	CollectionID string
}

// reconcilePlacements diffs the posted editor draft against the model's
// existing collection placements and returns the upserts
// and deletes the save must apply. Rules:
//   - only collection-group items carry placements; the synthetic
//     direct-fields bucket never does;
//   - a posted placement (even all-defaults) upserts — the round-trip is
//     what the editor sent back;
//   - an existing placement whose (category, collection) group is absent
//     from the draft is deleted (group removed);
//   - validation: min_occurs >= 0 and, when max_occurs is set,
//     max_occurs >= min_occurs. Violations reject the whole save.
func reconcilePlacements(
	existing []domain.CollectionPlacement,
	categories []overrideEditorCategory,
	projectID, modelID string,
) (upserts []domain.CollectionPlacement, deletes []placementKey, err error) {
	posted := make(map[placementKey]struct{})

	for _, cat := range categories {
		for _, item := range cat.Items {
			if item.Widget != "collection-group" {
				continue
			}
			key := placementKey{CategoryID: cat.CategoryID, CollectionID: item.ID}
			posted[key] = struct{}{}
			if item.Placement == nil {
				continue
			}
			p := *item.Placement
			if p.MinOccurs < 0 {
				return nil, nil, fmt.Errorf("collection %s: min_occurs must be >= 0", item.ID)
			}
			if p.MaxOccurs != nil && *p.MaxOccurs < p.MinOccurs {
				return nil, nil, fmt.Errorf("collection %s: max_occurs must be >= min_occurs", item.ID)
			}
			p.ProjectID = projectID
			p.ModelID = modelID
			p.CategoryID = key.CategoryID
			p.CollectionID = key.CollectionID
			upserts = append(upserts, p)
		}
	}

	for _, ex := range existing {
		key := placementKey{CategoryID: ex.CategoryID, CollectionID: ex.CollectionID}
		if _, ok := posted[key]; !ok {
			deletes = append(deletes, key)
		}
	}
	return upserts, deletes, nil
}
