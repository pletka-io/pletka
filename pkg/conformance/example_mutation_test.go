package conformance

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// The editing service can hold only the operation boundary. A new method cannot
// acquire a hot reader by adding an optional view/store dependency or assertion.
func TestExampleMutationsCannotAcquireLiveValidationReaders(t *testing.T) {
	root := filepath.Join(repoRoot(t), "pkg/weave/example")
	for _, name := range []string{"service.go", "nested.go", "validation.go", "mutations.go"} {
		f, err := parser.ParseFile(token.NewFileSet(), filepath.Join(root, name), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range f.Imports {
			if strings.Contains(imp.Path.Value, "/database/") {
				t.Errorf("%s acquired persistence outside the operation adapter", name)
			}
		}
		ast.Inspect(f, func(n ast.Node) bool {
			if call, ok := n.(*ast.CallExpr); ok {
				if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
					switch sel.Sel.Name {
					case "ModelView", "GetByID", "At", "ConceptURIAllowedForLists":
						t.Errorf("%s calls live validation reader %s", name, sel.Sel.Name)
					}
				}
			}
			if spec, ok := n.(*ast.TypeSpec); ok && spec.Name.Name == "Service" {
				body, ok := spec.Type.(*ast.StructType)
				if !ok {
					t.Fatal("Service must be a struct")
				}
				for _, field := range body.Fields.List {
					for _, id := range field.Names {
						if id.Name != "mutations" && id.Name != "maxDepth" {
							t.Errorf("Service acquired %s", id.Name)
						}
					}
				}
			}
			return true
		})
	}
	// Context routing belongs to one infrastructure runner. The slice adapter
	// cannot open an independent read or write transaction itself.
	f, err := parser.ParseFile(token.NewFileSet(), filepath.Join(root, "mutations_postgres.go"), nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	runners := 0
	ast.Inspect(f, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
				if id, ok := sel.X.(*ast.Ident); ok && id.Name == "txscope" && sel.Sel.Name == "Run" {
					runners++
				}
				switch sel.Sel.Name {
				case "Begin", "BeginTx", "Commit", "At", "ModelView", "GetByID":
					t.Errorf("mutation adapter bypasses the shared operation with %s", sel.Sel.Name)
				}
			}
		}
		return true
	})
	if runners != 1 {
		t.Fatalf("expected one shared transaction entry, got %d", runners)
	}
}
