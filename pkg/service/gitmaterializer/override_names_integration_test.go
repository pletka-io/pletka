//go:build integration

package gitmaterializer_test

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/service/gitmaterializer"
)

// overrideFiles maps every override file under root to its content.
func overrideFiles(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Base(filepath.Dir(p)) != "overrides" {
			return err
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		out[rel] = string(raw)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// A load-git inserts the override rows again with new ids; the next init-git
// must still write the same file names, not names that follow the row ids.
func TestLoadGitKeepsOverrideFileNames(t *testing.T) {
	ctx := context.Background()
	if err := roundTripLA(t, ctx, nil); err != nil {
		t.Fatalf("hydrate: %v", err)
	}
	dir := t.TempDir()
	m := gitmaterializer.NewMaterializer(testdb.Pool(t), dir, nil)
	if err := m.InitProject(ctx, "LA"); err != nil {
		t.Fatalf("InitProject: %v", err)
	}
	before := overrideFiles(t, filepath.Join(dir, "LA"))
	if len(before) == 0 {
		t.Fatal("LA has no override files")
	}
	rowID := regexp.MustCompile(`@[0-9]+\.yaml$`)
	for rel := range before {
		if rowID.MatchString(rel) {
			t.Fatalf("%s is named after a row id", rel)
		}
	}

	if err := roundTripLA(t, ctx, nil); err != nil {
		t.Fatalf("hydrate: %v", err)
	}
	dir2 := t.TempDir()
	if err := gitmaterializer.NewMaterializer(testdb.Pool(t), dir2, nil).InitProject(ctx, "LA"); err != nil {
		t.Fatalf("InitProject after load: %v", err)
	}
	after := overrideFiles(t, filepath.Join(dir2, "LA"))
	if len(after) != len(before) {
		t.Fatalf("%d override files after load-git, %d before", len(after), len(before))
	}
	for rel, body := range before {
		if after[rel] != body {
			t.Fatalf("%s changed or renamed across load-git", rel)
		}
	}
}
