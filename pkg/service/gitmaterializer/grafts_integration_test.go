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

func mustWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// firstModel returns one model id of the tree at root.
func firstModel(t *testing.T, root string) string {
	t.Helper()
	ents, err := os.ReadDir(filepath.Join(root, "models"))
	if err != nil || len(ents) == 0 {
		t.Fatalf("no models in tree: %v", err)
	}
	return ents[0].Name()
}

// cleanGraftRows removes LA's graft overrides and generator files after the
// test, so other tests see LA as the fixture left it.
func cleanGraftRows(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		q := sqlcgen.New(testdb.Pool(t))
		_ = q.WeaveDeleteGraftOverridesByProject(context.Background(), "LA")
		_ = q.WeaveDeleteGeneratorFilesByProject(context.Background(), "LA")
	})
}

func TestLoadGitStoresGraftsAndGeneratorFiles(t *testing.T) {
	ctx := context.Background()
	cleanGraftRows(t)
	var model string
	if err := roundTripLA(t, ctx, func(root string) {
		model = firstModel(t, root)
		mustWrite(t, filepath.Join(root, "models", model, "grafts.yaml"), "LAF.1/LAF.2: {max_occurs: 1, display_name: {en: Two}}\nLAF.1/LAF.3: {is_hidden: true}\nLAF.1/LAF.4: {max_occurs: null}\n")
		mustWrite(t, filepath.Join(root, "generators", "arches", "x", "aliases.yaml"), "LAF.1: x\n")
	}); err != nil {
		t.Fatalf("load-git: %v", err)
	}
	q := sqlcgen.New(testdb.Pool(t))
	rows, err := q.WeaveListGraftOverridesByModel(ctx, sqlcgen.WeaveListGraftOverridesByModelParams{ProjectID: "LA", ModelID: model})
	if err != nil || len(rows) != 3 || rows[0].Address != "LAF.1/LAF.2" || rows[0].MaxOccurs == nil || *rows[0].MaxOccurs != 1 || !rows[0].MaxOccursSet || !rows[1].IsHidden || rows[1].MaxOccursSet {
		t.Fatalf("graft rows %+v err %v", rows, err)
	}
	// max_occurs: null overrides the stub's max to unbounded; it is not "unset".
	if rows[2].MaxOccurs != nil || !rows[2].MaxOccursSet {
		t.Fatalf("null max_occurs row %+v", rows[2])
	}
	if !strings.Contains(string(rows[0].DisplayName), `"Two"`) {
		t.Fatalf("display name %s", rows[0].DisplayName)
	}
	files, err := q.WeaveListGeneratorFilesByProject(ctx, "LA")
	if err != nil || len(files) != 1 || files[0].Path != "generators/arches/x/aliases.yaml" || string(files[0].Content) != "LAF.1: x\n" {
		t.Fatalf("generator files %+v err %v", files, err)
	}
}

func TestLoadGitReplacesGrafts(t *testing.T) {
	ctx := context.Background()
	cleanGraftRows(t)
	if err := roundTripLA(t, ctx, func(root string) {
		mustWrite(t, filepath.Join(root, "models", firstModel(t, root), "grafts.yaml"), "LAF.1/LAF.2: {is_hidden: true}\n")
	}); err != nil {
		t.Fatal(err)
	}
	// init-git does not write grafts back until Task 5; either way the second
	// tree has no grafts.yaml, so the rows must go.
	if err := roundTripLA(t, ctx, func(root string) {
		_ = os.Remove(filepath.Join(root, "models", firstModel(t, root), "grafts.yaml"))
	}); err != nil {
		t.Fatal(err)
	}
	rows, _ := sqlcgen.New(testdb.Pool(t)).WeaveListGraftOverridesByProject(ctx, "LA")
	if len(rows) != 0 {
		t.Fatalf("stale graft rows after reload: %+v", rows)
	}
}

func TestLoadGitRejectsGraftsWithoutModel(t *testing.T) {
	ctx := context.Background()
	cleanGraftRows(t)
	err := roundTripLA(t, ctx, func(root string) {
		mustWrite(t, filepath.Join(root, "models", "LAM.9999", "grafts.yaml"), "LAF.1/LAF.2: {is_hidden: true}\n")
	})
	if err == nil || !strings.Contains(err.Error(), "models/LAM.9999/grafts.yaml") {
		t.Fatalf("want an error naming the file, got %v", err)
	}
}

func TestInitGitWritesGraftsAndGeneratorFilesBack(t *testing.T) {
	ctx := context.Background()
	cleanGraftRows(t)
	grafts := "LAF.1/LAF.2:\n  display_name:\n    en: Two\n  max_occurs: 1\nLAF.1/LAF.3:\n  is_hidden: true\nLAF.1/LAF.4:\n  max_occurs: null\n"
	readme := "# not yaml\n\tkept as is\n"
	var model string
	if err := roundTripLA(t, ctx, func(root string) {
		model = firstModel(t, root)
		mustWrite(t, filepath.Join(root, "models", model, "grafts.yaml"), grafts)
		mustWrite(t, filepath.Join(root, "generators", "arches", "README.md"), readme)
	}); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := gitmaterializer.NewMaterializer(testdb.Pool(t), dir, nil).InitProject(ctx, "LA"); err != nil {
		t.Fatalf("InitProject: %v", err)
	}
	root2 := filepath.Join(dir, "LA")
	got, err := os.ReadFile(filepath.Join(root2, "models", model, "grafts.yaml"))
	if err != nil || string(got) != grafts {
		t.Fatalf("grafts.yaml written back:\n%s\nerr %v\nwant:\n%s", got, err, grafts)
	}
	got, err = os.ReadFile(filepath.Join(root2, "generators", "arches", "README.md"))
	if err != nil || string(got) != readme {
		t.Fatalf("generator file written back: %q err %v", got, err)
	}
}
