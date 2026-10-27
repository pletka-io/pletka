//go:build integration

package ontology_test

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

	"github.com/go-chi/chi/v5"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/database/ontologyrelease"
	"github.com/pletka-io/pletka/pkg/weave/ontology"
)

// Opt in after building frontend assets. An explicitly requested browser run
// fails if prerequisites are missing; it never silently reports a skipped pass.
func TestOntologyEditorBrowser(t *testing.T) {
	if os.Getenv("PLETKA_EDITOR_BROWSER") != "1" {
		t.Skip("set PLETKA_EDITOR_BROWSER=1 after npm --prefix frontend run build")
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
	entry := manifest["src/islands/ontology-page.ts"].File
	css := manifest["src/global/global-css.css"].File
	if entry == "" {
		t.Fatal("ontology island missing from built assets")
	}
	pool, store, drafts, document, _ := draftFixture(t)
	if _, err := drafts.Start(t.Context(), draftOwner, document); err != nil {
		t.Fatal(err)
	}
	router := chi.NewRouter()
	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(auth.WithSnapshot(r.Context(), &auth.AuthSnapshot{IsSuperAdmin: true})))
		})
	})
	ontology.MountEditorAPI(router, &ontology.EditorHost{Drafts: drafts, Catalog: ontology.NewService(store, nil, nil), Releases: ontologyrelease.NewFactory(pool)})
	router.Handle("/dist/*", http.StripPrefix("/dist/", http.FileServer(http.Dir(dist))))
	router.Get("/admin/ontologies/EDITOR_ONTOLOGY/draft/page", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprintf(w, `<!doctype html><html lang="en"><head><meta charset="utf-8"><link rel="stylesheet" href="/dist/%s"></head><body class="bg-gray-50 p-8"><main data-island="ontology-page" data-prop-schema-url="%s/page-schema"></main><script type="module" src="/dist/%s"></script></body></html>`, css, editorURL, entry)
	})
	server := httptest.NewServer(router)
	defer server.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "node", filepath.Join(root, "test/browser/ontology-editor.mjs"), server.URL, chromium, os.Getenv("PLETKA_EDITOR_SCREENSHOT"))
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("browser check: %v\n%s", err, output)
	}
	t.Log(string(output))
}
