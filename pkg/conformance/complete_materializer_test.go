package conformance

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestCompleteReleaseWriterCannotQueryOrUseLegacyProjection(t *testing.T) {
	path := filepath.Join(repoRoot(t), "pkg/service/gitmaterializer/complete_materialize.go")
	f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			selector, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			switch selector.Sel.Name {
			case "queries", "Query", "QueryRow", "Exec", "BeginTx", "writeProjectTreeVersion", "vendorDependenciesForProject", "loadReleasesIndex":
				t.Errorf("complete release writer regained independent content capability: %s", selector.Sel.Name)
			case "writeLegacyReleaseSnapshot":
				if fn.Name.Name != "writeVerificationTree" {
					t.Error("legacy projection escaped stored-format verification")
				}
			}
			return true
		})
	}
}

func TestReleaseCatalogQueriesReadOnlyBoundMetadata(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join(repoRoot(t), "pkg/database/queries/read_catalog/*.sql"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("missing catalog query inventory: %v", err)
	}
	tables := regexp.MustCompile(`(?i)\b(?:FROM|JOIN)\s+([a-z_]+)`)
	writes := regexp.MustCompile(`(?i)\b(?:INSERT|UPDATE|DELETE|MERGE|TRUNCATE|ALTER|CREATE|DROP)\b`)
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		sql := string(data)
		if writes.MatchString(sql) || !strings.Contains(sql, "r.project_id=sqlc.arg(project_id)::text") {
			t.Error("catalog query writes or loses bound project identity")
		}
		for _, match := range tables.FindAllStringSubmatch(sql, -1) {
			if match[1] != "weave_releases" && match[1] != "r" {
				t.Errorf("catalog gained content lookup: %s", match[1])
			}
		}
	}
}
