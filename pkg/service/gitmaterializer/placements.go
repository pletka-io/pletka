package gitmaterializer

import (
	"path"
	"strings"

	"github.com/pletka-io/pletka/pkg/domain"
)

const entityTypeCollectionPlacement = "collection_placement"

// uncategorizedPlacementToken stands for an empty category_id in a
// placement file name.
const uncategorizedPlacementToken = "_"

// placementFilePath is the tree path of one collection placement:
// models/<model>/placements/<collection>@<category|_>.yaml.
func placementFilePath(modelID, categoryID, collectionID string) string {
	modelDir := path.Dir(domain.FilePath(domain.PathSpec{EntityType: entityTypeModel, EntityID: modelID}))
	cat := categoryID
	if cat == "" {
		cat = uncategorizedPlacementToken
	}
	return path.Join(modelDir, "placements", collectionID+"@"+cat+".yaml")
}

// parsePlacementFileName splits "<collection>@<category|_>.yaml".
func parsePlacementFileName(name string) (collectionID, categoryID string, ok bool) {
	if !strings.HasSuffix(name, ".yaml") {
		return "", "", false
	}
	coll, cat, found := strings.Cut(strings.TrimSuffix(name, ".yaml"), "@")
	if !found || strings.TrimSpace(coll) == "" || strings.TrimSpace(cat) == "" {
		return "", "", false
	}
	if cat == uncategorizedPlacementToken {
		cat = ""
	}
	return coll, cat, true
}
