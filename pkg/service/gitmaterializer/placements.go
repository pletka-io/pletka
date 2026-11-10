package gitmaterializer

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/jackc/pgx/v5"

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

// writeProjectPlacements writes every placement row of projectID whose model
// places the collection through a model override; any other row would be a
// file load-git rejects.
func (m *Materializer) writeProjectPlacements(ctx context.Context, workDir, projectID string) error {
	rows, err := m.queries.WeaveListPlacedCollectionPlacementsByProject(ctx, projectID)
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
// the database: cleared first, so deleted rows drop their files. Like
// writeProjectPlacements it writes only rows whose collection the model places.
func (m *Materializer) writePlacementsForModel(ctx context.Context, workDir, projectID, modelID string) error {
	dir := filepath.Join(workDir, filepath.FromSlash(path.Dir(placementFilePath(modelID, "", "x"))))
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("clear placements dir for model %s: %w", modelID, err)
	}
	rows, err := m.queries.WeaveListPlacedCollectionPlacementsByProject(ctx, projectID)
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

// canonicalPlacementDoc is the decoded form of a placement file.
type canonicalPlacementDoc struct {
	ModelID      string `json:"model_id"`
	CategoryID   string `json:"category_id"`
	CollectionID string `json:"collection_id"`
	IsRequired   bool   `json:"is_required"`
	MinOccurs    int    `json:"min_occurs"`
	MaxOccurs    *int   `json:"max_occurs"`
	IsHidden     bool   `json:"is_hidden"`
}

// placedCollectionsByModel returns, per model of the snapshot, the
// collections its override files place (part_of_collection_id).
func placedCollectionsByModel(snap *ProjectSnapshot) (map[string]map[string]bool, error) {
	out := map[string]map[string]bool{}
	for _, item := range snap.Entities.Models {
		out[item.EntityID] = map[string]bool{}
	}
	for _, item := range snap.Entities.ModelOverrides {
		doc, err := decodeCanonicalYAML[canonicalOverrideDoc](item.Payload)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", item.Path, err)
		}
		if doc.PartOfCollectionID == "" {
			continue
		}
		if out[item.OwnerID] == nil {
			out[item.OwnerID] = map[string]bool{}
		}
		out[item.OwnerID][doc.PartOfCollectionID] = true
	}
	return out, nil
}

// hydratePlacementsTx inserts the snapshot's placement files. The caller has
// cleared the project's rows in the same transaction.
func (m *Materializer) hydratePlacementsTx(ctx context.Context, q *sqlcgen.Queries, projectID string, snap *ProjectSnapshot) error {
	if len(snap.Entities.Placements) == 0 {
		return nil
	}
	placed, err := placedCollectionsByModel(snap)
	if err != nil {
		return err
	}
	for _, item := range snap.Entities.Placements {
		doc, err := decodeTreeFile[canonicalPlacementDoc](item.Path, item.Payload)
		if err != nil {
			return err
		}
		collectionID, categoryID, _ := parsePlacementFileName(path.Base(item.Path))
		switch {
		case doc.ModelID != item.OwnerID || doc.CollectionID != collectionID || doc.CategoryID != categoryID:
			return fmt.Errorf("%s: model_id, collection_id and category_id must match the file's place in the tree", item.Path)
		case doc.MinOccurs < 0:
			return fmt.Errorf("%s: min_occurs must be >= 0", item.Path)
		case doc.MinOccurs > math.MaxInt32:
			return fmt.Errorf("%s: min_occurs must be <= %d", item.Path, math.MaxInt32)
		case doc.MaxOccurs != nil && *doc.MaxOccurs < doc.MinOccurs:
			return fmt.Errorf("%s: max_occurs must be >= min_occurs", item.Path)
		case placed[doc.ModelID] == nil:
			return fmt.Errorf("%s: model %s is not in the tree", item.Path, doc.ModelID)
		case !placed[doc.ModelID][doc.CollectionID]:
			return fmt.Errorf("%s: model %s places no field of collection %s", item.Path, doc.ModelID, doc.CollectionID)
		}
		var max *int32
		if doc.MaxOccurs != nil {
			v := *doc.MaxOccurs // >= min_occurs >= 0, checked above
			if v < 0 || v > math.MaxInt32 {
				return fmt.Errorf("%s: max_occurs must be <= %d", item.Path, math.MaxInt32)
			}
			n := int32(v)
			max = &n
		}
		_, err = q.WeaveRestoreCollectionPlacement(ctx, sqlcgen.WeaveRestoreCollectionPlacementParams{
			ProjectID: projectID, ModelID: doc.ModelID, CategoryID: doc.CategoryID, CollectionID: doc.CollectionID,
			IsRequired: doc.IsRequired, MinOccurs: int32(doc.MinOccurs), MaxOccurs: max, IsHidden: doc.IsHidden,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%s: placement belongs to another project", item.Path)
		}
		if err != nil {
			return fmt.Errorf("%s: %w", item.Path, err)
		}
	}
	return nil
}
