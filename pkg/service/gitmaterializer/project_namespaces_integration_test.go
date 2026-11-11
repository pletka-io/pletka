//go:build integration

package gitmaterializer_test

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/service/gitmaterializer"
)

// The tree's prefix binding decides on load, survives the next init-git, and
// goes away when the tree drops it.
func TestLoadGitProjectYAMLDecidesNamespaceBindings(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Pool(t)
	m := gitmaterializer.NewMaterializer(pool, t.TempDir(), nil)
	export := func() string {
		t.Helper()
		dir := t.TempDir()
		if err := gitmaterializer.NewMaterializer(pool, dir, nil).InitProject(ctx, "LA"); err != nil {
			t.Fatalf("InitProject: %v", err)
		}
		raw, err := os.ReadFile(filepath.Join(dir, "LA", "project.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}
	load := func(projectYAML string) {
		t.Helper()
		dir := t.TempDir()
		if err := gitmaterializer.NewMaterializer(pool, dir, nil).InitProject(ctx, "LA"); err != nil {
			t.Fatalf("InitProject: %v", err)
		}
		root := filepath.Join(dir, "LA")
		if err := os.WriteFile(filepath.Join(root, "project.yaml"), []byte(projectYAML), 0o644); err != nil {
			t.Fatal(err)
		}
		snap, err := gitmaterializer.LoadProjectSnapshot(root)
		if err != nil {
			t.Fatalf("LoadProjectSnapshot: %v", err)
		}
		if err := m.HydrateProjectShell(ctx, gitmaterializer.BuildRestorePlan(snap)); err != nil {
			t.Fatalf("HydrateProjectShell: %v", err)
		}
	}
	projectRows := func() int {
		t.Helper()
		var n int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM weave_namespace_bindings WHERE project_id = 'LA' AND namespace = 'https://tree.example/crm/'`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	original := export()
	if strings.Contains(original, "source:") {
		t.Fatalf("project.yaml still writes source:\n%s", original)
	}
	crm := regexp.MustCompile(`(?m)^(\s+- namespace: )http://www\.cidoc-crm\.org/cidoc-crm/(\n\s+prefix: crm\n)`)
	if !crm.MatchString(original) {
		t.Fatalf("no crm binding in project.yaml:\n%s", original)
	}
	chosen := crm.ReplaceAllString(original, "${1}https://tree.example/crm/${2}")

	// A global row with the same pair, outweighed by the instance's crm
	// binding: the project still owns its own row (per-scope uniqueness).
	if _, err := pool.Exec(ctx, `INSERT INTO weave_namespace_bindings (id, project_id, prefix, namespace, weight, source, created_at, updated_at)
		VALUES ('nb-test-global-tree-crm', NULL, 'crm', 'https://tree.example/crm/', 0, 'system', now(), now())`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM weave_namespace_bindings WHERE id = 'nb-test-global-tree-crm'`)
	})

	load(chosen)
	if n := projectRows(); n != 1 {
		t.Fatalf("%d project bindings for the tree's crm namespace, want 1", n)
	}
	if got := export(); got != chosen {
		t.Fatalf("init-git after load-git does not reproduce project.yaml:\n%s", got)
	}

	// A project inheriting from LA resolves crm with LA's choice.
	var child string
	if err := pool.QueryRow(ctx, `SELECT project_id FROM weave_project_inheritance WHERE parent_project_id = 'LA' ORDER BY project_id LIMIT 1`).Scan(&child); err != nil {
		t.Fatalf("no project inherits from LA in the fixtures: %v", err)
	}
	childDir := t.TempDir()
	if err := gitmaterializer.NewMaterializer(pool, childDir, nil).InitProject(ctx, child); err != nil {
		t.Fatalf("InitProject %s: %v", child, err)
	}
	childYAML, err := os.ReadFile(filepath.Join(childDir, child, "project.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(childYAML), "- namespace: https://tree.example/crm/\n      prefix: crm\n") {
		t.Fatalf("%s does not inherit LA's crm binding:\n%s", child, childYAML)
	}

	load(original)
	if n := projectRows(); n != 0 {
		t.Fatalf("%d project bindings left after the tree dropped its choice", n)
	}
	if got := export(); got != original {
		t.Fatalf("project.yaml not back to the instance's bindings:\n%s", got)
	}
}
