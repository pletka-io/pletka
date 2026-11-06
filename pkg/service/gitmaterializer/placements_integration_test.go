//go:build integration

package gitmaterializer_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/service/gitmaterializer"
)

// placedCollection picks one (model, category, collection) of project LA that
// an override file places, so a placement row for it is valid.
func placedCollection(t *testing.T, ctx context.Context) (modelID, categoryID, collectionID string) {
	t.Helper()
	err := testdb.Pool(t).QueryRow(ctx, `
		SELECT entity_id, category_id, part_of_collection_id
		FROM weave_field_overrides
		WHERE project_id = 'LA' AND entity_type = 'model' AND part_of_collection_id <> ''
		ORDER BY entity_id, part_of_collection_id LIMIT 1`).Scan(&modelID, &categoryID, &collectionID)
	if err != nil {
		t.Fatalf("pick placed collection: %v", err)
	}
	return
}

func upsertPlacement(t *testing.T, ctx context.Context, modelID, categoryID, collectionID string, max *int32) {
	t.Helper()
	q := sqlcgen.New(testdb.Pool(t))
	if _, err := q.WeaveUpsertCollectionPlacement(ctx, sqlcgen.WeaveUpsertCollectionPlacementParams{
		ProjectID: "LA", ModelID: modelID, CategoryID: categoryID, CollectionID: collectionID, MaxOccurs: max,
	}); err != nil {
		t.Fatalf("upsert placement: %v", err)
	}
	t.Cleanup(func() {
		_ = q.WeaveDeleteCollectionPlacement(context.Background(), sqlcgen.WeaveDeleteCollectionPlacementParams{
			ModelID: modelID, CategoryID: categoryID, CollectionID: collectionID,
		})
	})
}

func placementFile(root, modelID, categoryID, collectionID string) string {
	cat := categoryID
	if cat == "" {
		cat = "_"
	}
	return filepath.Join(root, "models", modelID, "placements", collectionID+"@"+cat+".yaml")
}

func TestInitProjectWritesPlacements(t *testing.T) {
	ctx := context.Background()
	modelID, categoryID, collectionID := placedCollection(t, ctx)
	one := int32(1)
	upsertPlacement(t, ctx, modelID, categoryID, collectionID, &one)

	dir := t.TempDir()
	if err := gitmaterializer.NewMaterializer(testdb.Pool(t), dir, nil).InitProject(ctx, "LA"); err != nil {
		t.Fatalf("InitProject: %v", err)
	}
	raw, err := os.ReadFile(placementFile(filepath.Join(dir, "LA"), modelID, categoryID, collectionID))
	if err != nil {
		t.Fatalf("placement file: %v", err)
	}
	if !strings.Contains(string(raw), "max_occurs: 1\n") || !strings.Contains(string(raw), "collection_id: "+collectionID+"\n") {
		t.Fatalf("placement file content:\n%s", raw)
	}
}

func TestScopedRewriteRemovesDeletedPlacements(t *testing.T) {
	ctx := context.Background()
	modelID, categoryID, collectionID := placedCollection(t, ctx)
	upsertPlacement(t, ctx, modelID, categoryID, collectionID, nil)

	dir := t.TempDir()
	m := gitmaterializer.NewMaterializer(testdb.Pool(t), dir, nil)
	if err := m.InitProject(ctx, "LA"); err != nil {
		t.Fatalf("InitProject: %v", err)
	}
	root := filepath.Join(dir, "LA")
	file := placementFile(root, modelID, categoryID, collectionID)
	if _, err := os.Stat(file); err != nil {
		t.Fatalf("placement file before delete: %v", err)
	}
	q := sqlcgen.New(testdb.Pool(t))
	if err := q.WeaveDeleteCollectionPlacement(ctx, sqlcgen.WeaveDeleteCollectionPlacementParams{
		ModelID: modelID, CategoryID: categoryID, CollectionID: collectionID,
	}); err != nil {
		t.Fatal(err)
	}
	if err := gitmaterializer.ExportWritePlacementsForModel(ctx, m, root, "LA", modelID); err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatalf("placement file still there after delete: %v", err)
	}
}
