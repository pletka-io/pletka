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

func TestOntologyBodyReaderHasOnlySealedCapability(t *testing.T) {
	checkOntologyCapability(t, "ontologybody", "read_ontology_body", "weave_ontology_body_seals")
}

func TestOntologyReleaseReaderHasOnlyPublishedCapability(t *testing.T) {
	checkOntologyCapability(t, "ontologyrelease", "read_ontology_release", "weave_ontology_releases")
}

func checkOntologyCapability(t *testing.T, pkg, queries, table string) {
	t.Helper()
	root := repoRoot(t)
	file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(root, "pkg/database/"+pkg+"/reader.go"), nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.StructType:
			for _, field := range node.Fields.List {
				switch ty := field.Type.(type) {
				case *ast.Ident:
					if ty.Name != "Reference" && ty.Name != "bool" {
						t.Errorf("reader retains %s", ty.Name)
					}
				case *ast.SelectorExpr:
					id, ok := ty.X.(*ast.Ident)
					if !ok || id.Name != "readsql" || ty.Sel.Name != "Querier" {
						t.Error("reader retains unrestricted capability")
					}
				default:
					t.Error("reader retains unrestricted capability")
				}
			}
		case *ast.CallExpr:
			if sel, ok := node.Fun.(*ast.SelectorExpr); ok && (sel.Sel.Name == "New" || sel.Sel.Name == "BeginTx") {
				t.Error("body reader constructs a database capability")
			}
		}
		return true
	})
	paths, err := filepath.Glob(filepath.Join(root, "pkg/database/queries/"+queries+"/*.sql"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("body query sources missing: %v", err)
	}
	tables := regexp.MustCompile(`(?i)\b(?:FROM|JOIN)\s+([a-z_]+)`)
	writes := regexp.MustCompile(`(?i)\b(?:INSERT|UPDATE|DELETE|TRUNCATE|CALL)\b`)
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		sql := string(data)
		if writes.MatchString(sql) {
			t.Errorf("%s contains a write", path)
		}
		for _, match := range tables.FindAllStringSubmatch(sql, -1) {
			if match[1] != table {
				t.Errorf("%s reads mutable table %s", path, match[1])
			}
		}
		if !strings.Contains(sql, "ontology_id = @ontology_id") || !strings.Contains(sql, "version_id = @version_id") {
			t.Errorf("%s is not bound to owner and version", path)
		}
	}
}
