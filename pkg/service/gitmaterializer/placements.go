package gitmaterializer

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/canonical"
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

// placementFromRow converts a sqlc row to the domain type.
func placementFromRow(r sqlcgen.WeaveCollectionPlacement) domain.CollectionPlacement {
	p := domain.CollectionPlacement{
		ID: r.ID, ProjectID: r.ProjectID, ModelID: r.ModelID, CategoryID: r.CategoryID,
		CollectionID: r.CollectionID, IsRequired: r.IsRequired, MinOccurs: int(r.MinOccurs), IsHidden: r.IsHidden,
	}
	if r.MaxOccurs != nil {
		n := int(*r.MaxOccurs)
		p.MaxOccurs = &n
	}
	return p
}

func writePlacementFile(workDir string, p domain.CollectionPlacement) error {
	payload, err := canonical.Placement(&p)
	if err != nil {
		return fmt.Errorf("encode placement %s/%s/%s: %w", p.ModelID, p.CategoryID, p.CollectionID, err)
	}
	return writeEntityFile(workDir, placementFilePath(p.ModelID, p.CategoryID, p.CollectionID), payload)
}

// writeProjectPlacements writes every placement row of projectID.
func (m *Materializer) writeProjectPlacements(ctx context.Context, workDir, projectID string) error {
	rows, err := m.queries.WeaveListCollectionPlacementsByProject(ctx, projectID)
	if err != nil {
		return fmt.Errorf("list placements for project %s: %w", projectID, err)
	}
	for _, r := range rows {
		if err := writePlacementFile(workDir, placementFromRow(r)); err != nil {
			return err
		}
	}
	return nil
}

// writePlacementsForModel regenerates one model's placements/ directory from
// the database: cleared first, so deleted rows drop their files.
func (m *Materializer) writePlacementsForModel(ctx context.Context, workDir, projectID, modelID string) error {
	dir := filepath.Join(workDir, filepath.FromSlash(path.Dir(placementFilePath(modelID, "", "x"))))
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("clear placements dir for model %s: %w", modelID, err)
	}
	rows, err := m.queries.WeaveListCollectionPlacementsByProject(ctx, projectID)
	if err != nil {
		return fmt.Errorf("list placements for project %s: %w", projectID, err)
	}
	for _, r := range rows {
		if r.ModelID != modelID {
			continue
		}
		if err := writePlacementFile(workDir, placementFromRow(r)); err != nil {
			return err
		}
	}
	return nil
}
