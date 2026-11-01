//go:build integration

package projectpage

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/formschema"
)

// Uses the catalog test's actual scoped routes and built shared workspace.
func runExampleWorkspaceBrowser(t *testing.T, api http.Handler) {
	t.Helper()
	if os.Getenv("PLETKA_EXAMPLE_BROWSER") != "1" {
		return
	}
	chromium, err := exec.LookPath("chromium")
	if err != nil {
		t.Fatal(err)
	}
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	dist := filepath.Join(root, "pkg/assets/static/dist")
	data, err := os.ReadFile(filepath.Join(dist, ".vite/manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]struct {
		File string `json:"file"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	entry, css := manifest["src/islands/entity-list.ts"].File, manifest["src/global/global-css.css"].File
	if entry == "" {
		t.Fatal("build frontend assets first")
	}
	files := http.StripPrefix("/dist/", http.FileServer(http.Dir(dist)))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner := false
		if c, err := r.Cookie("test-owner"); err == nil && c.Value == "1" {
			owner = true
		}
		if owner {
			r = r.WithContext(auth.WithSnapshot(r.Context(), &auth.AuthSnapshot{IsSuperAdmin: true}))
		}
		switch {
		case r.URL.Path == "/__example-ui":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = fmt.Fprintf(w, `<!doctype html><html lang="en"><head><meta charset="utf-8"><link rel="stylesheet" href="/dist/%s"></head><body><main data-island="entity-list" data-prop-schema-url="/__example-schema" data-prop-initial-item-id="CATP.E"></main><script type="module" src="/dist/%s"></script></body></html>`, css, entry)
		case r.URL.Path == "/__example-schema":
			schema := formschema.BuildExampleEntityListSchema("CATP", "en", nil, "model", "CATS.M")
			if !owner {
				schema.BindReadVersion("2.0.0")
				schema.StripEditActions()
			}
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(schema); err != nil {
				t.Error(err)
			}
		case len(r.URL.Path) > 6 && r.URL.Path[:6] == "/dist/":
			files.ServeHTTP(w, r)
		default:
			api.ServeHTTP(w, r)
		}
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "node", filepath.Join(root, "test/browser/example-workspace.mjs"), server.URL, chromium)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("browser: %v\n%s", err, output)
	}
	t.Log(string(output))
}
