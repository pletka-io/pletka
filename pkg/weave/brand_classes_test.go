package weave

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRuntimeAssetsDoNotUseRetiredBrandClasses(t *testing.T) {
	root := findRepoRoot(t)
	retired := []string{
		"zellij-" + "primary",
		"zellij-" + "secondary",
		"zellij-" + "primary-dark",
		"zellij-" + "gradient",
	}
	targets := []string{
		"pkg/weave",
		"pkg/assets/i18n",
		"pkg/assets/static/dist",
		"frontend/src",
	}

	var hits []string
	for _, target := range targets {
		base := filepath.Join(root, target)
		if err := filepath.WalkDir(base, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				if entry.Name() == "node_modules" || entry.Name() == "dist" {
					return filepath.SkipDir
				}
				return nil
			}
			if !isBrandClassScanFile(path) {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			text := string(data)
			for _, token := range retired {
				if strings.Contains(text, token) {
					rel, _ := filepath.Rel(root, path)
					hits = append(hits, rel+": "+token)
				}
			}
			return nil
		}); err != nil {
			t.Fatalf("scan %s: %v", target, err)
		}
	}
	if len(hits) > 0 {
		t.Fatalf("runtime assets use retired brand classes:\n- %s", strings.Join(hits, "\n- "))
	}
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(wd, "go.mod")); err == nil {
			return wd
		}
		parent := filepath.Dir(wd)
		if parent == wd {
			t.Fatal("go.mod not found")
		}
		wd = parent
	}
}

func isBrandClassScanFile(path string) bool {
	switch filepath.Ext(path) {
	case ".go", ".gohtml", ".md", ".json", ".svelte", ".ts", ".js", ".css":
		return true
	default:
		return false
	}
}
