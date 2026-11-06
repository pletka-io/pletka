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

// roundTripLA materializes LA, then hydrates the overrides (and placements)
// back from the tree into the same database.
func roundTripLA(t *testing.T, ctx context.Context, mutate func(root string)) error {
	t.Helper()
	dir := t.TempDir()
	m := gitmaterializer.NewMaterializer(testdb.Pool(t), dir, nil)
	if err := m.InitProject(ctx, "LA"); err != nil {
		t.Fatalf("InitProject: %v", err)
	}
	root := filepath.Join(dir, "LA")
	if mutate != nil {
		mutate(root)
	}
	snap, err := gitmaterializer.LoadProjectSnapshot(root)
	if err != nil {
		t.Fatalf("LoadProjectSnapshot: %v", err)
	}
	return m.HydrateProjectOverrides(ctx, gitmaterializer.BuildRestorePlan(snap))
}

func placementRow(t *testing.T, ctx context.Context, modelID, categoryID, collectionID string) (found bool, max *int32) {
	t.Helper()
	r, err := sqlcgen.New(testdb.Pool(t)).WeaveGetCollectionPlacement(ctx, sqlcgen.WeaveGetCollectionPlacementParams{
		ModelID: modelID, CategoryID: categoryID, CollectionID: collectionID,
	})
	if err != nil {
		return false, nil
	}
	return true, r.MaxOccurs
}

// upsertFullPlacement seeds a placement with non-default values.
func upsertFullPlacement(t *testing.T, ctx context.Context, modelID, categoryID, collectionID string) {
	t.Helper()
	q := sqlcgen.New(testdb.Pool(t))
	one := int32(1)
	if _, err := q.WeaveUpsertCollectionPlacement(ctx, sqlcgen.WeaveUpsertCollectionPlacementParams{
		ProjectID: "LA", ModelID: modelID, CategoryID: categoryID, CollectionID: collectionID,
		IsRequired: true, MinOccurs: 1, MaxOccurs: &one, IsHidden: true,
	}); err != nil {
		t.Fatalf("upsert placement: %v", err)
	}
	t.Cleanup(func() {
		_ = q.WeaveDeleteCollectionPlacement(context.Background(), sqlcgen.WeaveDeleteCollectionPlacementParams{
			ModelID: modelID, CategoryID: categoryID, CollectionID: collectionID,
		})
	})
}

// assertFullPlacementRoundTrip seeds a non-default placement, deletes its row
// after materialization, hydrates, and expects every value back from the file.
func assertFullPlacementRoundTrip(t *testing.T, categoryID string, wantSuffix string) {
	t.Helper()
	ctx := context.Background()
	modelID, _, collectionID := placedCollection(t, ctx)
	upsertFullPlacement(t, ctx, modelID, categoryID, collectionID)
	err := roundTripLA(t, ctx, func(root string) {
		if _, err := os.Stat(placementFile(root, modelID, categoryID, collectionID)); err != nil {
			t.Fatalf("placement file missing: %v", err)
		}
		if wantSuffix != "" && !strings.HasSuffix(placementFile(root, modelID, categoryID, collectionID), wantSuffix) {
			t.Fatalf("file name does not end in %s", wantSuffix)
		}
		if err := sqlcgen.New(testdb.Pool(t)).WeaveDeleteCollectionPlacement(ctx, sqlcgen.WeaveDeleteCollectionPlacementParams{
			ModelID: modelID, CategoryID: categoryID, CollectionID: collectionID,
		}); err != nil {
			t.Fatalf("delete row: %v", err)
		}
	})
	if err != nil {
		t.Fatalf("hydrate: %v", err)
	}
	r, err := sqlcgen.New(testdb.Pool(t)).WeaveGetCollectionPlacement(ctx, sqlcgen.WeaveGetCollectionPlacementParams{
		ModelID: modelID, CategoryID: categoryID, CollectionID: collectionID,
	})
	if err != nil {
		t.Fatalf("placement not restored from the file: %v", err)
	}
	if r.CategoryID != categoryID || !r.IsRequired || r.MinOccurs != 1 || r.MaxOccurs == nil || *r.MaxOccurs != 1 || !r.IsHidden {
		t.Fatalf("restored placement = %+v", r)
	}
}

func TestLoadGitRoundTripsPlacements(t *testing.T) {
	_, categoryID, _ := placedCollection(t, context.Background())
	assertFullPlacementRoundTrip(t, categoryID, "")
}

func TestLoadGitRoundTripsUncategorizedPlacement(t *testing.T) {
	assertFullPlacementRoundTrip(t, "", "@_.yaml")
}

func TestLoadGitClearsPlacementsMissingFromTree(t *testing.T) {
	ctx := context.Background()
	modelID, categoryID, collectionID := placedCollection(t, ctx)
	upsertPlacement(t, ctx, modelID, categoryID, collectionID, nil)
	err := roundTripLA(t, ctx, func(root string) {
		_ = os.RemoveAll(filepath.Join(root, "models", modelID, "placements"))
	})
	if err != nil {
		t.Fatalf("hydrate tree without placements/: %v", err)
	}
	if found, _ := placementRow(t, ctx, modelID, categoryID, collectionID); found {
		t.Fatal("placement survived a tree without it")
	}
}

func TestLoadGitRejectsBadPlacements(t *testing.T) {
	ctx := context.Background()
	modelID, categoryID, collectionID := placedCollection(t, ctx)
	upsertPlacement(t, ctx, modelID, categoryID, collectionID, nil)
	var before int
	_ = testdb.Pool(t).QueryRow(ctx, `SELECT count(*) FROM weave_field_overrides WHERE project_id='LA'`).Scan(&before)

	reasons := map[string]string{"range": "max_occurs must be", "orphan": "places no field"}
	cases := map[string]string{
		"range":  "category_id: " + categoryID + "\ncollection_id: " + collectionID + "\nis_hidden: false\nis_required: true\nmax_occurs: 0\nmin_occurs: 1\nmodel_id: " + modelID + "\n",
		"orphan": "category_id: " + categoryID + "\ncollection_id: NOPE.1\nis_hidden: false\nis_required: false\nmax_occurs: null\nmin_occurs: 0\nmodel_id: " + modelID + "\n",
	}
	for name, body := range cases {
		var rel string
		err := roundTripLA(t, ctx, func(root string) {
			coll := collectionID
			if name == "orphan" {
				coll = "NOPE.1"
			}
			p := placementFile(root, modelID, categoryID, coll)
			rel, _ = filepath.Rel(root, p)
			_ = os.MkdirAll(filepath.Dir(p), 0o755)
			_ = os.WriteFile(p, []byte(body), 0o644)
		})
		if err == nil || !strings.Contains(err.Error(), filepath.ToSlash(rel)) || !strings.Contains(err.Error(), reasons[name]) {
			t.Fatalf("%s: want error naming %s with %q, got %v", name, rel, reasons[name], err)
		}
		if found, _ := placementRow(t, ctx, modelID, categoryID, collectionID); !found {
			t.Fatalf("%s: pre-existing placement lost by a failed hydration (clear was committed)", name)
		}
		var after int
		_ = testdb.Pool(t).QueryRow(ctx, `SELECT count(*) FROM weave_field_overrides WHERE project_id='LA'`).Scan(&after)
		if after != before {
			t.Fatalf("%s: overrides changed by a failed hydration: %d -> %d", name, before, after)
		}
	}
}
