package conformance

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

func TestOntologyBodyReaderHasOnlySealedCapability(t *testing.T) {
	checkOntologyCapability(t, "ontologybody/reader.go", "read_ontology_body", []string{"weave_ontology_body_seals"}, []string{"ontology_id = @ontology_id", "version_id = @version_id"})
}

func TestOntologyReleaseReaderHasOnlyPublishedCapability(t *testing.T) {
	checkOntologyCapability(t, "ontologyrelease/reader.go", "read_ontology_release", []string{"weave_ontology_releases"}, []string{"ontology_id = @ontology_id", "version_id = @version_id"})
}

func TestProjectOntologyReaderHasOnlyCapturedCapability(t *testing.T) {
	checkOntologyCapability(t, "ontologyrelease/project_reader.go", "read_project_ontologies",
		[]string{"weave_releases", "weave_release_ontology_dependencies", "weave_ontology_releases", "weave_project_ontology_versions_archive", "weave_release_ontology_references"},
		[]string{"project_id = @project_id", "version_number = @version"})
}

func checkOntologyCapability(t *testing.T, reader, queries string, tablesAllowed, bindings []string) {
	t.Helper()
	root := repoRoot(t)
	file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(root, "pkg/database/"+reader), nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.StructType:
			for _, field := range node.Fields.List {
				switch ty := field.Type.(type) {
				case *ast.Ident:
					if ty.Name != "Reference" && ty.Name != "bool" && (reader != "ontologyrelease/project_reader.go" || ty.Name != "string") {
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
			if !slices.Contains(tablesAllowed, match[1]) {
				t.Errorf("%s reads mutable table %s", path, match[1])
			}
		}
		for _, binding := range bindings {
			if !strings.Contains(sql, binding) {
				t.Errorf("%s is missing scope binding %s", path, binding)
			}
		}
	}
}
