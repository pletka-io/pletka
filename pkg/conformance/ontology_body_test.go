package conformance

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
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
	checkOntologyCapability(t, "readsession/ontologies.go", "read_project_ontologies",
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
	if reader == "readsession/ontologies.go" {
		allowed := map[string]bool{
			"context": true, "encoding/json": true, "errors": true, "fmt": true,
			"github.com/jackc/pgx/v5": true,
			"github.com/pletka-io/pletka/pkg/database/readsql/projectontologies": true,
			"github.com/pletka-io/pletka/pkg/ontologyartifact":                   true,
		}
		for _, imp := range file.Imports {
			name, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				t.Fatal(err)
			}
			if !allowed[name] {
				t.Errorf("project ontology facet imports %s outside its captured SELECT capability", name)
			}
		}
	}
	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.StructType:
			for _, field := range node.Fields.List {
				switch ty := field.Type.(type) {
				case *ast.Ident:
					if ty.Name != "Reference" && ty.Name != "bool" && (reader != "readsession/ontologies.go" || ty.Name != "string") {
						t.Errorf("reader retains %s", ty.Name)
					}
				case *ast.StarExpr:
					id, ok := ty.X.(*ast.Ident)
					if reader != "readsession/ontologies.go" || !ok || id.Name != "lifetime" {
						t.Error("reader retains unrestricted pointer capability")
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

func TestLegacyOntologySnapshotReaderHasOnlyRetainedCapability(t *testing.T) {
	checkOntologyCapability(t, "ontologysnapshot/reader.go", "read_ontology_snapshot", []string{"weave_legacy_ontology_snapshots"}, []string{"project_id = @project_id", "version_number = @version", "digest = @digest"})
}

func TestImportedOntologySnapshotReaderHasOnlyRetainedCapability(t *testing.T) {
	checkOntologyCapability(t, "ontologysnapshot/imported_reader.go", "read_imported_ontology_snapshot", []string{"weave_imported_legacy_ontology_snapshots"}, []string{"project_id = @project_id", "version_number = @version", "digest = @digest"})
}
