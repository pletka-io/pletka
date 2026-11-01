package conformance

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestSKOSHTTPHandlerHasOnlyAnExportCapability(t *testing.T) {
	path := filepath.Join(repoRoot(t), "pkg/weave/vocabulary/skos_handler.go")
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	ast.Inspect(file, func(node ast.Node) bool {
		if spec, ok := node.(*ast.TypeSpec); ok && spec.Name.Name == "skosHandler" {
			body, ok := spec.Type.(*ast.StructType)
			if !ok || len(body.Fields.List) != 1 {
				t.Fatal("SKOS handler must retain only its exporter")
			}
			field, ok := body.Fields.List[0].Type.(*ast.Ident)
			if !ok || field.Name != "SKOSExporter" {
				t.Fatal("SKOS handler acquired an editing/store dependency")
			}
		}
		return true
	})
}

// A renderer or projection must not reacquire data through a new slice, pool,
// HTTP client or session. Adding a renderer does not grant loading capabilities.
func TestGeneratorProjectionHasNoLiveDataCapabilities(t *testing.T) {
	root := repoRoot(t)
	for _, file := range goFilesUnder(t, filepath.Join(root, "pkg/weave/generators")) {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		syntax, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(syntax, func(node ast.Node) bool {
			if call, ok := node.(*ast.CallExpr); ok {
				if selector, ok := call.Fun.(*ast.SelectorExpr); ok && selector.Sel.Name == "Legacy" {
					t.Errorf("%s reacquires raw legacy bytes; render from LegacyInterpretation", file)
				}
			}
			return true
		})
		for _, imp := range syntax.Imports {
			name, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(name, "/pkg/database") || strings.Contains(name, "/pkg/service") || strings.Contains(name, "pgx") || name == "database/sql" || name == "net/http" || strings.Contains(name, "/internal/test") {
				t.Errorf("%s imports live capability %s", file, name)
			}
			if strings.Contains(name, "/pkg/weave/") && !strings.Contains(name, "/pkg/weave/generators") && !strings.HasSuffix(name, "/pkg/weave/views") {
				t.Errorf("%s imports slice %s", file, name)
			}
		}
	}
}

func TestSyntheticSnapshotsAreOnlyImportedByTests(t *testing.T) {
	root := repoRoot(t)
	for _, file := range goFilesUnder(t, root) {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		syntax, err := parser.ParseFile(token.NewFileSet(), file, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range syntax.Imports {
			if strings.Contains(imp.Path.Value, "/internal/testsnapshot") {
				t.Errorf("production file %s imports synthetic data", file)
			}
		}
	}
}
